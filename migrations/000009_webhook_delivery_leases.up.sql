ALTER TABLE webhook_deliveries
    ADD COLUMN IF NOT EXISTS lease_expires_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_claimable
    ON webhook_deliveries (delivery_status, next_attempt_at, lease_expires_at, created_at);

