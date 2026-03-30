ALTER TABLE webhook_deliveries
    DROP CONSTRAINT IF EXISTS chk_webhook_deliveries_state_consistency,
    DROP CONSTRAINT IF EXISTS chk_webhook_deliveries_response_status_code,
    DROP CONSTRAINT IF EXISTS chk_webhook_deliveries_attempt_bounds,
    DROP CONSTRAINT IF EXISTS chk_webhook_deliveries_max_attempts,
    DROP CONSTRAINT IF EXISTS chk_webhook_deliveries_attempt_count,
    DROP CONSTRAINT IF EXISTS chk_webhook_deliveries_delivery_status,
    DROP CONSTRAINT IF EXISTS chk_webhook_deliveries_transfer_status;
