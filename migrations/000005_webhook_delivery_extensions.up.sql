ALTER TABLE webhook_deliveries
    ADD COLUMN IF NOT EXISTS transfer_status_history_id UUID REFERENCES transfer_status_history(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS transfer_status TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS delivery_status TEXT NOT NULL DEFAULT 'PENDING',
    ADD COLUMN IF NOT EXISTS max_attempts INT NOT NULL DEFAULT 5,
    ADD COLUMN IF NOT EXISTS response_status_code INT,
    ADD COLUMN IF NOT EXISTS response_body TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS uq_webhook_deliveries_transfer_status_history_id
    ON webhook_deliveries (transfer_status_history_id)
    WHERE transfer_status_history_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_status_next_attempt
    ON webhook_deliveries (delivery_status, next_attempt_at);
