-- Delivery record for each invitation email (sent in the background after "Add user").
ALTER TABLE user_invitations
    ADD COLUMN email_status TEXT NOT NULL DEFAULT 'pending'
        CHECK (email_status IN ('pending', 'sent', 'failed', 'not_configured', 'unknown')),
    ADD COLUMN email_attempts INTEGER NOT NULL DEFAULT 0 CHECK (email_attempts >= 0),
    ADD COLUMN email_sent_at TIMESTAMPTZ,
    ADD COLUMN email_error TEXT CHECK (email_error IS NULL OR length(email_error) <= 500);
-- Invitations created before delivery was recorded.
UPDATE user_invitations SET email_status = 'unknown';
