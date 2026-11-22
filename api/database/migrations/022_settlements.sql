CREATE TABLE settlements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    payout_reference TEXT NOT NULL UNIQUE,
    source_account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    clearing_account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    destination_account_id UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    amount BIGINT NOT NULL CHECK (amount > 0),
    currency TEXT NOT NULL CHECK (currency IN ('JPY', 'USD', 'EUR')),
    status TEXT NOT NULL DEFAULT 'RESERVED' CHECK (status IN ('RESERVED', 'CAPTURED', 'RELEASED')),
    provider_mode TEXT NOT NULL DEFAULT 'SANDBOX' CHECK (provider_mode IN ('SANDBOX', 'LIVE')),
    provider_reference TEXT NOT NULL,
    beneficiary_name TEXT NOT NULL,
    beneficiary_bank TEXT NOT NULL,
    beneficiary_account TEXT NOT NULL,
    reserve_journal_id UUID NOT NULL REFERENCES journal_entries(id) ON DELETE RESTRICT,
    capture_journal_id UUID REFERENCES journal_entries(id) ON DELETE RESTRICT,
    release_journal_id UUID REFERENCES journal_entries(id) ON DELETE RESTRICT,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_settlements_status ON settlements (status, created_at DESC);
CREATE INDEX idx_settlements_source ON settlements (source_account_id);
