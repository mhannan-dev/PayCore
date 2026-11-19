-- Append-only audit trail. User IDs and emails are snapshots, deliberately without
-- foreign keys: deleting a user must never change or remove their history.
CREATE TABLE audit_events (
    id BIGSERIAL PRIMARY KEY,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    action TEXT NOT NULL CHECK (action IN (
        'login_succeeded', 'login_failed', 'logout',
        'user_invited', 'invitation_resent', 'invitation_accepted',
        'roles_changed', 'user_deactivated', 'user_activated', 'user_deleted',
        'password_reset_requested', 'password_reset_completed')),
    actor_user_id UUID,
    actor_email TEXT CHECK (actor_email IS NULL OR length(actor_email) <= 254),
    target_user_id UUID,
    target_email TEXT CHECK (target_email IS NULL OR length(target_email) <= 254),
    details JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(details) = 'object'),
    ip TEXT CHECK (ip IS NULL OR length(ip) <= 64),
    user_agent TEXT CHECK (user_agent IS NULL OR length(user_agent) <= 300)
);
CREATE INDEX idx_audit_events_occurred ON audit_events (occurred_at DESC, id DESC);
CREATE INDEX idx_audit_events_action ON audit_events (action, occurred_at DESC);
CREATE INDEX idx_audit_events_actor ON audit_events (actor_user_id, occurred_at DESC);
CREATE INDEX idx_audit_events_target ON audit_events (target_user_id, occurred_at DESC);

-- The application may only INSERT and SELECT: history cannot be rewritten.
CREATE FUNCTION audit_events_append_only() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_events is append-only (% is not allowed)', TG_OP
        USING ERRCODE = 'insufficient_privilege';
END;
$$;
CREATE TRIGGER audit_events_no_update_delete
    BEFORE UPDATE OR DELETE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION audit_events_append_only();
CREATE TRIGGER audit_events_no_truncate
    BEFORE TRUNCATE ON audit_events
    FOR EACH STATEMENT EXECUTE FUNCTION audit_events_append_only();
