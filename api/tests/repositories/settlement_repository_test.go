package repositories_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"paycore/api/internal/models"
	"paycore/api/internal/repositories"
	"paycore/api/internal/services"
)

func TestSettlementAtomicLifecycle(t *testing.T) {
	f := newLedgerFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO users(id, first_name, email, password_hash) VALUES ($1, 'Payout tester', 'payout@example.test', 'not-a-login-hash')`, f.actor.ID); err != nil {
		t.Fatal(err)
	}
	clearingRole := models.RolePayoutClearing
	clearing, err := f.svc.CreateAccount(ctx, models.UserDTO{}, models.CreateAccountRequest{AccountNumber: "2003", Name: "Payout clearing", Type: models.AccountLiability, Currency: models.CurrencyJPY, SystemRole: &clearingRole})
	if err != nil {
		t.Fatal(err)
	}
	// The shared fixture's bank has no role; capture needs one.
	if _, err := f.pool.Exec(ctx, `UPDATE accounts SET system_role = 'OPERATING_BANK' WHERE id = $1`, f.bank.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Transfer(ctx, f.actor, f.key(), transfer("FUND-PAYOUT", f.bank, f.alice, 1000)); err != nil {
		t.Fatal(err)
	}
	repo := repositories.NewSettlementRepository(f.pool)
	svc := services.NewSettlementService(repo)
	req := models.CreatePayoutRequest{SourceAccountID: f.alice.ID, Amount: "250", Currency: models.CurrencyJPY, BeneficiaryName: "Hana", BeneficiaryBank: "Bank", BeneficiaryAccount: "00123456"}
	result, err := svc.Reserve(ctx, f.actor, "payout-reserve", req)
	if err != nil {
		t.Fatal(err)
	}
	var payout models.Settlement
	if err := json.Unmarshal(result.Body, &payout); err != nil {
		t.Fatal(err)
	}
	if f.balance(t, f.alice) != "750" || f.balance(t, clearing) != "250" {
		t.Fatal("reserve balances incorrect")
	}
	replay, err := svc.Reserve(ctx, f.actor, "payout-reserve", req)
	if err != nil || !replay.Replayed || f.balance(t, f.alice) != "750" {
		t.Fatalf("reservation replay: %v", err)
	}
	items, total, err := svc.List(ctx, models.SettlementQuery{Currency: models.CurrencyJPY, Limit: 25})
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("JPY page: %d %v", total, err)
	}
	items, total, err = svc.List(ctx, models.SettlementQuery{Currency: models.CurrencyUSD, Limit: 25})
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("USD page: %d %v", total, err)
	}

	// Failed state persistence must roll back its journal, postings, outbox and key claim.
	var before int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM journal_entries`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected persistence failure")
	_, err = repo.Execute(ctx, f.actor.ID, "rollback-payout", strings.Repeat("ab", 32), func(ctx context.Context, w models.SettlementWriter) (int, []byte, error) {
		_, err := w.PostJournal(ctx, models.NewJournal{ReferenceID: "ROLLBACK-PAYOUT", Description: "Rollback payout", Currency: models.CurrencyJPY, Legs: []models.Leg{{AccountID: f.alice.ID, Direction: models.Debit, Amount: 10}, {AccountID: clearing.ID, Direction: models.Credit, Amount: 10}}})
		if err != nil {
			return 0, nil, err
		}
		return 0, nil, injected
	})
	if !errors.Is(err, injected) {
		t.Fatalf("rollback result: %v", err)
	}
	var after int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM journal_entries`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after || f.balance(t, f.alice) != "750" {
		t.Fatal("failed payout left a committed journal or balance")
	}

	// Opposing actions race on the payout row; exactly one may post money.
	outcomes := make(chan error, 2)
	go func() { _, err := svc.Capture(ctx, f.actor, "payout-capture", payout.ID); outcomes <- err }()
	go func() { _, err := svc.Release(ctx, f.actor, "payout-release", payout.ID); outcomes <- err }()
	successes := 0
	for range 2 {
		if <-outcomes == nil {
			successes++
		}
	}
	if successes != 1 || f.balance(t, clearing) != "0" {
		t.Fatalf("competing actions: %d successes", successes)
	}
	stored, err := svc.Get(ctx, payout.ID)
	if err != nil || stored.Status == models.SettlementReserved {
		t.Fatalf("final payout: %+v %v", stored, err)
	}
	var journalCount int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM journal_entries`).Scan(&journalCount); err != nil {
		t.Fatal(err)
	}
	if journalCount != before+1 {
		t.Fatalf("competing actions posted %d journals", journalCount-before)
	}
}

// The database keeps one role holder per currency and ties each role to an account type.
func TestAccountSystemRoleConstraints(t *testing.T) {
	f := newLedgerFixture(t)
	ctx := context.Background()
	clearing := models.RolePayoutClearing
	create := func(number string, currency models.Currency) error {
		_, err := f.svc.CreateAccount(ctx, models.UserDTO{}, models.CreateAccountRequest{AccountNumber: number, Name: "Clearing " + number, Type: models.AccountLiability, Currency: currency, SystemRole: &clearing})
		return err
	}
	if err := create("2003", models.CurrencyJPY); err != nil {
		t.Fatal(err)
	}
	if err := create("2004", models.CurrencyJPY); !errors.Is(err, repositories.ErrSystemRoleTaken) {
		t.Fatalf("second JPY clearing account: %v", err)
	}
	if err := create("2005", models.CurrencyUSD); err != nil {
		t.Fatalf("USD clearing account: %v", err)
	}
	account, err := f.svc.Account(ctx, f.alice.ID)
	if err != nil || account.SystemRole != nil {
		t.Fatalf("ordinary account role = %v, err %v", account.SystemRole, err)
	}
	// Bypassing the application cannot give a role to the wrong account type.
	if _, err := f.pool.Exec(ctx, `UPDATE accounts SET system_role = 'OPERATING_BANK' WHERE id = $1`, f.alice.ID); err == nil {
		t.Fatal("database accepted OPERATING_BANK on a liability account")
	}
}
