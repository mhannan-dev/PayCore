-- Transactional outbox delivery: leased claims, bounded retry backoff and audited replay.
-- Migration 014 created outbox_events; this adds delivery state without rewriting it.
ALTER TABLE outbox_events
    ADD COLUMN lease_id TEXT,
    ADD COLUMN lease_until TIMESTAMPTZ,
    ADD COLUMN available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN last_error TEXT CHECK (last_error IS NULL OR length(last_error) <= 500);

ALTER TABLE outbox_events DROP CONSTRAINT outbox_events_status_check;
ALTER TABLE outbox_events ADD CONSTRAINT outbox_events_status_check
    CHECK (status IN ('PENDING', 'SENDING', 'PUBLISHED', 'FAILED'));
CREATE INDEX idx_outbox_due ON outbox_events (status, available_at, created_at)
    WHERE status IN ('PENDING', 'SENDING');

-- Replay changes delivery state, so it is restricted to system operators and audited.
INSERT INTO permissions (code, name, description) VALUES
    ('system:write', 'Manage System Delivery', 'Replay failed outbox events and retry broker delivery')
ON CONFLICT (code) DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
WHERE r.code IN ('ADMIN', 'STAFF_ENGINEER') AND p.code = 'system:write'
ON CONFLICT DO NOTHING;

ALTER TABLE audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_action_check CHECK(action IN (
    'login_succeeded','login_failed','logout','user_invited','invitation_resent','invitation_accepted',
    'roles_changed','user_deactivated','user_activated','user_deleted','password_reset_requested','password_reset_completed',
    'mfa_enrolled','mfa_verified','mfa_failed','mfa_recovery_used',
    'mfa_recovery_regenerated','mfa_replacement_started','mfa_replaced','sessions_revoked','statement_imported',
    'statement_matching_configured','statement_match_confirmed','statement_match_rejected','statement_match_reset','statement_exception_noted',
    'outbox_replay'));
