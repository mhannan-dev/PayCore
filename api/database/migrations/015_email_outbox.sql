CREATE TABLE email_outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind TEXT NOT NULL CHECK (kind IN ('invitation', 'password_reset')),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash BYTEA NOT NULL CHECK (octet_length(token_hash) = 32),
    recipient TEXT NOT NULL,
    subject TEXT NOT NULL DEFAULT '',
    text_body TEXT NOT NULL DEFAULT '',
    html_body TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'sending', 'sent', 'failed', 'not_configured', 'canceled')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 5),
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    lease_id UUID,
    lease_until TIMESTAMPTZ,
    last_error TEXT,
    sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (kind, token_hash),
    CHECK ((status = 'sending' AND lease_id IS NOT NULL AND lease_until IS NOT NULL)
        OR (status <> 'sending' AND lease_id IS NULL AND lease_until IS NULL)),
    CHECK (status IN ('pending', 'sending') OR (text_body = '' AND html_body = ''))
);

CREATE INDEX email_outbox_ready_idx ON email_outbox(available_at, created_at)
    WHERE status = 'pending';
CREATE INDEX email_outbox_lease_idx ON email_outbox(lease_until)
    WHERE status = 'sending';
