DROP INDEX IF EXISTS idx_webhook_deliveries_status_next_attempt;
DROP INDEX IF EXISTS uq_webhook_deliveries_transfer_status_history_id;

ALTER TABLE webhook_deliveries
    DROP COLUMN IF EXISTS response_body,
    DROP COLUMN IF EXISTS response_status_code,
    DROP COLUMN IF EXISTS max_attempts,
    DROP COLUMN IF EXISTS delivery_status,
    DROP COLUMN IF EXISTS payload_json,
    DROP COLUMN IF EXISTS transfer_status,
    DROP COLUMN IF EXISTS transfer_status_history_id;
