CREATE TABLE IF NOT EXISTS reconciliation_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transfer_request_id UUID NOT NULL REFERENCES transfer_requests(id) ON DELETE CASCADE,
    tx_hash TEXT NOT NULL DEFAULT '',
    internal_status TEXT NOT NULL,
    blockchain_status TEXT NOT NULL,
    is_mismatch BOOLEAN NOT NULL DEFAULT false,
    notes TEXT NOT NULL DEFAULT '',
    trigger_source TEXT NOT NULL DEFAULT 'manual',
    checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_reconciliation_results_transfer_request_id_checked_at
    ON reconciliation_results (transfer_request_id, checked_at DESC);

CREATE INDEX IF NOT EXISTS idx_reconciliation_results_is_mismatch_checked_at
    ON reconciliation_results (is_mismatch, checked_at DESC);
