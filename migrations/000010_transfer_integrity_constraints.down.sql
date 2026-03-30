DROP INDEX IF EXISTS uq_transaction_attempts_transaction_hash;

CREATE INDEX IF NOT EXISTS idx_transaction_attempts_transaction_hash
    ON transaction_attempts (transaction_hash)
    WHERE transaction_hash IS NOT NULL AND transaction_hash <> '';

ALTER TABLE transaction_attempts
    DROP CONSTRAINT IF EXISTS chk_transaction_attempts_status;

ALTER TABLE transfer_status_history
    DROP CONSTRAINT IF EXISTS chk_transfer_status_history_to_status,
    DROP CONSTRAINT IF EXISTS chk_transfer_status_history_from_status;

ALTER TABLE transfer_requests
    DROP CONSTRAINT IF EXISTS fk_transfer_requests_source_wallet_id,
    DROP CONSTRAINT IF EXISTS chk_transfer_requests_status;

ALTER TABLE transfer_requests
    ALTER COLUMN source_wallet_id TYPE TEXT USING source_wallet_id::text;

ALTER TABLE wallets
    DROP CONSTRAINT IF EXISTS chk_wallets_status;
