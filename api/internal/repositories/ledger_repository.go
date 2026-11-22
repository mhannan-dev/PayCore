package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"paycore/api/internal/models"
)

// Ledger errors. Not-found errors wrap pgx.ErrNoRows.
var (
	ErrAccountNotFound     = fmt.Errorf("account not found: %w", pgx.ErrNoRows)
	ErrJournalNotFound     = fmt.Errorf("journal not found: %w", pgx.ErrNoRows)
	ErrAccountNumberTaken  = errors.New("account number already exists")
	ErrSystemRoleTaken     = errors.New("system role already assigned for this currency")
	ErrAccountInactive     = errors.New("account is inactive")
	ErrCurrencyMismatch    = errors.New("account currency does not match the journal currency")
	ErrAlreadyReversed     = errors.New("journal has already been reversed")
	ErrIdempotencyMismatch = errors.New("idempotency key was used with a different request")
	ErrIdempotencyInFlight = errors.New("a request with this idempotency key is still in progress")
	ErrLedgerBusy          = errors.New("accounts are locked by other transfers")
)

// InsufficientFundsError: posting would take a non-overdraft account below zero.
type InsufficientFundsError struct {
	AccountNumber string
	Balance       string // current balance, minor units
	Change        string // signed change this journal would apply
}

func (e InsufficientFundsError) Error() string {
	return "insufficient funds in account " + e.AccountNumber + ": balance " + e.Balance + ", change " + e.Change
}

const (
	claimLockTimeout   = "5s"  // wait for a concurrent request with the same idempotency key
	accountLockTimeout = "10s" // wait for other transfers holding the same accounts
	maxTxAttempts      = 3     // whole-transaction retries for deadlock / serialization failures
)

// LedgerRepository stores the chart of accounts and the append-only journal.
type LedgerRepository struct{ pool *pgxpool.Pool }

func NewLedgerRepository(pool *pgxpool.Pool) *LedgerRepository { return &LedgerRepository{pool: pool} }

// accountColumns selects an account with its derived balance (normal direction).
const accountColumns = `a.id, a.account_number, a.name, a.type, a.currency, a.overdraft_allowed, a.is_active, a.system_role, a.created_at,
	((CASE WHEN a.type IN ('ASSET', 'EXPENSE') THEN 1 ELSE -1 END) *
	  COALESCE((SELECT SUM(CASE p.direction WHEN 'DEBIT' THEN p.amount ELSE -p.amount END)
	            FROM ledger_postings p WHERE p.account_id = a.id), 0))::text`

func scanAccount(row pgx.Row) (a models.Account, err error) {
	var typ, currency string
	var role *string
	err = row.Scan(&a.ID, &a.AccountNumber, &a.Name, &typ, &currency, &a.OverdraftAllowed, &a.IsActive, &role, &a.CreatedAt, &a.Balance)
	a.Type, a.Currency, a.SystemRole = models.AccountType(typ), models.Currency(currency), systemRole(role)
	return
}

// CreateAccount adds an account; createdBy may be "".
func (r *LedgerRepository) CreateAccount(ctx context.Context, req models.CreateAccountRequest, createdBy string) (models.Account, error) {
	var id string
	err := r.pool.QueryRow(ctx, `INSERT INTO accounts (account_number, name, type, currency, overdraft_allowed, system_role, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		req.AccountNumber, req.Name, string(req.Type), string(req.Currency), req.OverdraftAllowed, req.SystemRole, nullableUUID(createdBy)).Scan(&id)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		if pgErr.ConstraintName == "accounts_currency_system_role_key" {
			return models.Account{}, ErrSystemRoleTaken
		}
		return models.Account{}, ErrAccountNumberTaken
	}
	if err != nil {
		return models.Account{}, err
	}
	return r.GetAccount(ctx, id)
}

// systemRole converts a nullable system_role column.
func systemRole(raw *string) *models.AccountSystemRole {
	if raw == nil {
		return nil
	}
	role := models.AccountSystemRole(*raw)
	return &role
}

// GetAccount returns one account with its balance.
func (r *LedgerRepository) GetAccount(ctx context.Context, id string) (models.Account, error) {
	a, err := scanAccount(r.pool.QueryRow(ctx, "SELECT "+accountColumns+" FROM accounts a WHERE a.id = $1", id))
	return a, notFound(err, ErrAccountNotFound)
}

// ListAccounts returns one page of accounts ordered by account number.
func (r *LedgerRepository) ListAccounts(ctx context.Context, q models.AccountQuery) ([]models.Account, int, error) {
	pattern := likePattern(q.Search)
	const where = ` WHERE ($1 = '%%' OR a.account_number ILIKE $1 OR a.name ILIKE $1)
		AND ($2 = '' OR a.type = $2) AND ($3 = '' OR a.currency = $3)`
	args := []any{pattern, string(q.Type), string(q.Currency)}
	var total int
	if err := r.pool.QueryRow(ctx, "SELECT count(*) FROM accounts a"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, "SELECT "+accountColumns+" FROM accounts a"+where+" ORDER BY a.account_number LIMIT $4 OFFSET $5", append(args, q.Limit, q.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	accounts := []models.Account{}
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, 0, err
		}
		accounts = append(accounts, a)
	}
	return accounts, total, rows.Err()
}

func likePattern(search string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(strings.TrimSpace(search)) + "%"
}

const postingColumns = `p.id, p.journal_entry_id, p.account_id, a.account_number, a.name, p.direction, p.amount, p.created_at`

func scanPosting(row pgx.Row, extra ...any) (p models.Posting, err error) {
	var direction string
	var amount int64
	dest := append([]any{&p.ID, &p.JournalID, &p.AccountID, &p.AccountNumber, &p.AccountName, &direction, &amount, &p.CreatedAt}, extra...)
	err = row.Scan(dest...)
	p.Direction, p.Amount = models.Direction(direction), models.Amount(amount)
	return
}

// ListAccountPostings returns one page of an account's postings, newest first.
func (r *LedgerRepository) ListAccountPostings(ctx context.Context, accountID string, limit, offset int) ([]models.AccountPosting, int, error) {
	var total int
	var exists bool
	if err := r.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM accounts WHERE id = $1), (SELECT count(*) FROM ledger_postings WHERE account_id = $1)", accountID).Scan(&exists, &total); err != nil {
		return nil, 0, err
	}
	if !exists {
		return nil, 0, ErrAccountNotFound
	}
	rows, err := r.pool.Query(ctx, "SELECT "+postingColumns+`, j.reference_id, j.description, j.reversal_for_id
		FROM ledger_postings p JOIN accounts a ON a.id = p.account_id JOIN journal_entries j ON j.id = p.journal_entry_id
		WHERE p.account_id = $1 ORDER BY p.created_at DESC, p.id DESC LIMIT $2 OFFSET $3`, accountID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []models.AccountPosting{}
	for rows.Next() {
		var ap models.AccountPosting
		p, err := scanPosting(rows, &ap.ReferenceID, &ap.Description, &ap.ReversalFor)
		if err != nil {
			return nil, 0, err
		}
		ap.Posting = p
		out = append(out, ap)
	}
	return out, total, rows.Err()
}

const journalColumns = `j.id, j.reference_id, j.description, j.currency, j.posted_at, j.reversal_for_id,
	(SELECT r.id FROM journal_entries r WHERE r.reversal_for_id = j.id), j.created_by_email`

func scanJournal(row pgx.Row) (j models.Journal, err error) {
	var currency string
	err = row.Scan(&j.ID, &j.ReferenceID, &j.Description, &currency, &j.PostedAt, &j.ReversalForID, &j.ReversedByID, &j.CreatedByEmail)
	j.Currency = models.Currency(currency)
	return
}

type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// attachPostings loads the postings of journals (in place) and sets totals.
func attachPostings(ctx context.Context, db querier, journals []models.Journal) error {
	if len(journals) == 0 {
		return nil
	}
	ids := make([]string, len(journals))
	index := make(map[string]int, len(journals))
	for i, j := range journals {
		ids[i] = j.ID
		index[j.ID] = i
		journals[i].Postings = []models.Posting{}
	}
	rows, err := db.Query(ctx, "SELECT "+postingColumns+` FROM ledger_postings p JOIN accounts a ON a.id = p.account_id
		WHERE p.journal_entry_id = ANY($1) ORDER BY p.journal_entry_id, p.created_at, p.id`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		p, err := scanPosting(rows)
		if err != nil {
			return err
		}
		j := &journals[index[p.JournalID]]
		j.Postings = append(j.Postings, p)
		if p.Direction == models.Debit {
			j.Total += p.Amount
		}
	}
	return rows.Err()
}

// GetJournal returns one journal with its postings.
func (r *LedgerRepository) GetJournal(ctx context.Context, id string) (models.Journal, error) {
	j, err := scanJournal(r.pool.QueryRow(ctx, "SELECT "+journalColumns+" FROM journal_entries j WHERE j.id = $1", id))
	if err != nil {
		return models.Journal{}, notFound(err, ErrJournalNotFound)
	}
	list := []models.Journal{j}
	if err := attachPostings(ctx, r.pool, list); err != nil {
		return models.Journal{}, err
	}
	return list[0], nil
}

// ListJournals returns one page of journals, newest first.
func (r *LedgerRepository) ListJournals(ctx context.Context, q models.JournalQuery) ([]models.Journal, int, error) {
	pattern := likePattern(q.Search)
	const where = ` WHERE ($1 = '%%' OR j.reference_id ILIKE $1 OR j.description ILIKE $1)
		AND ($2 = '' OR EXISTS (SELECT 1 FROM ledger_postings p WHERE p.journal_entry_id = j.id AND p.account_id::text = $2))
		AND ($3 = '' OR j.currency = $3)`
	var total int
	if err := r.pool.QueryRow(ctx, "SELECT count(*) FROM journal_entries j"+where, pattern, q.AccountID, string(q.Currency)).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, "SELECT "+journalColumns+" FROM journal_entries j"+where+" ORDER BY j.posted_at DESC, j.id DESC LIMIT $4 OFFSET $5", pattern, q.AccountID, string(q.Currency), q.Limit, q.Offset)
	if err != nil {
		return nil, 0, err
	}
	journals := []models.Journal{}
	for rows.Next() {
		j, err := scanJournal(rows)
		if err != nil {
			rows.Close()
			return nil, 0, err
		}
		journals = append(journals, j)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return journals, total, attachPostings(ctx, r.pool, journals)
}

// ZeroSum checks the double-entry invariant across all journals in a currency.
// One statement keeps totals, counts and findings on the same PostgreSQL snapshot,
// even when transfers commit while the audit runs.
func (r *LedgerRepository) ZeroSum(ctx context.Context, currency models.Currency) (models.ZeroSumReport, error) {
	report := models.ZeroSumReport{Currency: currency, UnbalancedJournals: []string{}}
	err := r.pool.QueryRow(ctx, `
		WITH totals AS (
			SELECT
				COALESCE(SUM(p.amount) FILTER (WHERE p.direction = 'DEBIT'), 0)::text AS debits,
				COALESCE(SUM(p.amount) FILTER (WHERE p.direction = 'CREDIT'), 0)::text AS credits,
				(COALESCE(SUM(p.amount) FILTER (WHERE p.direction = 'DEBIT'), 0) - COALESCE(SUM(p.amount) FILTER (WHERE p.direction = 'CREDIT'), 0))::text AS difference,
				count(DISTINCT j.id) AS journals,
				count(p.id) AS postings
			FROM journal_entries j JOIN ledger_postings p ON p.journal_entry_id = j.id
			WHERE j.currency = $1
		), unbalanced AS (
			SELECT j.id
			FROM journal_entries j LEFT JOIN ledger_postings p ON p.journal_entry_id = j.id
			WHERE j.currency = $1
			GROUP BY j.id
			HAVING count(p.id) < 2
			    OR COALESCE(SUM(p.amount) FILTER (WHERE p.direction = 'DEBIT'), 0) <> COALESCE(SUM(p.amount) FILTER (WHERE p.direction = 'CREDIT'), 0)
			LIMIT 100
		)
		SELECT t.debits, t.credits, t.difference, t.journals, t.postings, NOW(),
			COALESCE((SELECT array_agg(id::text ORDER BY id) FROM unbalanced), '{}'::text[])
		FROM totals t`, string(currency)).
		Scan(&report.TotalDebits, &report.TotalCredits, &report.Difference, &report.Journals, &report.Postings, &report.CheckedAt, &report.UnbalancedJournals)
	if err != nil {
		return report, err
	}
	if report.UnbalancedJournals == nil {
		report.UnbalancedJournals = []string{}
	}
	report.Balanced = report.Difference == "0" && len(report.UnbalancedJournals) == 0
	return report, nil
}

// --- Writes ---------------------------------------------------------------------

// Idempotent runs op in one transaction together with the idempotency claim for
// (clientID, key), and stores op's response before committing. A repeated key
// with the same requestHash returns the stored response (Replayed); a different
// hash returns ErrIdempotencyMismatch; a key still held by an uncommitted request
// for longer than the claim timeout returns ErrIdempotencyInFlight. Deadlocks and
// serialization failures retry the whole transaction, keeping the key.
func (r *LedgerRepository) Idempotent(ctx context.Context, clientID, key, requestHash string,
	op func(ctx context.Context, w models.LedgerWriter) (int, []byte, error)) (models.IdempotentResult, error) {
	return runIdempotent(ctx, r.pool, clientID, key, requestHash, func(ctx context.Context, tx pgx.Tx) (int, []byte, error) {
		return op(ctx, &ledgerWriter{tx: tx})
	})
}

// runIdempotent also lets payouts commit their state in the ledger transaction.
func runIdempotent(ctx context.Context, pool *pgxpool.Pool, clientID, key, requestHash string,
	op func(context.Context, pgx.Tx) (int, []byte, error)) (models.IdempotentResult, error) {
	var err error
	for attempt := 0; attempt < maxTxAttempts; attempt++ {
		var result models.IdempotentResult
		result, err = idempotentOnce(ctx, pool, clientID, key, requestHash, op)
		if !retryable(err) {
			return result, err
		}
	}
	return models.IdempotentResult{}, ErrLedgerBusy
}

func idempotentOnce(ctx context.Context, pool *pgxpool.Pool, clientID, key, requestHash string,
	op func(context.Context, pgx.Tx) (int, []byte, error)) (models.IdempotentResult, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return models.IdempotentResult{}, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))

	if _, err := tx.Exec(ctx, "SELECT set_config('lock_timeout', $1, true)", claimLockTimeout); err != nil {
		return models.IdempotentResult{}, err
	}
	// Concurrent claims of the same key wait here on the primary key until the
	// first transaction commits (then we replay) or rolls back (then we own it).
	tag, err := tx.Exec(ctx, `INSERT INTO idempotency_records (client_id, key, request_hash)
		VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, clientID, key, requestHash)
	if isLockTimeout(err) {
		return models.IdempotentResult{}, ErrIdempotencyInFlight
	}
	if err != nil {
		return models.IdempotentResult{}, err
	}
	if tag.RowsAffected() == 0 {
		var storedHash string
		var status *int
		var body []byte
		if err := tx.QueryRow(ctx, "SELECT request_hash, status_code, response_body FROM idempotency_records WHERE client_id = $1 AND key = $2",
			clientID, key).Scan(&storedHash, &status, &body); err != nil {
			return models.IdempotentResult{}, err
		}
		if storedHash != requestHash {
			return models.IdempotentResult{}, ErrIdempotencyMismatch
		}
		if status == nil {
			return models.IdempotentResult{}, ErrIdempotencyInFlight
		}
		return models.IdempotentResult{Status: *status, Body: body, Replayed: true}, nil
	}

	if _, err := tx.Exec(ctx, "SELECT set_config('lock_timeout', $1, true)", accountLockTimeout); err != nil {
		return models.IdempotentResult{}, err
	}
	status, body, err := op(ctx, tx)
	if isLockTimeout(err) {
		return models.IdempotentResult{}, ErrLedgerBusy
	}
	if err != nil {
		return models.IdempotentResult{}, err
	}
	if _, err := tx.Exec(ctx, "UPDATE idempotency_records SET status_code = $3, response_body = $4 WHERE client_id = $1 AND key = $2",
		clientID, key, status, body); err != nil {
		return models.IdempotentResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return models.IdempotentResult{}, err
	}
	return models.IdempotentResult{Status: status, Body: body}, nil
}

func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

// retryable: deadlock_detected or serialization_failure.
func retryable(err error) bool { c := pgCode(err); return c == "40P01" || c == "40001" }

// isLockTimeout: lock_not_available (lock_timeout expired).
func isLockTimeout(err error) bool { return pgCode(err) == "55P03" }

// ledgerWriter posts inside the Idempotent transaction.
type ledgerWriter struct{ tx pgx.Tx }

type lockedAccount struct {
	id, number, currency string
	typ                  models.AccountType
	overdraft, active    bool
}

// PostJournal locks every account in ascending ID order, checks they exist, are
// active and use the journal currency, checks overdraft limits against
// committed postings, then inserts the journal, its postings and an outbox
// event. Balance and mirroring are re-checked by the database at commit.
func (w *ledgerWriter) PostJournal(ctx context.Context, nj models.NewJournal) (models.Journal, error) {
	ids := make([]string, 0, len(nj.Legs))
	seen := map[string]bool{}
	for _, leg := range nj.Legs {
		if !seen[leg.AccountID] {
			seen[leg.AccountID] = true
			ids = append(ids, leg.AccountID)
		}
	}
	sort.Strings(ids) // canonical lowercase UUID text sorts like PostgreSQL's uuid order

	rows, err := w.tx.Query(ctx, `SELECT id, account_number, type, currency, overdraft_allowed, is_active
		FROM accounts WHERE id = ANY($1) ORDER BY id FOR UPDATE`, ids)
	if err != nil {
		return models.Journal{}, err
	}
	locked := map[string]lockedAccount{}
	for rows.Next() {
		var a lockedAccount
		var typ string
		if err := rows.Scan(&a.id, &a.number, &typ, &a.currency, &a.overdraft, &a.active); err != nil {
			rows.Close()
			return models.Journal{}, err
		}
		a.typ = models.AccountType(typ)
		locked[a.id] = a
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return models.Journal{}, err
	}
	for _, id := range ids {
		a, ok := locked[id]
		switch {
		case !ok:
			return models.Journal{}, ErrAccountNotFound
		case !a.active:
			return models.Journal{}, ErrAccountInactive
		case a.currency != string(nj.Currency):
			return models.Journal{}, ErrCurrencyMismatch
		}
	}

	if err := w.checkFunds(ctx, nj.Legs, locked); err != nil {
		return models.Journal{}, err
	}

	var j models.Journal
	err = w.tx.QueryRow(ctx, `INSERT INTO journal_entries (reference_id, description, currency, reversal_for_id, created_by, created_by_email)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id, posted_at`,
		nj.ReferenceID, nj.Description, string(nj.Currency), nj.ReversalForID, nullableUUID(nj.CreatedBy), nullableText(nj.CreatedByEmail)).Scan(&j.ID, &j.PostedAt)
	if pgCode(err) == "23505" {
		return models.Journal{}, ErrAlreadyReversed // unique reversal_for_id
	}
	if err != nil {
		return models.Journal{}, err
	}
	j.ReferenceID, j.Description, j.Currency, j.ReversalForID = nj.ReferenceID, nj.Description, nj.Currency, nj.ReversalForID
	j.CreatedByEmail = nullableTextPtr(nj.CreatedByEmail)
	j.Postings = make([]models.Posting, 0, len(nj.Legs))
	for _, leg := range nj.Legs {
		p := models.Posting{JournalID: j.ID, AccountID: leg.AccountID, AccountNumber: locked[leg.AccountID].number, Direction: leg.Direction, Amount: leg.Amount}
		if err := w.tx.QueryRow(ctx, `INSERT INTO ledger_postings (journal_entry_id, account_id, amount, direction)
			VALUES ($1, $2, $3, $4) RETURNING id, created_at`, j.ID, leg.AccountID, int64(leg.Amount), string(leg.Direction)).Scan(&p.ID, &p.CreatedAt); err != nil {
			return models.Journal{}, err
		}
		if leg.Direction == models.Debit {
			j.Total += leg.Amount
		}
		j.Postings = append(j.Postings, p)
	}
	if err := w.fillAccountNames(ctx, &j); err != nil {
		return models.Journal{}, err
	}

	eventType := "ledger.journal_posted.v1"
	if nj.ReversalForID != nil {
		eventType = "ledger.journal_reversed.v1"
	}
	payload, err := json.Marshal(j)
	if err != nil {
		return models.Journal{}, err
	}
	if _, err := w.tx.Exec(ctx, `INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload) VALUES ('JOURNAL', $1, $2, $3)`,
		j.ID, eventType, payload); err != nil {
		return models.Journal{}, err
	}
	return j, nil
}

// checkFunds rejects postings that would take a non-overdraft account below zero.
// It runs after the accounts are locked, so concurrent transfers cannot overspend.
func (w *ledgerWriter) checkFunds(ctx context.Context, legs []models.Leg, locked map[string]lockedAccount) error {
	change := map[string]*big.Int{} // signed change in each account's normal direction
	for _, leg := range legs {
		a := locked[leg.AccountID]
		delta := big.NewInt(int64(leg.Amount))
		if (leg.Direction == models.Debit) != a.typ.DebitNormal() {
			delta.Neg(delta)
		}
		if change[leg.AccountID] == nil {
			change[leg.AccountID] = new(big.Int)
		}
		change[leg.AccountID].Add(change[leg.AccountID], delta)
	}
	for id, delta := range change {
		a := locked[id]
		if a.overdraft || delta.Sign() >= 0 {
			continue
		}
		var balanceText string
		if err := w.tx.QueryRow(ctx, `SELECT ((CASE WHEN $2 IN ('ASSET', 'EXPENSE') THEN 1 ELSE -1 END) *
				COALESCE(SUM(CASE direction WHEN 'DEBIT' THEN amount ELSE -amount END), 0))::text
			FROM ledger_postings WHERE account_id = $1`, id, string(a.typ)).Scan(&balanceText); err != nil {
			return err
		}
		balance, ok := new(big.Int).SetString(balanceText, 10)
		if !ok {
			return fmt.Errorf("unreadable balance %q", balanceText)
		}
		if new(big.Int).Add(balance, delta).Sign() < 0 {
			return InsufficientFundsError{AccountNumber: a.number, Balance: balance.String(), Change: delta.String()}
		}
	}
	return nil
}

func (w *ledgerWriter) fillAccountNames(ctx context.Context, j *models.Journal) error {
	rows, err := w.tx.Query(ctx, "SELECT id, name FROM accounts WHERE id = ANY($1)", accountIDs(j.Postings))
	if err != nil {
		return err
	}
	defer rows.Close()
	names := map[string]string{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		names[id] = name
	}
	for i := range j.Postings {
		j.Postings[i].AccountName = names[j.Postings[i].AccountID]
	}
	return rows.Err()
}

func accountIDs(postings []models.Posting) []string {
	out := make([]string, 0, len(postings))
	for _, p := range postings {
		out = append(out, p.AccountID)
	}
	return out
}

// LockJournal locks a journal (serializing reversals of it) and loads it with
// its postings and reversal status.
func (w *ledgerWriter) LockJournal(ctx context.Context, id string) (models.Journal, error) {
	if _, err := w.tx.Exec(ctx, "SELECT 1 FROM journal_entries WHERE id = $1 FOR UPDATE", id); err != nil {
		return models.Journal{}, err
	}
	j, err := scanJournal(w.tx.QueryRow(ctx, "SELECT "+journalColumns+" FROM journal_entries j WHERE j.id = $1", id))
	if err != nil {
		return models.Journal{}, notFound(err, ErrJournalNotFound)
	}
	list := []models.Journal{j}
	if err := attachPostings(ctx, w.tx, list); err != nil {
		return models.Journal{}, err
	}
	return list[0], nil
}

func nullableText(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullableTextPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
