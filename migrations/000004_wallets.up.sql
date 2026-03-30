CREATE TABLE IF NOT EXISTS wallets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chain TEXT NOT NULL,
    address TEXT NOT NULL,
    label TEXT NOT NULL,
    signing_type TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_wallets_active_chain_address
    ON wallets (chain, lower(address))
    WHERE status = 'ACTIVE';

CREATE INDEX IF NOT EXISTS idx_wallets_chain_status_created_at
    ON wallets (chain, status, created_at DESC);
