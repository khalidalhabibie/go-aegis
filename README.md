# Aegis

Aegis is a Go backend for orchestrating EVM-compatible transfer workflows. It exposes a transfer API, persists workflow state in PostgreSQL, dispatches transfer work asynchronously through RabbitMQ, uses Redis to reduce duplicate processing, delivers outbound status webhooks, and provides internal operational endpoints for wallet and reconciliation workflows.

## Documentation

- [Architecture](docs/architecture.md)
- [Reliability Model](docs/reliability-model.md)
- [Security Model](docs/security-model.md)

Use the README as the project entrypoint. The detailed system design, delivery guarantees, and security controls now live in the documents above.

## Project Overview

### What Aegis does

- Accepts transfer requests with idempotency keys.
- Persists transfer state and status history in PostgreSQL.
- Dispatches transfer jobs asynchronously through a transactional outbox.
- Tracks durable blockchain submission attempts so retries can resume from stored state.
- Schedules and delivers transfer status webhooks with retry and lease-based claiming.
- Provides internal wallet and reconciliation endpoints for operational workflows.

### Current scope and limitations

- The transfer processor is wired for blockchain submission, but the default signer and broadcaster are mocks in `internal/modules/transfers/mocks.go`.
- Reconciliation uses a placeholder receipt checker in `internal/modules/reconciliation/checker.go`; it is not a real on-chain receipt poller yet.
- There is no automatic confirmation watcher in the worker today. `CONFIRMED` state is driven by reconciliation, not by a dedicated listener.
- Queue publish, blockchain submission recovery, and webhook delivery remain at-least-once workflows. See the [Reliability Model](docs/reliability-model.md) for the exact guarantees.

## Quickstart

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

### Start infrastructure

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

When enabled, the worker health endpoint is exposed at `http://127.0.0.1:8081/healthz` by default.

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

## Configuration Highlights

Important environment variables are defined in `.env.example`.

### Core runtime

- `DATABASE_URL`: PostgreSQL connection string.
- `RABBITMQ_URL`: RabbitMQ connection string.
- `REDIS_ADDR`: Redis address for duplicate-processing locks.
- `EVM_RPC_URL`: RPC endpoint used by the EVM adapter and health checks.
- `EVM_CHAIN_ID`: expected chain ID for RPC validation.

### Worker and delivery controls

- `WORKER_TRANSFER_MAX_RETRIES`
- `WORKER_TRANSFER_RETRY_DELAY`
- `WORKER_TRANSFER_PROCESS_LOCK_TTL`
- `WORKER_TRANSFER_OUTBOX_POLL_INTERVAL`
- `WORKER_TRANSFER_OUTBOX_BATCH_SIZE`
- `WORKER_TRANSFER_OUTBOX_RETRY_DELAY`
- `WORKER_TRANSFER_OUTBOX_PROCESSING_AFTER`
- `WORKER_WEBHOOK_POLL_INTERVAL`
- `WEBHOOK_TIMEOUT`
- `WEBHOOK_MAX_ATTEMPTS`
- `WEBHOOK_INITIAL_BACKOFF`
- `WEBHOOK_BATCH_SIZE`
- `WEBHOOK_LEASE_DURATION`

### Security-related settings

- `INTERNAL_AUTH_HEADER`
- `INTERNAL_AUTH_API_KEY`
- `CALLBACK_URL_ALLOWED_HOSTS`
- `CALLBACK_URL_ALLOW_PRIVATE_TARGETS`
- `WEBHOOK_SIGNING_SECRET`
- `WEBHOOK_RESPONSE_BODY_MAX_BYTES`

See the [Reliability Model](docs/reliability-model.md) and [Security Model](docs/security-model.md) for the behavioral impact of these settings.

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

## Further Reading

- [Architecture](docs/architecture.md) for the request lifecycle, worker topology, and data model.
- [Reliability Model](docs/reliability-model.md) for retry semantics, duplicate prevention, lease fencing, and tradeoffs.
- [Security Model](docs/security-model.md) for internal auth, webhook signing, callback validation, and SSRF boundaries.
