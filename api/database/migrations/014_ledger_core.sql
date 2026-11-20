-- Ledger core (ProjectFeature.md §2 and §4): chart of accounts, append-only
-- double-entry journals, idempotency records and the transactional outbox.
-- Amounts are BIGINT minor units (JPY scale 0, USD/EUR scale 2). Balances are
-- never stored: they are derived from postings.

CREATE TABLE accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_number TEXT NOT NULL UNIQUE CHECK (account_number ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'),
    name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 255),
    type TEXT NOT NULL CHECK (type IN ('ASSET', 'LIABILITY', 'EQUITY', 'REVENUE', 'EXPENSE')),
    currency TEXT NOT NULL CHECK (currency IN ('JPY', 'USD', 'EUR')),
    -- Without overdraft, a posting may not take the balance below zero.
    overdraft_allowed BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- created_by is a snapshot without a foreign key: journals are immutable, and
-- deleting a user must not need to update them.
CREATE TABLE journal_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    reference_id TEXT NOT NULL CHECK (length(btrim(reference_id)) BETWEEN 1 AND 128),
    description TEXT NOT NULL CHECK (length(btrim(description)) BETWEEN 1 AND 500),
    currency TEXT NOT NULL CHECK (currency IN ('JPY', 'USD', 'EUR')),
    posted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reversal_for_id UUID UNIQUE REFERENCES journal_entries(id) ON DELETE RESTRICT,
    created_by UUID,
    created_by_email TEXT CHECK (created_by_email IS NULL OR length(created_by_email) <= 254),
    CHECK (reversal_for_id IS NULL OR reversal_for_id <> id)
);
CREATE INDEX idx_journal_entries_posted ON journal_entries (posted_at DESC, id DESC);

CREATE TABLE ledger_postings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    journal_entry_id UUID NOT NULL REFERENCES journal_entries(id) ON DELETE RESTRICT,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    amount BIGINT NOT NULL CHECK (amount > 0),
    direction TEXT NOT NULL CHECK (direction IN ('DEBIT', 'CREDIT')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_postings_account_created ON ledger_postings (account_id, created_at DESC, id DESC);
CREATE INDEX idx_postings_journal ON ledger_postings (journal_entry_id);

CREATE TABLE idempotency_records (
    client_id TEXT NOT NULL CHECK (length(client_id) BETWEEN 1 AND 128),
    key TEXT NOT NULL CHECK (length(key) BETWEEN 1 AND 255),
    request_hash TEXT NOT NULL CHECK (request_hash ~ '^[a-f0-9]{64}$'),
    status_code INT CHECK (status_code BETWEEN 100 AND 599),
    response_body JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (client_id, key),
    CHECK ((status_code IS NULL) = (response_body IS NULL))
);

CREATE TABLE outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type TEXT NOT NULL DEFAULT 'JOURNAL',
    aggregate_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'PUBLISHED', 'FAILED')),
    retry_count INT NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ
);
CREATE INDEX idx_outbox_pending ON outbox_events (status, created_at) WHERE status = 'PENDING';

-- ---------------------------------------------------------------------------
-- Invariants enforced by the database itself
-- ---------------------------------------------------------------------------

-- 1. Journals and postings are append-only.
CREATE FUNCTION ledger_append_only() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% is append-only (% is not allowed)', TG_TABLE_NAME, TG_OP
        USING ERRCODE = 'insufficient_privilege';
END;
$$;
CREATE TRIGGER journal_entries_append_only BEFORE UPDATE OR DELETE ON journal_entries
    FOR EACH ROW EXECUTE FUNCTION ledger_append_only();
CREATE TRIGGER journal_entries_no_truncate BEFORE TRUNCATE ON journal_entries
    FOR EACH STATEMENT EXECUTE FUNCTION ledger_append_only();
CREATE TRIGGER ledger_postings_append_only BEFORE UPDATE OR DELETE ON ledger_postings
    FOR EACH ROW EXECUTE FUNCTION ledger_append_only();
CREATE TRIGGER ledger_postings_no_truncate BEFORE TRUNCATE ON ledger_postings
    FOR EACH STATEMENT EXECUTE FUNCTION ledger_append_only();

-- 2. Postings may only be added to a journal created in the same transaction,
--    so a committed journal can never gain postings later.
CREATE FUNCTION ledger_posting_same_transaction() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    journal_xmin xid;
BEGIN
    SELECT xmin INTO journal_xmin FROM journal_entries WHERE id = NEW.journal_entry_id;
    IF journal_xmin IS DISTINCT FROM pg_current_xact_id()::xid THEN
        RAISE EXCEPTION 'postings can only be added to a journal created in the same transaction'
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER ledger_postings_same_transaction BEFORE INSERT ON ledger_postings
    FOR EACH ROW EXECUTE FUNCTION ledger_posting_same_transaction();

-- 3. At commit, every journal has >= 2 postings, debits equal credits, every
--    account uses the journal currency, and a reversal mirrors its original.
CREATE FUNCTION ledger_check_journal() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    jid UUID;
    jcurrency TEXT;
    original UUID;
    posting_count INT;
    debits NUMERIC;
    credits NUMERIC;
BEGIN
    IF TG_TABLE_NAME = 'journal_entries' THEN
        jid := NEW.id;
    ELSE
        jid := NEW.journal_entry_id;
    END IF;
    SELECT currency, reversal_for_id INTO jcurrency, original FROM journal_entries WHERE id = jid;

    SELECT count(*),
           COALESCE(SUM(amount) FILTER (WHERE direction = 'DEBIT'), 0),
           COALESCE(SUM(amount) FILTER (WHERE direction = 'CREDIT'), 0)
      INTO posting_count, debits, credits
      FROM ledger_postings WHERE journal_entry_id = jid;
    IF posting_count < 2 THEN
        RAISE EXCEPTION 'journal % has % posting(s); at least 2 are required', jid, posting_count
            USING ERRCODE = 'check_violation';
    END IF;
    IF debits <> credits THEN
        RAISE EXCEPTION 'journal % is unbalanced: debits % <> credits %', jid, debits, credits
            USING ERRCODE = 'check_violation';
    END IF;
    IF EXISTS (SELECT 1 FROM ledger_postings p JOIN accounts a ON a.id = p.account_id
               WHERE p.journal_entry_id = jid AND a.currency <> jcurrency) THEN
        RAISE EXCEPTION 'journal % posts to an account in another currency', jid
            USING ERRCODE = 'check_violation';
    END IF;
    IF original IS NOT NULL THEN
        IF (SELECT currency FROM journal_entries WHERE id = original) <> jcurrency
           OR EXISTS (SELECT account_id, amount, CASE direction WHEN 'DEBIT' THEN 'CREDIT' ELSE 'DEBIT' END
                        FROM ledger_postings WHERE journal_entry_id = original
                      EXCEPT ALL
                      SELECT account_id, amount, direction FROM ledger_postings WHERE journal_entry_id = jid)
           OR EXISTS (SELECT account_id, amount, direction FROM ledger_postings WHERE journal_entry_id = jid
                      EXCEPT ALL
                      SELECT account_id, amount, CASE direction WHEN 'DEBIT' THEN 'CREDIT' ELSE 'DEBIT' END
                        FROM ledger_postings WHERE journal_entry_id = original) THEN
            RAISE EXCEPTION 'reversal % does not mirror journal %', jid, original
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;
    RETURN NULL;
END;
$$;
CREATE CONSTRAINT TRIGGER journal_entries_balanced AFTER INSERT ON journal_entries
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION ledger_check_journal();
CREATE CONSTRAINT TRIGGER ledger_postings_balanced AFTER INSERT ON ledger_postings
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION ledger_check_journal();

-- 4. An idempotency claim cannot commit without its final response.
CREATE FUNCTION idempotency_record_complete() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM idempotency_records
               WHERE client_id = NEW.client_id AND key = NEW.key AND status_code IS NULL) THEN
        RAISE EXCEPTION 'idempotency record %/% committed without a response', NEW.client_id, NEW.key
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NULL;
END;
$$;
CREATE CONSTRAINT TRIGGER idempotency_records_complete AFTER INSERT OR UPDATE ON idempotency_records
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION idempotency_record_complete();

-- ---------------------------------------------------------------------------
-- Permission to manage the chart of accounts
-- ---------------------------------------------------------------------------
INSERT INTO permissions (code, name, description)
VALUES ('accounts:write', 'Manage Accounts', 'Create ledger accounts in the chart of accounts')
ON CONFLICT (code) DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.code IN ('ADMIN', 'LEAD_OPERATOR') AND p.code = 'accounts:write'
ON CONFLICT DO NOTHING;
