DROP INDEX IF EXISTS idx_transaction_attempts_transfer_request_active_attempt;
DROP INDEX IF EXISTS idx_transaction_attempts_transaction_hash;
DROP INDEX IF EXISTS idx_transaction_attempts_transfer_request_status_created_at;

ALTER TABLE transaction_attempts
    DROP COLUMN IF EXISTS last_error;
