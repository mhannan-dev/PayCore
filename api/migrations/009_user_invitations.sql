-- Invited users have no password until they accept their emailed invitation.
-- (Very old schemas never had the column; their users had no password either.)
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash TEXT;
ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;

-- Single-use set-password links. Only the SHA-256 of the emailed token is stored.
CREATE TABLE user_invitations (
    token_hash BYTEA PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    CHECK (expires_at > created_at)
);
CREATE INDEX idx_user_invitations_user ON user_invitations(user_id);
