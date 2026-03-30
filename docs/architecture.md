# Architecture

Aegis separates request durability, asynchronous execution, blockchain submission, and webhook delivery into distinct components. This document captures the system shape and the reasoning behind the main design choices.

## System Overview

High-level flow:

`API -> Postgres -> transfer_outbox -> RabbitMQ -> worker -> blockchain signer/broadcaster adapter -> webhook_deliveries -> reconciliation_results`

At a glance:

- The API owns validation, persistence, and transfer creation.
- PostgreSQL is the source of truth for transfer state, status history, transaction attempts, outbox events, webhooks, wallets, and reconciliation results.
- RabbitMQ carries asynchronous transfer jobs from the API side to the worker side.
- Redis provides a short-lived processing lock to reduce concurrent duplicate transfer work.
- Worker subsystems handle outbox dispatch, transfer processing, and webhook delivery independently.

## Core Components

### API service

- Exposes the public transfer endpoints and internal operational endpoints.
- Writes the primary transfer record, status history, and outbox event in a single PostgreSQL transaction.
- Returns success once the request is durable in PostgreSQL; queue publish happens later.

### Transfer outbox dispatcher

- Polls `transfer_outbox`.
- Claims pending or stale rows.
- Publishes transfer jobs to RabbitMQ.
- Marks rows as `DISPATCHED` only after successful publish.

### Transfer consumer

- Consumes transfer jobs from RabbitMQ.
- Acquires a Redis lock per transfer to reduce concurrent work on the same transfer.
- Advances the transfer state machine.
- Persists durable `transaction_attempts` so retries can resume the exact submission path.

### Webhook worker

- Creates webhook delivery rows for eligible status changes.
- Claims due webhook rows with a lease.
- Sends outbound callbacks with optional HMAC signing.
- Retries failures with backoff until the delivery exhausts its attempt budget.

### Reconciliation flow

- Compares persisted transfer state with blockchain receipt observations.
- Writes `reconciliation_results` for later inspection.
- Drives mismatch visibility and can move transfers toward `CONFIRMED` once a real checker exists.

## Request Lifecycle

### 1. Transfer creation

`POST /api/v1/transfers` validates the request and writes:

- `transfer_requests`
- the initial `transfer_status_history` row
- a `transfer_outbox` event

These writes happen in one PostgreSQL transaction so transfer creation stays durable even if RabbitMQ is temporarily unavailable.

### 2. Durable dispatch

The outbox dispatcher claims available rows from `transfer_outbox`, publishes the transfer job, and updates the row only after the broker acknowledges the publish path. If publish fails, the row remains retryable and will be picked up again later.

### 3. Transfer processing

The transfer consumer acquires a Redis lock, loads the current transfer state, and advances the workflow through validation, signing, and submission-related states. Before blockchain broadcast, it persists a durable `transaction_attempts` row containing the signed payload, nonce, and deterministic transaction hash.

### 4. Webhook delivery

When transfers move into webhook-eligible states, the worker creates `webhook_deliveries` rows. Another worker loop claims due rows with a lease, sends the HTTP callback, and records either delivery success or the next retry time.

### 5. Reconciliation

Internal reconciliation compares the stored transfer status against observed on-chain receipt state and writes `reconciliation_results`. This is the current path for surfacing mismatches and confirming transfers after broadcast.

## Key Design Decisions

### Why the transactional outbox exists

Without the outbox, transfer creation would depend on RabbitMQ being available during the API request. The outbox keeps the request durable in PostgreSQL first and lets asynchronous dispatch happen later. That decouples API success from temporary broker outages and makes queue publish retryable.

### Why durable transaction attempts exist

Blockchain submission is not safe to treat as a stateless retry. Aegis stores the signed payload, nonce, and deterministic `tx_hash` before broadcast so a retry can resume the same attempt rather than sign a brand-new transaction with different chain effects.

### Why webhook claims use leases

Webhook delivery can run on multiple workers. A lease on each claimed row reduces double ownership, and delivery result writes are fenced on both the row identity and the claimed lease value so stale workers cannot overwrite newer outcomes after losing ownership.

## Worker Topology

The worker supervises these subsystems independently:

- transfer outbox dispatcher
- transfer consumer
- webhook worker

That supervision model provides:

- restart with backoff per subsystem
- failure isolation between dispatch, transfer processing, and webhook delivery
- a worker health endpoint that can expose degraded subsystem state when enabled
- process exit after a subsystem exceeds its consecutive failure budget

Graceful shutdown is also split cleanly:

- the API server uses `APP_SHUTDOWN_TIMEOUT` for HTTP shutdown
- the worker waits for supervised subsystems to stop after the root context is cancelled

## Data Model Summary

- `transfer_requests`: primary transfer record, including idempotency key, wallet reference, callback URL, amount, status, and optional `tx_hash`
- `transfer_status_history`: append-only transfer status transitions
- `transaction_attempts`: durable signed payloads, nonce, transaction hash, last error, and submission status for recovery
- `transfer_outbox`: pending, retrying, processing, or dispatched transfer job publish records
- `webhook_deliveries`: per-status delivery rows with payload, attempts, last error, next attempt time, and lease expiry
- `reconciliation_results`: persisted comparison of internal status and observed blockchain receipt status
- `wallets`: source wallets, with duplicate active wallet protection on `(chain, lower(address))`

## Current Gaps

- The default signer and broadcaster are mocks in `internal/modules/transfers/mocks.go`.
- Reconciliation still relies on a placeholder receipt checker in `internal/modules/reconciliation/checker.go`.
- There is no dedicated automatic confirmation watcher; confirmation currently depends on reconciliation.

For delivery semantics and duplicate-handling guarantees, continue with the [Reliability Model](reliability-model.md). For request validation, auth, signing, and callback hardening, see the [Security Model](security-model.md).
