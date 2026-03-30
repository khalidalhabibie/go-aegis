# Reliability Model

Aegis is designed around durable state transitions and retryable asynchronous work. It does not attempt exactly-once execution across brokers, workers, blockchain submission, and outbound webhooks. This document defines what is durable, what is at-least-once, and what protections exist against stale or duplicate work.

## Reliability Goals

- Keep transfer creation durable once the API responds successfully.
- Allow queue publish, transfer processing, and webhook delivery to recover after process or dependency failures.
- Reduce concurrent duplicate work without depending on a single in-memory worker owner.
- Resume blockchain submission from previously persisted attempt data instead of creating a fresh attempt after a crash.

## Delivery Guarantees

### Transfer creation

- A successful create request means the transfer record, initial status history, and outbox event are committed in PostgreSQL.
- RabbitMQ availability is not part of the synchronous success path.

### Queue publish and outbox dispatch

- Transfer job dispatch is at-least-once.
- `transfer_outbox` rows stay retryable until RabbitMQ publish succeeds.
- Duplicate outbox publish is tolerated by design.

### Transfer processing

- The transfer consumer uses a Redis lock per transfer ID to reduce concurrent duplicate work.
- Status transitions are compare-and-set on the current transfer status.
- Duplicate processing is reduced, not eliminated. A short-lived lock is still an operational coordination mechanism, not a global consensus lease.

### Blockchain submission recovery

- Aegis persists durable `transaction_attempts` before broadcast.
- Retries load the latest durable attempt and resume from its stored status.
- Recovery favors rebroadcasting the same signed payload over signing a new transaction.

### Webhook delivery

- Webhook delivery is at-least-once.
- Webhook receivers should treat `X-Aegis-Delivery-ID` as an idempotency key.
- A worker can retry even after the receiver processed the callback if the sender crashes, times out, or loses ownership before persisting `DELIVERED`.

## Duplicate-Work Controls

### Transfer-side controls

- Redis processing lock per transfer
- compare-and-set transfer status transitions
- compare-and-set `transaction_attempts` updates on the expected attempt status

These layers narrow the stale-worker window and prevent older workers from blindly overwriting newer transfer or attempt state.

### Webhook-side controls

- claim rows only when they are due
- move claimed rows to `IN_PROGRESS` with a lease expiry
- fence delivery result writes on both `id` and `lease_expires_at`

This prevents a stale worker from recording a result after its lease has expired and another worker has taken ownership.

## Durable Submission Recovery

The durable attempt lifecycle is centered on three recovery-relevant states:

- `SIGNED`: the raw signed transaction, nonce, and `tx_hash` are persisted
- `BROADCASTING`: the worker is attempting to submit the exact stored payload
- `BROADCASTED`: broadcast completed and follow-up state advancement can resume later

If a worker crashes after signing or during broadcast, the next run reloads the latest durable attempt and continues from that state instead of constructing a new transaction attempt from scratch.

## Webhook Retry and Lease Model

- Failed deliveries retry with exponential backoff.
- `WEBHOOK_MAX_ATTEMPTS` caps the retry budget.
- `WEBHOOK_LEASE_DURATION` controls row ownership while a worker is attempting delivery.
- The worker raises the effective lease duration to at least `WEBHOOK_TIMEOUT + 5s` to reduce lease expiry during slow outbound HTTP calls.

This lease floor reduces, but does not eliminate, the chance that a slow delivery attempt loses ownership before its result is persisted.

## Failure Isolation and Supervision

The worker supervises its major subsystems independently:

- transfer outbox dispatcher
- transfer consumer
- webhook worker

Operational consequences:

- a failure in one subsystem does not immediately stop the others
- each subsystem restarts with exponential backoff
- the worker health endpoint can report degraded subsystem state
- the worker exits if a subsystem exceeds its consecutive failure budget instead of looping forever in a broken state

## Operational Tradeoffs

- RabbitMQ publish and webhook delivery are at-least-once, not exactly-once.
- Redis locking reduces duplicate transfer work but does not provide a strict single-owner guarantee.
- Blockchain submission recovery is safer than signing a new transaction after a crash, but a recovered worker can still rebroadcast the same signed transaction.
- Lease and timeout sizing still matter. The worker enforces a minimum lease floor, but production values should still be tuned deliberately.

## Testing Focus

Current tests concentrate on the highest-risk correctness paths:

- transfer outbox durability and retry behavior
- durable transaction attempt recovery after crashes
- stale-worker attempt update conflicts
- multi-worker webhook claiming behavior
- webhook lease-loss handling

The corresponding security-sensitive tests, such as callback validation and internal auth behavior, are covered in the [Security Model](security-model.md).
