DROP INDEX IF EXISTS idx_webhook_deliveries_claimable;

ALTER TABLE webhook_deliveries
    DROP COLUMN IF EXISTS lease_expires_at;
