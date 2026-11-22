package repositories_test

import (
	"context"
	"testing"

	"paycore/api/internal/models"
)

func TestJournalCurrencyPagination(t *testing.T) {
	f := newLedgerFixture(t)
	ctx := context.Background()
	usdWallet, err := f.svc.CreateAccount(ctx, models.UserDTO{}, models.CreateAccountRequest{
		AccountNumber: "2101", Name: "USD wallet", Type: models.AccountLiability, Currency: models.CurrencyUSD,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		ref           string
		debit, credit models.Account
	}{
		{"JPY-1", f.bank, f.alice},
		{"USD-1", f.usdCash, usdWallet},
		{"USD-2", f.usdCash, usdWallet},
	} {
		req := transfer(tc.ref, tc.debit, tc.credit, 100)
		req.Currency = tc.debit.Currency
		if _, err := f.svc.Transfer(ctx, f.actor, f.key(), req); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		currency models.Currency
		total    int
	}{
		{"", 3}, {models.CurrencyJPY, 1}, {models.CurrencyUSD, 2}, {models.CurrencyEUR, 0},
	} {
		items, total, err := f.svc.Journals(ctx, models.JournalQuery{Currency: tc.currency, Limit: 1})
		if err != nil || total != tc.total || len(items) != min(tc.total, 1) {
			t.Fatalf("%s: items %v total %d error %v", tc.currency, items, total, err)
		}
		for _, item := range items {
			if tc.currency != "" && item.Currency != tc.currency {
				t.Fatalf("%s returned %s", tc.currency, item.Currency)
			}
		}
	}
	items, total, err := f.svc.Journals(ctx, models.JournalQuery{Currency: models.CurrencyUSD, AccountID: usdWallet.ID, Limit: 1, Offset: 1})
	if err != nil || total != 2 || len(items) != 1 || items[0].ReferenceID != "USD-1" {
		t.Fatalf("USD second page: %v %d %v", items, total, err)
	}
	_, _, err = f.svc.Journals(ctx, models.JournalQuery{Currency: "GBP", Limit: 1})
	if err == nil {
		t.Fatal("unsupported currency was accepted")
	}
}
