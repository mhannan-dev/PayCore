-- Financial import history is retained even if the uploader is deleted. The
-- owner UUID is an immutable actor snapshot, like journal creator snapshots.
ALTER TABLE accounts ADD CONSTRAINT accounts_id_currency_key UNIQUE(id,currency);
CREATE TABLE statement_imports (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 owner_user_id UUID NOT NULL,
 attachment_id UUID REFERENCES attachments(id) ON DELETE SET NULL,
 account_id UUID NOT NULL,
 currency TEXT NOT NULL CHECK(currency IN ('JPY','USD','EUR')),
 file_name TEXT NOT NULL CHECK(length(file_name) BETWEEN 1 AND 255),
 content_hash TEXT NOT NULL CHECK(content_hash ~ '^[a-f0-9]{64}$'),
 source_sha256 TEXT NOT NULL CHECK(source_sha256 ~ '^[a-f0-9]{64}$'),
 row_count INTEGER NOT NULL CHECK(row_count BETWEEN 1 AND 2000),
 total_debits NUMERIC(40,0) NOT NULL CHECK(total_debits>=0),
 total_credits NUMERIC(40,0) NOT NULL CHECK(total_credits>=0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(account_id,currency) REFERENCES accounts(id,currency),
 CONSTRAINT statement_imports_duplicate_key UNIQUE(owner_user_id,account_id,content_hash)
);
CREATE INDEX statement_imports_owner_idx ON statement_imports(owner_user_id,created_at DESC,id DESC);
CREATE TABLE statement_rows (
 import_id UUID NOT NULL REFERENCES statement_imports(id),
 row_number INTEGER NOT NULL CHECK(row_number>0),
 booked_date DATE NOT NULL CHECK(booked_date>='1900-01-01' AND booked_date<='9999-12-31'),
 reference TEXT NOT NULL CHECK(length(reference) BETWEEN 1 AND 128),
 amount BIGINT NOT NULL CHECK(amount>0),
 direction TEXT NOT NULL CHECK(direction IN ('DEBIT','CREDIT')),
 description TEXT NOT NULL CHECK(length(description)<=500),
 PRIMARY KEY(import_id,row_number)
);
INSERT INTO permissions(code,name,description) VALUES
 ('reconciliation:read','Read Own Statements','View statements imported by the signed-in user'),
 ('reconciliation:import','Import Own Statements','Validate and import owned CSV statements without posting ledger entries')
ON CONFLICT(code) DO NOTHING;
INSERT INTO role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM roles r CROSS JOIN permissions p
WHERE (r.code IN ('ADMIN','OPERATOR','LEAD_OPERATOR','COMPLIANCE_AUDITOR') AND p.code='reconciliation:read')
 OR (r.code IN ('ADMIN','OPERATOR','LEAD_OPERATOR') AND p.code='reconciliation:import')
ON CONFLICT DO NOTHING;
ALTER TABLE audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_action_check CHECK(action IN (
 'login_succeeded','login_failed','logout','user_invited','invitation_resent','invitation_accepted',
 'roles_changed','user_deactivated','user_activated','user_deleted','password_reset_requested','password_reset_completed',
 'mfa_enrolled','mfa_verified','mfa_failed','mfa_recovery_used',
 'mfa_recovery_regenerated','mfa_replacement_started','mfa_replaced','sessions_revoked','statement_imported'));
