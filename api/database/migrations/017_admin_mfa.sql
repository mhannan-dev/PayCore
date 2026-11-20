ALTER TABLE auth_sessions ADD COLUMN mfa_verified BOOLEAN NOT NULL DEFAULT FALSE;
CREATE TABLE user_mfa (
 user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 encrypted_secret BYTEA,
 enabled_at TIMESTAMPTZ,
 last_counter BIGINT NOT NULL DEFAULT -1,
 failed_attempts INTEGER NOT NULL DEFAULT 0 CHECK(failed_attempts BETWEEN 0 AND 5),
 locked_until TIMESTAMPTZ,
 CHECK ((encrypted_secret IS NULL) = (enabled_at IS NULL))
);
CREATE TABLE mfa_challenges (
 token_hash BYTEA PRIMARY KEY CHECK(octet_length(token_hash)=32),
 user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
 password_hash_snapshot TEXT NOT NULL,
 pending_secret BYTEA,
 expires_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE mfa_recovery_codes (
 user_id UUID NOT NULL REFERENCES user_mfa(user_id) ON DELETE CASCADE,
 code_hash BYTEA NOT NULL CHECK(octet_length(code_hash)=32),
 PRIMARY KEY(user_id,code_hash)
);
-- Existing password-only admin sessions must not bypass enrollment.
DELETE FROM auth_sessions WHERE user_id IN (SELECT id FROM users WHERE 'ADMIN'=ANY(roles));

ALTER TABLE audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_action_check CHECK(action IN (
 'login_succeeded','login_failed','logout','user_invited','invitation_resent','invitation_accepted',
 'roles_changed','user_deactivated','user_activated','user_deleted','password_reset_requested','password_reset_completed',
 'mfa_enrolled','mfa_verified','mfa_failed','mfa_recovery_used'));
