# Aegis

Aegis is a Go backend for orchestrating EVM-compatible transfer workflows. It exposes an API for transfer creation and lookup, persists workflow state in PostgreSQL, dispatches transfer work through RabbitMQ, uses Redis to reduce duplicate processing, delivers outbound status webhooks, and provides internal reconciliation endpoints.

## Project Overview

### What Aegis does

- Accepts transfer requests with idempotency keys.
- Persists transfer state and status history in PostgreSQL.
- Dispatches transfer jobs asynchronously through a transactional outbox.
- Tracks durable blockchain submission attempts so retries can resume from stored state.
- Schedules and delivers transfer status webhooks with retry and lease-based claiming.
- Provides internal wallet and reconciliation endpoints for operational workflows.

### Key capabilities

- Public transfer API: create, get by ID, list.
- Worker-side outbox dispatcher for durable RabbitMQ publish.
- Transfer consumer with Redis-backed duplicate-processing lock.
- Durable `transaction_attempts` recovery for `SIGNED`, `BROADCASTING`, and `BROADCASTED` states.
- Webhook retries with exponential backoff, HMAC signing, response body truncation, and lease-fenced multi-worker claiming.
- Internal authentication for wallet and reconciliation routes.
- Integrity constraints for transfer status, wallet status, transfer wallet foreign keys, and transaction attempt status/hash rules.

### Current scope and limitations

- The transfer processor is wired for blockchain submission, but the default signer and broadcaster are mocks in `internal/modules/transfers/mocks.go`.
- Reconciliation uses a placeholder receipt checker in `internal/modules/reconciliation/checker.go`; it is not a real on-chain receipt poller yet.
- There is no migration runner binary in the repo; SQL migrations are applied manually.
- There is no automatic confirmation watcher in the worker today. `CONFIRMED` state is driven by reconciliation, not by a dedicated listener.

## Architecture Summary

High-level flow:

`API -> Postgres -> transfer_outbox -> RabbitMQ -> worker -> blockchain signer/broadcaster adapter -> webhook_deliveries -> reconciliation_results`

More concretely:

1. `POST /api/v1/transfers` validates input and writes `transfer_requests`, the initial `transfer_status_history` row, and a `transfer_outbox` event in one PostgreSQL transaction.
2. The outbox dispatcher polls `transfer_outbox`, claims pending rows, publishes transfer jobs to RabbitMQ, and only marks rows `DISPATCHED` after successful publish.
3. The transfer consumer reads RabbitMQ jobs, acquires a short Redis lock per transfer, advances the transfer state machine, and persists durable transaction attempts before blockchain submission.
4. The webhook worker schedules `SUBMITTED`, `CONFIRMED`, and `FAILED` events into `webhook_deliveries`, claims due rows with leases, and retries failed deliveries with backoff.
5. Internal reconciliation compares stored transfer state with blockchain receipt observations and writes `reconciliation_results`.

### Why the outbox exists

Without the outbox, transfer creation would depend on RabbitMQ being available during the API request. The outbox keeps the create request durable in PostgreSQL first, then lets the worker publish later. That makes queue publish at-least-once and decouples API success from temporary broker outages.

### Why durable transaction attempts exist

Blockchain submission is not safe to treat as a stateless retry. Aegis persists the signed payload, nonce, and deterministic `tx_hash` in `transaction_attempts` before broadcast. If a worker crashes after signing or after broadcast, the next run resumes from the latest durable attempt instead of signing a brand-new transaction.

### Webhook claiming and lease safety

Webhook workers claim due deliveries by moving rows to `IN_PROGRESS` with a lease expiry. Delivery result writes are fenced on both `id` and the claimed `lease_expires_at`, so a stale worker cannot overwrite a newer worker's result after its lease is lost. The worker also enforces an effective lease duration of at least `WEBHOOK_TIMEOUT + 5s` to reduce lease expiry during slow HTTP calls.

## Reliability Model

### Queue publish and outbox dispatch

- Transfer job dispatch is at-least-once.
- `transfer_outbox` rows stay retryable until RabbitMQ publish succeeds.
- Duplicate outbox publish is tolerated by design.

### Duplicate-processing prevention

- The transfer consumer uses a Redis lock per transfer ID to reduce concurrent duplicate work.
- Transfer status transitions are compare-and-set on the current status.
- `transaction_attempts` updates are compare-and-set on the expected attempt status, so stale workers cannot regress a newer attempt state.
- Webhook delivery writes require the active lease to still match.
- Webhook receivers should still treat `X-Aegis-Delivery-ID` as an idempotency key, because webhook delivery is also at-least-once.

### Durable submission recovery

- `SIGNED`: the raw signed transaction, nonce, and `tx_hash` are persisted.
- `BROADCASTING`: the worker is attempting to submit the exact stored payload.
- `BROADCASTED`: the worker recorded that broadcast completed and can safely finish transfer state advancement later.
- On retry, the processor reloads the latest durable attempt and resumes from that state.

### Operational tradeoffs

- RabbitMQ publish and webhook delivery are at-least-once, not exactly-once.
- Redis locking reduces duplicate transfer work but is still a short-lived operational lock, not a global consensus mechanism.
- Lease and timeout sizing still matter. The worker raises the effective webhook lease above the HTTP timeout, but production values should still be set deliberately.

## Security Model

### Internal operational authentication

- Wallet and reconciliation routes are protected by `INTERNAL_AUTH_HEADER` and `INTERNAL_AUTH_API_KEY`.
- If the internal API key is unset, protected routes fail closed with `503 Service Unavailable`.

### Webhook signing

When `WEBHOOK_SIGNING_SECRET` is set, outbound webhooks include:

- `X-Aegis-Timestamp`
- `X-Aegis-Signature`

The signature format is `v1=<hex hmac sha256>`, computed over `<timestamp>.<raw_body>`.

### Callback URL validation policy

At transfer creation time, callback URLs:

- must use `http` or `https`
- must not include user credentials
- reject `localhost`, `.localhost`, `.local`, `.internal`, and private/local IP literals by default
- can be restricted further with `CALLBACK_URL_ALLOWED_HOSTS`
- can allow private targets only when `CALLBACK_URL_ALLOW_PRIVATE_TARGETS=true`

At webhook dispatch time, the worker re-validates the resolved destination:

- the hostname is checked against the allowlist policy again
- DNS results are rejected if any resolved IP is loopback, private, link-local, multicast, or unspecified
- redirects are re-validated before the client follows them

### SSRF handling note

These controls are application-layer SSRF mitigations. They materially reduce obvious callback abuse, but they are not a replacement for network egress controls. Production deployments should still restrict outbound network paths at the infrastructure layer.

### Response body storage limits

- Webhook response bodies are truncated and sanitized before persistence.
- `WEBHOOK_RESPONSE_BODY_MAX_BYTES` defaults to `512`.
- Null bytes are removed and invalid UTF-8 is normalized before storage.

## Local Development

### Prerequisites

- Go `1.20+`
- Docker and Docker Compose
- `psql` if you want to apply migrations manually from the shell

### Environment setup

```bash
cp .env.example .env
```

Review `.env` before running the stack, especially:

- `INTERNAL_AUTH_API_KEY`
- `WEBHOOK_SIGNING_SECRET`
- `CALLBACK_URL_ALLOWED_HOSTS`
- `CALLBACK_URL_ALLOW_PRIVATE_TARGETS`
- `EVM_RPC_URL`

### Start infrastructure with Docker Compose

```bash
docker compose up -d postgres redis rabbitmq
```

RabbitMQ management UI is exposed at `http://127.0.0.1:15672`.

### Apply migrations

The repo ships raw SQL migrations under `migrations/`. Apply them in filename order:

```bash
for f in migrations/*.up.sql; do
  psql "postgres://aegis:aegis@127.0.0.1:5432/aegis?sslmode=disable" -f "$f"
done
```

### Run the API

```bash
go run ./cmd/api
```

### Run the worker

```bash
go run ./cmd/worker
```

### Run the full stack in containers

```bash
cp .env.example .env
docker compose up --build
```

### Test commands

```bash
go test ./...
go test ./internal/modules/transfers ./internal/modules/webhooks ./internal/transport/http
```

## Configuration

Important environment variables are defined in `.env.example`.

### Core runtime

- `DATABASE_URL`: PostgreSQL connection string.
- `RABBITMQ_URL`: RabbitMQ connection string.
- `REDIS_ADDR`: Redis address for duplicate-processing locks.
- `EVM_RPC_URL`: RPC endpoint used by the EVM adapter and health checks.
- `EVM_CHAIN_ID`: expected chain ID for RPC validation.

### Transfer and worker reliability

- `WORKER_TRANSFER_MAX_RETRIES`: max RabbitMQ job retries after transient transfer errors.
- `WORKER_TRANSFER_RETRY_DELAY`: delay before a transfer job is republished.
- `WORKER_TRANSFER_PROCESS_LOCK_TTL`: Redis lock TTL for transfer processing.
- `WORKER_TRANSFER_OUTBOX_POLL_INTERVAL`: how often the outbox dispatcher polls.
- `WORKER_TRANSFER_OUTBOX_BATCH_SIZE`: number of outbox rows claimed per batch.
- `WORKER_TRANSFER_OUTBOX_RETRY_DELAY`: base delay for outbox publish retry backoff.
- `WORKER_TRANSFER_OUTBOX_PROCESSING_AFTER`: age after which a stuck outbox row can be reclaimed.
- `WORKER_WEBHOOK_POLL_INTERVAL`: webhook worker polling interval.

### Webhook delivery and signing

- `WEBHOOK_TIMEOUT`: HTTP timeout for outbound webhook delivery.
- `WEBHOOK_MAX_ATTEMPTS`: max webhook delivery attempts before permanent failure.
- `WEBHOOK_INITIAL_BACKOFF`: base retry delay for webhook delivery.
- `WEBHOOK_BATCH_SIZE`: number of webhook rows claimed per cycle.
- `WEBHOOK_LEASE_DURATION`: configured webhook claim lease; the worker raises the effective value if it is shorter than the timeout safety floor.
- `WEBHOOK_SIGNING_SECRET`: enables outbound HMAC signing when non-empty.
- `WEBHOOK_RESPONSE_BODY_MAX_BYTES`: persisted response body size cap.

### Security-related settings

- `INTERNAL_AUTH_HEADER`: header name checked on internal operational routes.
- `INTERNAL_AUTH_API_KEY`: shared secret for internal operational routes.
- `CALLBACK_URL_ALLOWED_HOSTS`: optional comma-separated allowlist for callback targets.
- `CALLBACK_URL_ALLOW_PRIVATE_TARGETS`: set to `true` only for trusted internal environments.

## API Summary

### Public endpoints

- `GET /healthz`
- `POST /api/v1/transfers`
- `GET /api/v1/transfers/:id`
- `GET /api/v1/transfers`

### Internal operational endpoints

These routes require the internal auth header and API key:

- `POST /api/v1/wallets`
- `GET /api/v1/wallets`
- `GET /api/v1/wallets/:id`
- `POST /api/v1/jobs/reconcile`
- `GET /api/v1/reconciliation/mismatches`

## Worker Behavior

### Transfer outbox dispatcher

- Polls `transfer_outbox`
- claims pending or stale-processing rows
- publishes transfer jobs to RabbitMQ
- records retry timing and last publish error

### Transfer consumer

- consumes RabbitMQ transfer jobs
- acquires a Redis processing lock
- advances transfers through `CREATED -> VALIDATED -> QUEUED -> SIGNING -> SUBMITTED -> PENDING_ON_CHAIN`
- persists and resumes durable `transaction_attempts`

### Webhook worker

- inserts webhook rows for eligible transfer status changes
- claims due webhook rows with leases
- dispatches signed HTTP callbacks
- retries failures with backoff until `WEBHOOK_MAX_ATTEMPTS`

### Failure isolation and supervision

- The worker supervises the transfer outbox dispatcher, transfer consumer, and webhook worker independently.
- A failure in one subsystem does not immediately stop the others.
- Each subsystem restarts with exponential backoff.
- This improves isolation, but production monitoring should alert on repeated restart loops because the process can remain alive while one subsystem is degraded.

### Graceful shutdown

- The API server uses `APP_SHUTDOWN_TIMEOUT` for HTTP shutdown.
- The worker waits for supervised subsystems to stop after the root context is cancelled.

## Data Model Summary

- `transfer_requests`: the primary transfer record, including idempotency key, wallet reference, callback URL, amount, status, and optional `tx_hash`.
- `transfer_status_history`: append-only transfer status transitions.
- `transaction_attempts`: durable signed payloads, nonce, `transaction_hash`, last error, and submission status for recovery.
- `transfer_outbox`: pending/retrying/processing/dispatched transfer job publish records.
- `webhook_deliveries`: per-status delivery rows with payload, delivery status, attempt counters, last error, next attempt time, and lease expiry.
- `reconciliation_results`: persisted comparison of internal transfer status vs observed blockchain receipt status.
- `wallets`: source wallets, with duplicate active wallet protection on `(chain, lower(address))`.

## Testing

Current tests focus on high-risk correctness and security paths:

- transfer outbox durability and retry behavior
- durable transaction attempt recovery after crashes
- stale-worker attempt update conflicts
- multi-worker webhook claiming behavior
- webhook lease-loss handling
- webhook signing and target validation
- internal auth middleware behavior

Useful commands:

```bash
go test ./...
go test ./internal/modules/transfers
go test ./internal/modules/webhooks
go test ./internal/transport/http
```

## Known Limitations

- Webhook lease handling is improved: stale workers can no longer overwrite a newer delivery state after losing their lease. Delivery is still at-least-once, not exactly-once. If a receiver processes the request but the worker crashes, times out, or loses the lease before persisting `DELIVERED`, the same webhook can be retried later. Production receivers still need idempotent handling keyed by `X-Aegis-Delivery-ID`.

- Durable transaction attempt ownership is improved: attempt status writes are now compare-and-set, so stale workers cannot silently regress a newer `transaction_attempts` row. The overall transfer worker still relies on a short Redis lock without lease renewal. If a worker stalls past the lock TTL or crashes after submitting a transaction but before persisting the next state, another worker can resume and rebroadcast the same signed transaction. That is safer than signing a new transaction, but it is still an at-least-once submission model and should not be treated as strict single-owner execution.

- Callback hardening is improved: Aegis validates callback URLs on create, re-validates DNS results before dispatch, and re-validates redirect targets. This is still application-layer SSRF mitigation, not a full network boundary. Validation and actual TCP connect are separate steps, so DNS rebinding and other time-of-check/time-of-use issues are not fully eliminated without tighter egress controls or a custom transport that pins validated IPs.

- Worker supervision is improved: transfer outbox dispatch, transfer consumption, and webhook delivery fail independently and restart with backoff. The tradeoff is degraded-but-still-running behavior. A broken subsystem can loop in restart while the process stays alive, so orchestration will not necessarily see a hard failure unless additional health signals, metrics, or alerts are added.

- The blockchain submission path is still not production-ready because the default signer and broadcaster are mocks in `internal/modules/transfers/mocks.go`. Reconciliation also still uses the placeholder receipt checker in `internal/modules/reconciliation/checker.go`, so on-chain state observation is not yet authoritative.
