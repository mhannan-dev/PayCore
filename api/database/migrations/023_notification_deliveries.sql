CREATE TABLE notification_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID NOT NULL REFERENCES outbox_events(id) ON DELETE RESTRICT,
    journal_id UUID NOT NULL REFERENCES journal_entries(id) ON DELETE RESTRICT,
    channel TEXT NOT NULL CHECK (channel IN ('EMAIL','WEBHOOK')),
    destination TEXT NOT NULL CHECK (destination IN ('sandbox-email-inbox','sandbox-webhook-receiver')),
    status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','DELIVERED','FAILED')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error TEXT,
    receipt TEXT,
    delivered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (event_id, channel),
    CHECK ((status = 'DELIVERED') = (receipt IS NOT NULL AND delivered_at IS NOT NULL)),
    CHECK ((status = 'FAILED') = (last_error IS NOT NULL)),
    CHECK (status = 'PENDING' OR attempts > 0)
);
CREATE INDEX idx_notification_history ON notification_deliveries (created_at DESC, id DESC);
CREATE TABLE notification_attempts (
    delivery_id UUID NOT NULL REFERENCES notification_deliveries(id) ON DELETE RESTRICT,
    lease_id TEXT NOT NULL,
    attempt INTEGER NOT NULL CHECK (attempt > 0),
    status TEXT NOT NULL CHECK (status IN ('DELIVERED','FAILED')),
    receipt TEXT,
    error TEXT,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (delivery_id, lease_id),
    UNIQUE (delivery_id, attempt),
    CHECK ((status = 'DELIVERED') = (receipt IS NOT NULL)),
    CHECK ((status = 'FAILED') = (error IS NOT NULL))
);
INSERT INTO permissions(code,name,description) VALUES
('notifications:read','Read Notification Deliveries','Inspect sandbox delivery outcomes and linked ledger records'),
('notifications:retry','Retry Notification Deliveries','Requeue failed notification events without resending successful channels')
ON CONFLICT(code) DO NOTHING;
INSERT INTO role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM roles r CROSS JOIN permissions p
WHERE r.code IN ('ADMIN','OPERATOR','LEAD_OPERATOR','COMPLIANCE_AUDITOR','STAFF_ENGINEER') AND p.code = 'notifications:read'
ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM roles r CROSS JOIN permissions p
WHERE r.code IN ('ADMIN','STAFF_ENGINEER') AND p.code = 'notifications:retry'
ON CONFLICT DO NOTHING;
ALTER TABLE audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_action_check CHECK(action IN (
'login_succeeded','login_failed','logout','user_invited','invitation_resent','invitation_accepted',
'roles_changed','user_deactivated','user_activated','user_deleted','password_reset_requested','password_reset_completed',
'mfa_enrolled','mfa_verified','mfa_failed','mfa_recovery_used','mfa_recovery_regenerated','mfa_replacement_started','mfa_replaced','sessions_revoked',
'statement_imported','statement_matching_configured','statement_match_confirmed','statement_match_rejected','statement_match_reset','statement_exception_noted',
'outbox_replay','notification_retry'));
