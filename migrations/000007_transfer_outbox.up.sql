CREATE TABLE IF NOT EXISTS transfer_outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transfer_request_id UUID NOT NULL REFERENCES transfer_requests(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL CHECK (status IN ('PENDING', 'PROCESSING', 'RETRY', 'DISPATCHED')),
    attempt_count INT NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_at TIMESTAMPTZ,
    dispatched_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_transfer_outbox_transfer_event_unique
    ON transfer_outbox (transfer_request_id, event_type);

CREATE INDEX IF NOT EXISTS idx_transfer_outbox_status_available_at
    ON transfer_outbox (status, available_at, created_at)
    WHERE status IN ('PENDING', 'RETRY');

CREATE INDEX IF NOT EXISTS idx_transfer_outbox_processing_locked_at
    ON transfer_outbox (status, locked_at)
    WHERE status = 'PROCESSING';
