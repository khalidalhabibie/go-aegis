DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM wallets
        WHERE status NOT IN ('ACTIVE', 'INACTIVE')
    ) THEN
        RAISE EXCEPTION 'cannot add wallet status constraint: existing rows contain invalid wallet status values';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM transfer_requests
        WHERE status NOT IN (
            'CREATED',
            'VALIDATED',
            'QUEUED',
            'SIGNING',
            'SUBMITTED',
            'PENDING_ON_CHAIN',
            'CONFIRMED',
            'FAILED'
        )
    ) THEN
        RAISE EXCEPTION 'cannot add transfer status constraint: existing rows contain invalid transfer status values';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM transfer_status_history
        WHERE (from_status IS NOT NULL AND from_status NOT IN (
            'CREATED',
            'VALIDATED',
            'QUEUED',
            'SIGNING',
            'SUBMITTED',
            'PENDING_ON_CHAIN',
            'CONFIRMED',
            'FAILED'
        ))
        OR to_status NOT IN (
            'CREATED',
            'VALIDATED',
            'QUEUED',
            'SIGNING',
            'SUBMITTED',
            'PENDING_ON_CHAIN',
            'CONFIRMED',
            'FAILED'
        )
    ) THEN
        RAISE EXCEPTION 'cannot add transfer status history constraint: existing rows contain invalid status values';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM transaction_attempts
        WHERE status NOT IN ('SIGNED', 'BROADCASTING', 'BROADCASTED', 'FAILED')
    ) THEN
        RAISE EXCEPTION 'cannot add transaction attempt status constraint: existing rows contain invalid attempt status values';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM transfer_requests
        WHERE source_wallet_id !~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
    ) THEN
        RAISE EXCEPTION 'cannot convert transfer_requests.source_wallet_id to UUID: existing rows contain non-UUID values';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM transfer_requests AS tr
        LEFT JOIN wallets AS w ON w.id = tr.source_wallet_id::uuid
        WHERE w.id IS NULL
    ) THEN
        RAISE EXCEPTION 'cannot add transfer wallet foreign key: existing transfer rows reference missing wallets';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM transaction_attempts
        WHERE transaction_hash IS NOT NULL
            AND transaction_hash <> ''
        GROUP BY transaction_hash
        HAVING COUNT(*) > 1
    ) THEN
        RAISE EXCEPTION 'cannot make transaction_hash unique: duplicate transaction hashes already exist';
    END IF;
END
$$;

ALTER TABLE wallets
    ADD CONSTRAINT chk_wallets_status
    CHECK (status IN ('ACTIVE', 'INACTIVE'));

ALTER TABLE transfer_requests
    ALTER COLUMN source_wallet_id TYPE UUID USING source_wallet_id::uuid;

ALTER TABLE transfer_requests
    ADD CONSTRAINT chk_transfer_requests_status
    CHECK (status IN (
        'CREATED',
        'VALIDATED',
        'QUEUED',
        'SIGNING',
        'SUBMITTED',
        'PENDING_ON_CHAIN',
        'CONFIRMED',
        'FAILED'
    )),
    ADD CONSTRAINT fk_transfer_requests_source_wallet_id
    FOREIGN KEY (source_wallet_id) REFERENCES wallets(id) ON DELETE RESTRICT;

ALTER TABLE transfer_status_history
    ADD CONSTRAINT chk_transfer_status_history_from_status
    CHECK (
        from_status IS NULL OR from_status IN (
            'CREATED',
            'VALIDATED',
            'QUEUED',
            'SIGNING',
            'SUBMITTED',
            'PENDING_ON_CHAIN',
            'CONFIRMED',
            'FAILED'
        )
    ),
    ADD CONSTRAINT chk_transfer_status_history_to_status
    CHECK (
        to_status IN (
            'CREATED',
            'VALIDATED',
            'QUEUED',
            'SIGNING',
            'SUBMITTED',
            'PENDING_ON_CHAIN',
            'CONFIRMED',
            'FAILED'
        )
    );

ALTER TABLE transaction_attempts
    ADD CONSTRAINT chk_transaction_attempts_status
    CHECK (status IN ('SIGNED', 'BROADCASTING', 'BROADCASTED', 'FAILED'));

DROP INDEX IF EXISTS idx_transaction_attempts_transaction_hash;

CREATE UNIQUE INDEX IF NOT EXISTS uq_transaction_attempts_transaction_hash
    ON transaction_attempts (transaction_hash)
    WHERE transaction_hash IS NOT NULL AND transaction_hash <> '';
