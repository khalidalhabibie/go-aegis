DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM webhook_deliveries
        WHERE transfer_status NOT IN (
            'SUBMITTED',
            'CONFIRMED',
            'FAILED'
        )
    ) THEN
        RAISE EXCEPTION 'cannot add webhook transfer status constraint: existing rows contain invalid transfer status values';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM webhook_deliveries
        WHERE delivery_status NOT IN (
            'PENDING',
            'RETRYING',
            'IN_PROGRESS',
            'DELIVERED',
            'FAILED'
        )
    ) THEN
        RAISE EXCEPTION 'cannot add webhook delivery status constraint: existing rows contain invalid delivery status values';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM webhook_deliveries
        WHERE attempt_count < 0
            OR max_attempts <= 0
            OR attempt_count > max_attempts
    ) THEN
        RAISE EXCEPTION 'cannot add webhook attempt counter constraints: existing rows contain invalid attempt counters';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM webhook_deliveries
        WHERE response_status_code IS NOT NULL
            AND (response_status_code < 100 OR response_status_code > 599)
    ) THEN
        RAISE EXCEPTION 'cannot add webhook response status constraint: existing rows contain invalid http status codes';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM webhook_deliveries
        WHERE (
            delivery_status IN ('PENDING', 'RETRYING')
            AND next_attempt_at IS NULL
        ) OR (
            delivery_status = 'IN_PROGRESS'
            AND lease_expires_at IS NULL
        ) OR (
            delivery_status = 'DELIVERED'
            AND (delivered_at IS NULL OR next_attempt_at IS NOT NULL)
        ) OR (
            delivery_status = 'FAILED'
            AND next_attempt_at IS NOT NULL
        )
    ) THEN
        RAISE EXCEPTION 'cannot add webhook state consistency constraint: existing rows contain inconsistent scheduling or lease state';
    END IF;
END
$$;

ALTER TABLE webhook_deliveries
    ADD CONSTRAINT chk_webhook_deliveries_transfer_status
    CHECK (transfer_status IN ('SUBMITTED', 'CONFIRMED', 'FAILED')),
    ADD CONSTRAINT chk_webhook_deliveries_delivery_status
    CHECK (delivery_status IN ('PENDING', 'RETRYING', 'IN_PROGRESS', 'DELIVERED', 'FAILED')),
    ADD CONSTRAINT chk_webhook_deliveries_attempt_count
    CHECK (attempt_count >= 0),
    ADD CONSTRAINT chk_webhook_deliveries_max_attempts
    CHECK (max_attempts > 0),
    ADD CONSTRAINT chk_webhook_deliveries_attempt_bounds
    CHECK (attempt_count <= max_attempts),
    ADD CONSTRAINT chk_webhook_deliveries_response_status_code
    CHECK (response_status_code IS NULL OR response_status_code BETWEEN 100 AND 599),
    ADD CONSTRAINT chk_webhook_deliveries_state_consistency
    CHECK (
        (
            delivery_status IN ('PENDING', 'RETRYING')
            AND next_attempt_at IS NOT NULL
        ) OR (
            delivery_status = 'IN_PROGRESS'
            AND lease_expires_at IS NOT NULL
        ) OR (
            delivery_status = 'DELIVERED'
            AND delivered_at IS NOT NULL
            AND next_attempt_at IS NULL
        ) OR (
            delivery_status = 'FAILED'
            AND next_attempt_at IS NULL
        )
    );
