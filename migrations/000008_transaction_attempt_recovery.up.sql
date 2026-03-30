ALTER TABLE transaction_attempts
    ADD COLUMN IF NOT EXISTS last_error TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_transaction_attempts_transfer_request_status_created_at
    ON transaction_attempts (transfer_request_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_transaction_attempts_transaction_hash
    ON transaction_attempts (transaction_hash)
    WHERE transaction_hash IS NOT NULL AND transaction_hash <> '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_transaction_attempts_transfer_request_active_attempt
    ON transaction_attempts (transfer_request_id)
    WHERE status IN ('SIGNED', 'BROADCASTING');

