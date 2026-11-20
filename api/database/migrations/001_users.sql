CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    first_name TEXT NOT NULL CHECK (length(btrim(first_name)) BETWEEN 1 AND 80),
    last_name TEXT CHECK (last_name IS NULL OR length(btrim(last_name)) BETWEEN 1 AND 80),
    email TEXT UNIQUE NOT NULL CHECK (email = lower(btrim(email)) AND length(email) BETWEEN 3 AND 254),
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'OPERATOR' CHECK (role IN ('OPERATOR', 'ADMIN', 'COMPLIANCE_AUDITOR', 'STAFF_ENGINEER')),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    photo TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE auth_sessions (
    token_hash BYTEA PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    CHECK (expires_at > created_at)
);
CREATE INDEX idx_auth_sessions_user ON auth_sessions(user_id);
CREATE INDEX idx_auth_sessions_expiry ON auth_sessions(expires_at);
