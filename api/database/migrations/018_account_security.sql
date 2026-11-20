ALTER TABLE auth_sessions ADD COLUMN id UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE;
CREATE TABLE mfa_replacements (
 user_id UUID PRIMARY KEY REFERENCES user_mfa(user_id) ON DELETE CASCADE,
 session_id UUID NOT NULL REFERENCES auth_sessions(id) ON DELETE CASCADE,
 password_hash_snapshot TEXT NOT NULL,
 encrypted_secret BYTEA NOT NULL,
 expires_at TIMESTAMPTZ NOT NULL
);
ALTER TABLE audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_action_check CHECK(action IN (
 'login_succeeded','login_failed','logout','user_invited','invitation_resent','invitation_accepted',
 'roles_changed','user_deactivated','user_activated','user_deleted','password_reset_requested','password_reset_completed',
 'mfa_enrolled','mfa_verified','mfa_failed','mfa_recovery_used',
 'mfa_recovery_regenerated','mfa_replacement_started','mfa_replaced','sessions_revoked'));
