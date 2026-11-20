ALTER TABLE statement_imports
 ADD COLUMN direction_mode TEXT CHECK(direction_mode IN ('SAME','OPPOSITE')),
 ADD COLUMN matching_version INTEGER NOT NULL DEFAULT 0 CHECK(matching_version>=0);

-- Reviewer UUIDs are immutable actor snapshots so deleting a user preserves
-- reconciliation history. Each posting can be reconciled only once globally.
CREATE TABLE statement_matches (
 import_id UUID NOT NULL,
 row_number INTEGER NOT NULL,
 posting_id UUID NOT NULL UNIQUE REFERENCES ledger_postings(id),
 matched_by UUID NOT NULL,
 matched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(import_id,row_number),
 FOREIGN KEY(import_id,row_number) REFERENCES statement_rows(import_id,row_number)
);
CREATE TABLE statement_rejections (
 import_id UUID NOT NULL,
 row_number INTEGER NOT NULL,
 posting_id UUID NOT NULL REFERENCES ledger_postings(id),
 rejected_by UUID NOT NULL,
 reason TEXT NOT NULL CHECK(length(btrim(reason)) BETWEEN 3 AND 500),
 rejected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(import_id,row_number,posting_id),
 FOREIGN KEY(import_id,row_number) REFERENCES statement_rows(import_id,row_number)
);
CREATE TABLE statement_row_notes (
 import_id UUID NOT NULL,
 row_number INTEGER NOT NULL,
 note TEXT NOT NULL CHECK(length(btrim(note)) BETWEEN 3 AND 500),
 updated_by UUID NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(import_id,row_number),
 FOREIGN KEY(import_id,row_number) REFERENCES statement_rows(import_id,row_number)
);
CREATE INDEX idx_matching_postings ON ledger_postings(account_id,amount,direction,journal_entry_id);
CREATE INDEX idx_matching_reference ON journal_entries(reference_id,currency);

INSERT INTO permissions(code,name,description) VALUES
 ('reconciliation:match','Review Own Statement Matches','Configure matching, confirm or reject exact candidates and record exception notes')
ON CONFLICT(code) DO NOTHING;
INSERT INTO role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM roles r CROSS JOIN permissions p
WHERE r.code IN ('ADMIN','OPERATOR','LEAD_OPERATOR') AND p.code='reconciliation:match'
ON CONFLICT DO NOTHING;
ALTER TABLE audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_action_check CHECK(action IN (
 'login_succeeded','login_failed','logout','user_invited','invitation_resent','invitation_accepted',
 'roles_changed','user_deactivated','user_activated','user_deleted','password_reset_requested','password_reset_completed',
 'mfa_enrolled','mfa_verified','mfa_failed','mfa_recovery_used',
 'mfa_recovery_regenerated','mfa_replacement_started','mfa_replaced','sessions_revoked','statement_imported',
 'statement_matching_configured','statement_match_confirmed','statement_match_rejected','statement_match_reset','statement_exception_noted'));
