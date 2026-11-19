package repository_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"paycore/api/internal/model"
	"paycore/api/internal/repository"
	"paycore/api/internal/service"
)

type ledgerFixture struct {
	pool                      *pgxpool.Pool
	repo                      *repository.LedgerRepository
	svc                       *service.LedgerService
	actor                     model.UserDTO
	bank, alice, bob, usdCash model.Account
	keySeq                    int
	mu                        sync.Mutex
}

func newLedgerFixture(t *testing.T) *ledgerFixture {
	t.Helper()
	pool := isolatedPool(t)
	f := &ledgerFixture{pool: pool, repo: repository.NewLedgerRepository(pool)}
	f.svc = service.NewLedgerService(f.repo)
	f.actor = model.UserDTO{ID: "11111111-1111-4111-8111-111111111111", Email: "operator@example.test"}
	ctx := context.Background()
	create := func(number, name string, typ model.AccountType, currency model.Currency) model.Account {
		a, err := f.svc.CreateAccount(ctx, model.UserDTO{}, model.CreateAccountRequest{AccountNumber: number, Name: name, Type: typ, Currency: currency})
		if err != nil {
			t.Fatalf("create %s: %v", number, err)
		}
		return a
	}
	f.bank = create("1001", "Operating bank", model.AccountAsset, model.CurrencyJPY)
	f.alice = create("2001", "Alice wallet", model.AccountLiability, model.CurrencyJPY)
	f.bob = create("2002", "Bob wallet", model.AccountLiability, model.CurrencyJPY)
	f.usdCash = create("1101", "USD cash", model.AccountAsset, model.CurrencyUSD)
	return f
}

func (f *ledgerFixture) key() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.keySeq++
	return fmt.Sprintf("test-key-%06d", f.keySeq)
}

func transfer(ref string, debit, credit model.Account, amount model.Amount) model.TransferRequest {
	return model.TransferRequest{ReferenceID: ref, Description: "test " + ref, Currency: model.CurrencyJPY, Legs: []model.Leg{
		{AccountID: debit.ID, Direction: model.Debit, Amount: amount},
		{AccountID: credit.ID, Direction: model.Credit, Amount: amount},
	}}
}

func (f *ledgerFixture) balance(t *testing.T, a model.Account) string {
	t.Helper()
	got, err := f.repo.GetAccount(context.Background(), a.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got.Balance
}

func decodeJournal(t *testing.T, r model.IdempotentResult) model.Journal {
	t.Helper()
	var j model.Journal
	if err := json.Unmarshal(r.Body, &j); err != nil {
		t.Fatalf("journal body %s: %v", r.Body, err)
	}
	return j
}

func TestLedgerTransfersAndReversals(t *testing.T) {
	f := newLedgerFixture(t)
	ctx := context.Background()

	// Fund Alice: debit bank (asset up), credit Alice's wallet (liability up).
	res, err := f.svc.Transfer(ctx, f.actor, f.key(), transfer("FUND-1", f.bank, f.alice, 1000))
	if err != nil || res.Status != 201 || res.Replayed {
		t.Fatalf("fund: %+v %v", res, err)
	}
	funding := decodeJournal(t, res)
	if funding.Total != 1000 || len(funding.Postings) != 2 || funding.Postings[0].AccountNumber != "1001" {
		t.Fatalf("funding journal: %+v", funding)
	}
	if f.balance(t, f.bank) != "1000" || f.balance(t, f.alice) != "1000" {
		t.Fatalf("balances after funding: bank %s alice %s", f.balance(t, f.bank), f.balance(t, f.alice))
	}

	// Alice pays Bob 300 (debit Alice's liability, credit Bob's).
	if _, err := f.svc.Transfer(ctx, f.actor, f.key(), transfer("PAY-1", f.alice, f.bob, 300)); err != nil {
		t.Fatal(err)
	}
	if f.balance(t, f.alice) != "700" || f.balance(t, f.bob) != "300" {
		t.Fatalf("after payment: alice %s bob %s", f.balance(t, f.alice), f.balance(t, f.bob))
	}

	// Overdraft: Bob cannot send 301; nothing is posted.
	_, err = f.svc.Transfer(ctx, f.actor, f.key(), transfer("PAY-2", f.bob, f.alice, 301))
	var funds repository.InsufficientFundsError
	if !errors.As(err, &funds) || funds.AccountNumber != "2002" || funds.Balance != "300" || funds.Change != "-301" {
		t.Fatalf("overdraft: %v", err)
	}

	// Validation, currency, inactive and unknown accounts.
	unbalanced := transfer("BAD-1", f.alice, f.bob, 10)
	unbalanced.Legs[1].Amount = 9
	var invalid model.InvalidRequestError
	if _, err := f.svc.Transfer(ctx, f.actor, f.key(), unbalanced); !errors.As(err, &invalid) || invalid.Code != "unbalanced_transfer" {
		t.Fatalf("unbalanced: %v", err)
	}
	if _, err := f.svc.Transfer(ctx, f.actor, f.key(), transfer("FX-1", f.usdCash, f.alice, 10)); !errors.Is(err, repository.ErrCurrencyMismatch) {
		t.Fatalf("currency: %v", err)
	}
	ghost := model.Account{ID: "99999999-9999-4999-8999-999999999999"}
	if _, err := f.svc.Transfer(ctx, f.actor, f.key(), transfer("GHOST", ghost, f.alice, 10)); !errors.Is(err, repository.ErrAccountNotFound) {
		t.Fatalf("unknown account: %v", err)
	}
	if _, err := f.pool.Exec(ctx, "UPDATE accounts SET is_active = false WHERE id = $1", f.usdCash.ID); err != nil {
		t.Fatal(err)
	}
	usd := transfer("USD-1", f.usdCash, f.usdCash, 10)
	usd.Currency = model.CurrencyUSD
	if _, err := f.svc.Transfer(ctx, f.actor, f.key(), usd); !errors.Is(err, repository.ErrAccountInactive) {
		t.Fatalf("inactive: %v", err)
	}

	// Reverse the payment: balances return; a second reversal and reversing the reversal fail.
	payments, _, err := f.svc.Journals(ctx, model.JournalQuery{Search: "PAY-1", Limit: 5})
	if err != nil || len(payments) != 1 {
		t.Fatalf("find payment: %v %v", payments, err)
	}
	rev, err := f.svc.Reverse(ctx, f.actor, f.key(), payments[0].ID, model.ReverseRequest{})
	if err != nil || rev.Status != 201 {
		t.Fatalf("reverse: %v", err)
	}
	reversal := decodeJournal(t, rev)
	if reversal.ReversalForID == nil || *reversal.ReversalForID != payments[0].ID || reversal.ReferenceID != "REV-PAY-1" {
		t.Fatalf("reversal journal: %+v", reversal)
	}
	if f.balance(t, f.alice) != "1000" || f.balance(t, f.bob) != "0" {
		t.Fatalf("after reversal: alice %s bob %s", f.balance(t, f.alice), f.balance(t, f.bob))
	}
	if _, err := f.svc.Reverse(ctx, f.actor, f.key(), payments[0].ID, model.ReverseRequest{}); !errors.Is(err, repository.ErrAlreadyReversed) {
		t.Fatalf("double reversal: %v", err)
	}
	if _, err := f.svc.Reverse(ctx, f.actor, f.key(), reversal.ID, model.ReverseRequest{}); !errors.Is(err, service.ErrCannotReverseReversal) {
		t.Fatalf("reverse a reversal: %v", err)
	}
	if got, _ := f.svc.Journal(ctx, payments[0].ID); got.ReversedByID == nil || *got.ReversedByID != reversal.ID {
		t.Fatalf("original should point at its reversal: %+v", got)
	}

	// Account history and the zero-sum audit.
	history, total, err := f.svc.AccountPostings(ctx, f.alice.ID, 10, 0)
	if err != nil || total != 3 || history[0].ReferenceID != "REV-PAY-1" {
		t.Fatalf("alice history: %d %+v %v", total, history, err)
	}
	report, err := f.svc.ZeroSum(ctx, model.CurrencyJPY)
	if err != nil || !report.Balanced || report.TotalDebits != "1600" || report.Journals != 3 || report.Postings != 6 {
		t.Fatalf("zero-sum: %+v %v", report, err)
	}
}

func TestLedgerIdempotency(t *testing.T) {
	f := newLedgerFixture(t)
	ctx := context.Background()
	key := f.key()
	first, err := f.svc.Transfer(ctx, f.actor, key, transfer("FUND-1", f.bank, f.alice, 500))
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.svc.Transfer(ctx, f.actor, key, transfer("FUND-1", f.bank, f.alice, 500))
	if err != nil || !again.Replayed || again.Status != 201 || decodeJournal(t, again).ID != decodeJournal(t, first).ID {
		t.Fatalf("replay: %+v %v", again, err)
	}
	if f.balance(t, f.alice) != "500" {
		t.Fatalf("a replay must not post twice: %s", f.balance(t, f.alice))
	}
	if _, err := f.svc.Transfer(ctx, f.actor, key, transfer("FUND-1", f.bank, f.alice, 501)); !errors.Is(err, repository.ErrIdempotencyMismatch) {
		t.Fatalf("changed request: %v", err)
	}
	// Keys are scoped per client: another user may use the same key.
	other := model.UserDTO{ID: "22222222-2222-4222-8222-222222222222"}
	if res, err := f.svc.Transfer(ctx, other, key, transfer("FUND-2", f.bank, f.alice, 1)); err != nil || res.Replayed {
		t.Fatalf("other client: %+v %v", res, err)
	}
	// A failed request does not consume its key.
	failKey := f.key()
	if _, err := f.svc.Transfer(ctx, f.actor, failKey, transfer("PAY-1", f.bob, f.alice, 10)); err == nil {
		t.Fatal("expected insufficient funds")
	}
	if _, err := f.svc.Transfer(ctx, f.actor, failKey, transfer("PAY-1", f.alice, f.bob, 10)); err != nil {
		t.Fatalf("retry after failure with a corrected request: %v", err)
	}
}

func TestLedgerConcurrency(t *testing.T) {
	f := newLedgerFixture(t)
	ctx := context.Background()
	for _, w := range []model.Account{f.alice, f.bob} {
		if _, err := f.svc.Transfer(ctx, f.actor, f.key(), transfer("FUND-"+w.AccountNumber, f.bank, w, 1000)); err != nil {
			t.Fatal(err)
		}
	}

	// Opposite-direction transfers between the same accounts: no deadlock errors surface.
	var wg sync.WaitGroup
	errs := make(chan error, 40)
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			from, to := f.alice, f.bob
			if i%2 == 1 {
				from, to = f.bob, f.alice
			}
			_, err := f.svc.Transfer(ctx, f.actor, f.key(), transfer(fmt.Sprintf("X-%d", i), from, to, 7))
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent opposite transfers: %v", err)
		}
	}
	if f.balance(t, f.alice) != "1000" || f.balance(t, f.bob) != "1000" {
		t.Fatalf("balances drifted: alice %s bob %s", f.balance(t, f.alice), f.balance(t, f.bob))
	}

	// Concurrent overspend: Bob has 1000; 10 parallel transfers of 300 → exactly 3 succeed.
	var ok, short int
	var mu sync.Mutex
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := f.svc.Transfer(ctx, f.actor, f.key(), transfer(fmt.Sprintf("SPEND-%d", i), f.bob, f.alice, 300))
			var funds repository.InsufficientFundsError
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errors.As(err, &funds):
				short++
			default:
				t.Errorf("unexpected: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if ok != 3 || short != 7 || f.balance(t, f.bob) != "100" {
		t.Fatalf("overspend: ok=%d short=%d bob=%s", ok, short, f.balance(t, f.bob))
	}

	// Concurrent retries of one key: exactly one journal, identical responses.
	key := f.key()
	bodies := make([][]byte, 8)
	for i := range bodies {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res, err := f.svc.Transfer(ctx, f.actor, key, transfer("ONCE", f.alice, f.bob, 50))
			if err != nil {
				t.Errorf("same-key retry %d: %v", i, err)
				return
			}
			bodies[i] = res.Body
		}(i)
	}
	wg.Wait()
	ids := map[string]bool{}
	for _, b := range bodies {
		if b != nil {
			var j model.Journal
			_ = json.Unmarshal(b, &j)
			ids[j.ID] = true
		}
	}
	if len(ids) != 1 {
		t.Fatalf("same key produced %d journals", len(ids))
	}
	if found, _, _ := f.svc.Journals(ctx, model.JournalQuery{Search: "ONCE", Limit: 10}); len(found) != 1 {
		t.Fatalf("expected one ONCE journal, got %d", len(found))
	}
	if report, err := f.svc.ZeroSum(ctx, model.CurrencyJPY); err != nil || !report.Balanced {
		t.Fatalf("zero-sum after concurrency: %+v %v", report, err)
	}
}

// The database rejects invariant violations even when the application is bypassed.
func TestLedgerDatabaseInvariants(t *testing.T) {
	f := newLedgerFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := f.svc.Transfer(ctx, f.actor, f.key(), transfer("FUND-1", f.bank, f.alice, 100)); err != nil {
		t.Fatal(err)
	}
	var journalID, postingID string
	if err := f.pool.QueryRow(ctx, "SELECT journal_entry_id, id FROM ledger_postings LIMIT 1").Scan(&journalID, &postingID); err != nil {
		t.Fatal(err)
	}

	commitFails := func(name string, statements ...string) {
		t.Helper()
		tx, err := f.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		for _, s := range statements {
			if _, err := tx.Exec(ctx, s); err != nil {
				return // rejected immediately: fine
			}
		}
		if err := tx.Commit(ctx); err == nil {
			t.Errorf("%s: database accepted it", name)
		}
	}
	j := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	insertJournal := fmt.Sprintf("INSERT INTO journal_entries (id, reference_id, description, currency) VALUES ('%s', 'R', 'D', 'JPY')", j)
	posting := func(account, direction string, amount int) string {
		return fmt.Sprintf("INSERT INTO ledger_postings (journal_entry_id, account_id, amount, direction) VALUES ('%s', '%s', %d, '%s')", j, account, amount, direction)
	}
	commitFails("unbalanced journal", insertJournal, posting(f.bank.ID, "DEBIT", 10), posting(f.alice.ID, "CREDIT", 9))
	commitFails("single posting", insertJournal, posting(f.bank.ID, "DEBIT", 10))
	commitFails("journal without postings", insertJournal)
	commitFails("account in another currency", insertJournal, posting(f.usdCash.ID, "DEBIT", 10), posting(f.alice.ID, "CREDIT", 10))
	commitFails("update a posting", "UPDATE ledger_postings SET amount = 1 WHERE id = '"+postingID+"'")
	commitFails("delete a journal", "DELETE FROM journal_entries WHERE id = '"+journalID+"'")
	commitFails("posting into a committed journal",
		fmt.Sprintf("INSERT INTO ledger_postings (journal_entry_id, account_id, amount, direction) VALUES ('%s', '%s', 5, 'DEBIT')", journalID, f.bank.ID),
		fmt.Sprintf("INSERT INTO ledger_postings (journal_entry_id, account_id, amount, direction) VALUES ('%s', '%s', 5, 'CREDIT')", journalID, f.alice.ID))
	commitFails("reversal that does not mirror",
		fmt.Sprintf("INSERT INTO journal_entries (id, reference_id, description, currency, reversal_for_id) VALUES ('%s', 'R', 'D', 'JPY', '%s')", j, journalID),
		posting(f.alice.ID, "DEBIT", 50), posting(f.bank.ID, "CREDIT", 50))
	commitFails("incomplete idempotency record", "INSERT INTO idempotency_records (client_id, key, request_hash) VALUES ('c', 'k', '"+string(bytes.Repeat([]byte("a"), 64))+"')")
	commitFails("truncate postings", "TRUNCATE ledger_postings CASCADE")

	if report, err := f.svc.ZeroSum(ctx, model.CurrencyJPY); err != nil || !report.Balanced || report.Journals != 1 {
		t.Fatalf("ledger must be unchanged: %+v %v", report, err)
	}
}
