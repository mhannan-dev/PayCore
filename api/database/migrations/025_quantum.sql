CREATE TABLE quantum_merchants (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(), owner_user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE RESTRICT,
 display_name TEXT NOT NULL CHECK(length(btrim(display_name)) BETWEEN 1 AND 120),
 receiver_address TEXT NOT NULL CHECK(receiver_address ~ '^0x[0-9a-f]{40}$' AND receiver_address <> '0x0000000000000000000000000000000000000000'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE quantum_invoices (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),merchant_id UUID NOT NULL REFERENCES quantum_merchants(id) ON DELETE RESTRICT,
 checkout_token TEXT NOT NULL UNIQUE CHECK(checkout_token ~ '^[A-Za-z0-9_-]{43}$'),
 description TEXT NOT NULL CHECK(length(btrim(description)) BETWEEN 1 AND 500),amount_base_units BIGINT NOT NULL CHECK(amount_base_units>0),
 chain_id BIGINT NOT NULL CHECK(chain_id>0 AND chain_id<=9007199254740991 AND chain_id NOT IN(1,10,56,137,8453,42161,44002)),
 token_address TEXT NOT NULL CHECK(token_address ~ '^0x[0-9a-f]{40}$'),receiver_address TEXT NOT NULL CHECK(receiver_address ~ '^0x[0-9a-f]{40}$'),
 token_decimals INTEGER NOT NULL CHECK(token_decimals BETWEEN 0 AND 18),explorer_url TEXT NOT NULL,checkout_url TEXT NOT NULL,
 status TEXT NOT NULL DEFAULT 'PENDING' CHECK(status IN('PENDING','PAID','EXPIRED','CANCELLED')),
 expires_at TIMESTAMPTZ NOT NULL,paid_at TIMESTAMPTZ,paid_tx_hash TEXT CHECK(paid_tx_hash ~ '^0x[0-9a-f]{64}$'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),version BIGINT NOT NULL DEFAULT 1 CHECK(version>0),
 CHECK(expires_at>created_at AND expires_at<=created_at+INTERVAL '24 hours'),CHECK((status='PAID' AND paid_at IS NOT NULL AND paid_tx_hash IS NOT NULL) OR (status<>'PAID' AND paid_at IS NULL AND paid_tx_hash IS NULL))
);
CREATE INDEX quantum_invoice_merchant_created ON quantum_invoices(merchant_id,created_at DESC,id DESC);
CREATE INDEX quantum_invoice_expiry ON quantum_invoices(status,expires_at);
CREATE TABLE quantum_payment_attempts (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),invoice_id UUID NOT NULL REFERENCES quantum_invoices(id) ON DELETE RESTRICT,
 payer_address TEXT NOT NULL CHECK(payer_address ~ '^0x[0-9a-f]{40}$'),
 tx_hash TEXT CHECK(tx_hash ~ '^0x[0-9a-f]{64}$'),
 status TEXT NOT NULL DEFAULT 'CREATED' CHECK(status IN('CREATED','BROADCAST','VERIFYING','CONFIRMED','FAILED','REORGED','REVIEW_REQUIRED')),
 failure_code TEXT CHECK(failure_code IN('wrong_chain','wrong_token','wrong_recipient','wrong_payer','underpaid','overpaid','late_payment','tx_failed','tx_not_found','duplicate_transfer')),
 confirmations BIGINT NOT NULL DEFAULT 0 CHECK(confirmations>=0),observed_block BIGINT,observed_hash TEXT CHECK(observed_hash ~ '^0x[0-9a-f]{64}$'),
 next_verify_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),lease_id TEXT,lease_until TIMESTAMPTZ,rpc_failures INTEGER NOT NULL DEFAULT 0 CHECK(rpc_failures>=0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 CHECK((lease_id IS NULL)=(lease_until IS NULL)),CHECK(status='CREATED' OR tx_hash IS NOT NULL),
 CHECK((status IN('FAILED','REVIEW_REQUIRED'))=(failure_code IS NOT NULL))
);
CREATE UNIQUE INDEX quantum_open_payer ON quantum_payment_attempts(invoice_id,payer_address) WHERE status IN('CREATED','BROADCAST','VERIFYING');
CREATE INDEX quantum_attempt_tx ON quantum_payment_attempts(tx_hash);
CREATE INDEX quantum_attempt_due ON quantum_payment_attempts(next_verify_at,created_at) WHERE status IN('BROADCAST','VERIFYING','CONFIRMED','REORGED');
CREATE TABLE quantum_transfer_claims (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),invoice_id UUID NOT NULL REFERENCES quantum_invoices(id) ON DELETE RESTRICT,
 attempt_id UUID NOT NULL REFERENCES quantum_payment_attempts(id) ON DELETE RESTRICT,
 chain_id BIGINT NOT NULL CHECK(chain_id>0),tx_hash TEXT NOT NULL CHECK(tx_hash ~ '^0x[0-9a-f]{64}$'),log_index BIGINT NOT NULL CHECK(log_index>=0),
 token_address TEXT NOT NULL CHECK(token_address ~ '^0x[0-9a-f]{40}$'),from_address TEXT NOT NULL CHECK(from_address ~ '^0x[0-9a-f]{40}$'),to_address TEXT NOT NULL CHECK(to_address ~ '^0x[0-9a-f]{40}$'),
 amount_base_units BIGINT NOT NULL CHECK(amount_base_units>0),block_number BIGINT NOT NULL CHECK(block_number>=0),block_hash TEXT NOT NULL CHECK(block_hash ~ '^0x[0-9a-f]{64}$'),
 canonical BOOLEAN NOT NULL DEFAULT true,created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),UNIQUE(chain_id,tx_hash,log_index)
);
CREATE UNIQUE INDEX quantum_one_canonical_payment ON quantum_transfer_claims(invoice_id) WHERE canonical;
INSERT INTO permissions(code,name,description) VALUES('quantum:read','Read Own Quantum Invoices','Inspect owned testnet merchant invoices'),('quantum:write','Manage Own Quantum Invoices','Create and cancel owned testnet invoices') ON CONFLICT(code) DO NOTHING;
INSERT INTO role_permissions(role_id,permission_id) SELECT r.id,p.id FROM roles r CROSS JOIN permissions p WHERE r.code='ADMIN' AND p.code IN('quantum:read','quantum:write') ON CONFLICT DO NOTHING;
ALTER TABLE audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_action_check CHECK(action IN(
'login_succeeded','login_failed','logout','user_invited','invitation_resent','invitation_accepted','roles_changed','user_deactivated','user_activated','user_deleted','password_reset_requested','password_reset_completed','mfa_enrolled','mfa_verified','mfa_failed','mfa_recovery_used','mfa_recovery_regenerated','mfa_replacement_started','mfa_replaced','sessions_revoked','statement_imported','statement_matching_configured','statement_match_confirmed','statement_match_rejected','statement_match_reset','statement_exception_noted','outbox_replay','notification_retry','quantum_payment_reorg'));
