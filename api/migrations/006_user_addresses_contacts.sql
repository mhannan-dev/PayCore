-- A user can hold many postal addresses and many contact numbers.
-- Each table allows at most one primary row per user.

CREATE TABLE user_addresses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label TEXT NOT NULL DEFAULT 'HOME'
        CHECK (label IN ('HOME', 'WORK', 'BILLING', 'MAILING', 'OTHER')),
    line1 TEXT NOT NULL CHECK (length(btrim(line1)) BETWEEN 1 AND 200),
    line2 TEXT CHECK (line2 IS NULL OR length(btrim(line2)) BETWEEN 1 AND 200),
    city TEXT NOT NULL CHECK (length(btrim(city)) BETWEEN 1 AND 100),
    state_region TEXT CHECK (state_region IS NULL OR length(btrim(state_region)) BETWEEN 1 AND 100),
    postal_code TEXT CHECK (postal_code IS NULL OR length(btrim(postal_code)) BETWEEN 1 AND 20),
    -- ISO 3166-1 alpha-2, e.g. JP, US, BD.
    country_code CHAR(2) NOT NULL CHECK (country_code ~ '^[A-Z]{2}$'),
    is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_user_addresses_user ON user_addresses(user_id);
CREATE UNIQUE INDEX uq_user_addresses_primary ON user_addresses(user_id) WHERE is_primary;

CREATE TABLE user_contact_numbers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label TEXT NOT NULL DEFAULT 'MOBILE'
        CHECK (label IN ('MOBILE', 'HOME', 'WORK', 'OTHER')),
    -- E.164: leading +, country code, up to 15 digits total.
    phone_number TEXT NOT NULL CHECK (phone_number ~ '^\+[1-9][0-9]{6,14}$'),
    is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    verified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, phone_number)
);
CREATE INDEX idx_user_contact_numbers_user ON user_contact_numbers(user_id);
CREATE UNIQUE INDEX uq_user_contact_numbers_primary ON user_contact_numbers(user_id) WHERE is_primary;
