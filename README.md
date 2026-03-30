# Aegis

Aegis is a production-style Go backend scaffold for orchestrating EVM-compatible transfer workflows. The project is structured as a modular monolith so API, worker, infrastructure wiring, and future orchestration modules can grow without fragmenting into premature abstractions.

## Stack

- Go + Gin
- PostgreSQL via `pgxpool`
- Redis via `go-redis`
- RabbitMQ via `amqp091-go`
- EVM adapter via `go-ethereum`
- Structured JSON logs via `zerolog`
- Docker + Docker Compose

## Project Structure

```text
.
├── cmd
│   ├── api
│   │   └── main.go
│   └── worker
│       └── main.go
├── internal
│   ├── app
│   │   ├── api.go
│   │   ├── transfers.go
│   │   └── worker.go
│   ├── bootstrap
│   │   └── container.go
│   ├── config
│   │   └── config.go
│   ├── modules
│   │   ├── health
│   │   │   └── service.go
│   │   ├── reconciliation
│   │   │   ├── checker.go
│   │   │   ├── model.go
│   │   │   ├── postgres_repository.go
│   │   │   ├── repository.go
│   │   │   ├── service.go
│   │   │   └── service_test.go
│   │   ├── transfers
│   │       ├── consumer.go
│   │       ├── errors.go
│   │       ├── job.go
│   │       ├── lock.go
│   │       ├── model.go
│   │       ├── mocks.go
│   │       ├── attempt.go
│   │       ├── outbox.go
│   │       ├── outbox_dispatcher.go
│   │       ├── outbox_dispatcher_test.go
│   │       ├── outbox_repository.go
│   │       ├── processor.go
│   │       ├── processor_test.go
│   │       ├── postgres_repository.go
│   │       ├── publisher.go
│   │       ├── repository.go
│   │       ├── service.go
│   │       └── service_test.go
│   │   └── webhooks
│   │       ├── dispatcher.go
│   │       ├── model.go
│   │       ├── postgres_repository.go
│   │       ├── repository.go
│   │       ├── service.go
│   │       ├── service_test.go
│   │       └── worker.go
│   │   └── wallets
│   │       ├── model.go
│   │       ├── postgres_repository.go
│   │       ├── repository.go
│   │       ├── service.go
│   │       └── service_test.go
│   ├── platform
│   │   ├── blockchain
│   │   │   └── evm.go
│   │   ├── logger
│   │   │   └── logger.go
│   │   ├── postgres
│   │   │   └── postgres.go
│   │   ├── rabbitmq
│   │   │   └── rabbitmq.go
│   │   └── redis
│   │       └── redis.go
│   └── transport
│       └── http
│           ├── handlers
│           │   ├── health.go
│           │   ├── reconciliation.go
│           │   ├── reconciliation_dto.go
│           │   ├── response.go
│           │   ├── transfers.go
│           │   ├── transfers_dto.go
│           │   ├── wallets.go
│           │   └── wallets_dto.go
│           └── server.go
├── migrations
│   ├── 000001_init.down.sql
│   ├── 000001_init.up.sql
│   ├── 000002_transfer_status_history.down.sql
│   ├── 000002_transfer_status_history.up.sql
│   ├── 000003_transfer_tx_hash.down.sql
│   ├── 000003_transfer_tx_hash.up.sql
│   ├── 000004_wallets.down.sql
│   ├── 000004_wallets.up.sql
│   ├── 000005_webhook_delivery_extensions.down.sql
│   ├── 000005_webhook_delivery_extensions.up.sql
│   ├── 000006_reconciliation_results.down.sql
│   ├── 000006_reconciliation_results.up.sql
│   ├── 000007_transfer_outbox.down.sql
│   ├── 000007_transfer_outbox.up.sql
│   ├── 000008_transaction_attempt_recovery.down.sql
│   ├── 000008_transaction_attempt_recovery.up.sql
│   ├── 000009_webhook_delivery_leases.down.sql
│   └── 000009_webhook_delivery_leases.up.sql
├── .dockerignore
├── .env.example
├── Dockerfile
├── docker-compose.yml
├── go.mod
└── README.md
```

## Prerequisites

- Go 1.20+
- Docker and Docker Compose

## Local Setup

1. Copy the environment file.

   ```bash
   cp .env.example .env
   ```

2. Start the infrastructure services.

   ```bash
   docker compose up -d postgres redis rabbitmq
   ```

3. Apply the initial migration.

   ```bash
   psql "postgres://aegis:aegis@127.0.0.1:5432/aegis?sslmode=disable" -f migrations/000001_init.up.sql
   psql "postgres://aegis:aegis@127.0.0.1:5432/aegis?sslmode=disable" -f migrations/000002_transfer_status_history.up.sql
   psql "postgres://aegis:aegis@127.0.0.1:5432/aegis?sslmode=disable" -f migrations/000003_transfer_tx_hash.up.sql
   psql "postgres://aegis:aegis@127.0.0.1:5432/aegis?sslmode=disable" -f migrations/000004_wallets.up.sql
   psql "postgres://aegis:aegis@127.0.0.1:5432/aegis?sslmode=disable" -f migrations/000005_webhook_delivery_extensions.up.sql
   psql "postgres://aegis:aegis@127.0.0.1:5432/aegis?sslmode=disable" -f migrations/000006_reconciliation_results.up.sql
   psql "postgres://aegis:aegis@127.0.0.1:5432/aegis?sslmode=disable" -f migrations/000007_transfer_outbox.up.sql
   psql "postgres://aegis:aegis@127.0.0.1:5432/aegis?sslmode=disable" -f migrations/000008_transaction_attempt_recovery.up.sql
   psql "postgres://aegis:aegis@127.0.0.1:5432/aegis?sslmode=disable" -f migrations/000009_webhook_delivery_leases.up.sql
   ```

4. Install Go dependencies and run the API.

   ```bash
   go mod tidy
   go run ./cmd/api
   ```

5. In a second terminal, run the worker.

   ```bash
   go run ./cmd/worker
   ```

The health endpoint is available at `http://127.0.0.1:8080/healthz`.

## Docker Compose

To run the full stack in containers:

```bash
cp .env.example .env
docker compose up --build
```

The API container listens on port `8080`. RabbitMQ management is exposed on `http://127.0.0.1:15672`.

## Git Ignore

The repository root includes a `.gitignore` for:

- Go build outputs and caches
- local `.env` files
- editor and OS noise
- Foundry build artifacts and installed libraries under `contracts/`

## CI/CD

GitHub Actions workflows are included under `.github/workflows`:

- `ci.yml`: runs Go formatting checks, Go tests, binary builds, Docker image build validation, and Foundry format/test/build checks
- `cd.yml`: builds and pushes the production image to `ghcr.io/<owner>/<repo>` on version tags like `v1.0.0` or manual dispatch

Notes:

- The CI workflow installs Foundry dependencies during the run, so contract CI does not depend on committed `contracts/lib` artifacts.
- The published runtime image contains both `aegis-api` and `aegis-worker`, so the deploy platform can run the same image with different commands for API and worker processes.

## Current Capabilities

- API and worker processes with separate entrypoints
- Environment-driven configuration
- Graceful shutdown plumbing
- Structured request and runtime logging
- Postgres, Redis, RabbitMQ, and EVM bootstrap layers
- Health endpoint covering core dependencies
- Transfer request create/get/list API with PostgreSQL persistence and idempotency
- Transactional transfer outbox so create requests do not depend on immediate RabbitMQ publish success
- RabbitMQ-backed async transfer job dispatch and worker consumption
- Durable transaction attempts so signed payloads and `tx_hash` survive worker crashes during submission
- Resumable status machine: `CREATED -> VALIDATED -> QUEUED -> SIGNING -> SUBMITTED -> PENDING_ON_CHAIN`
- Transfer status history table with initial `CREATED` transition writes and every later transition recorded
- Wallet registry API with duplicate active-wallet protection on the same chain/address
- Webhook delivery worker for `SUBMITTED`, `CONFIRMED`, and `FAILED` transfer status events with retry/backoff and persisted delivery logs
- Multi-worker-safe webhook claiming with `IN_PROGRESS` leases and expired-lease reclamation
- Manual reconciliation job plus mismatch query API backed by persisted reconciliation results
- Mock signer and mock blockchain broadcaster placeholders for future replacement
- Initial schema for transfer requests, transaction attempts, and webhook deliveries

## Transfer API

### Create a transfer request

```bash
curl --request POST \
  --url http://127.0.0.1:8080/api/v1/transfers \
  --header 'Content-Type: application/json' \
  --data '{
    "idempotency_key": "txn-001",
    "chain": "ethereum",
    "asset_type": "native",
    "source_wallet_id": "wallet_hot_001",
    "destination_address": "0x000000000000000000000000000000000000dEaD",
    "amount": "1000000000000000000",
    "callback_url": "https://example.com/webhooks/transfers",
    "metadata_json": {
      "reference": "merchant-payout-42",
      "tenant_id": "tenant_abc"
    }
  }'
```

Sample response:

```json
{
  "data": {
    "id": "9ee80db8-c74f-4faf-9f96-2a6a13ac0b58",
    "idempotency_key": "txn-001",
    "chain": "ethereum",
    "asset_type": "native",
    "source_wallet_id": "wallet_hot_001",
    "destination_address": "0x000000000000000000000000000000000000dEaD",
    "amount": "1000000000000000000",
    "callback_url": "https://example.com/webhooks/transfers",
    "metadata_json": {
      "reference": "merchant-payout-42",
      "tenant_id": "tenant_abc"
    },
    "tx_hash": "",
    "status": "CREATED",
    "created_at": "2026-03-30T12:00:00Z",
    "updated_at": "2026-03-30T12:00:00Z"
  }
}
```

`POST /api/v1/transfers` returns `201 Created` for a new transfer and `200 OK` when the `idempotency_key` already exists.
The API writes the transfer row and a pending transfer outbox event in one Postgres transaction. A worker-side outbox dispatcher publishes that event to RabbitMQ and retries later if RabbitMQ is unavailable, so the create request remains durable even when the broker is temporarily down.
The transfer worker asynchronously advances the transfer through validation, queueing, signing, submission, and `PENDING_ON_CHAIN`.

## Transfer Dispatch Architecture

Transfer creation uses a transactional outbox:

1. `POST /api/v1/transfers` inserts the transfer request, writes the initial `CREATED` status history row, and inserts a `transfer_outbox` event in the same database transaction.
2. The outbox dispatcher polls pending outbox rows and publishes transfer jobs to RabbitMQ.
3. Outbox rows are marked `DISPATCHED` only after a successful publish.
4. If publish fails, the row stays retryable and the dispatcher backs off before trying again.
5. Duplicate outbox dispatch is tolerated. The transfer consumer uses a short-lived Redis lock per transfer to avoid concurrent duplicate processing.

## Durable Submission Recovery

Transfer submission uses `transaction_attempts` as durable recovery state:

1. When a transfer reaches `SIGNING`, the worker signs it and persists the signed payload, nonce, and deterministic `tx_hash` as a `SIGNED` transaction attempt before broadcast.
2. The worker moves that attempt to `BROADCASTING` and sends the exact persisted payload to the blockchain adapter.
3. After broadcast succeeds, the attempt is marked `BROADCASTED`, then the transfer is advanced to `SUBMITTED` and later `PENDING_ON_CHAIN`.
4. If the worker crashes after broadcast, retries inspect the latest persisted attempt instead of signing a brand-new transaction.
5. A `BROADCASTING` attempt is safe to rebroadcast because it reuses the same signed payload and `tx_hash`, which avoids accidental double-send semantics.

### Get a transfer by ID

```bash
curl --request GET \
  --url http://127.0.0.1:8080/api/v1/transfers/9ee80db8-c74f-4faf-9f96-2a6a13ac0b58
```

### List transfers

```bash
curl --request GET \
  --url 'http://127.0.0.1:8080/api/v1/transfers?limit=20&offset=0'
```

## Reconciliation API

### Run reconciliation

```bash
curl --request POST \
  --url http://127.0.0.1:8080/api/v1/jobs/reconcile
```

### List latest mismatches

```bash
curl --request GET \
  --url http://127.0.0.1:8080/api/v1/reconciliation/mismatches
```

## Recommended Next Steps

1. Add a transfer orchestration module with request validation, persistence, and queue publishing.
2. Introduce an outbox or event log table for reliable webhook and indexing fanout.
3. Implement worker consumers for transfer submission, confirmation polling, and retry policies.
4. Add request id propagation, auth, idempotency keys, and API versioning.
5. Add integration tests with testcontainers or docker-compose-backed CI jobs.
