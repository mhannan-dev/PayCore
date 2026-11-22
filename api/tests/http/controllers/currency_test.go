package controllers_test

import (
	"net/http"
	"testing"

	"paycore/api/internal/models"
)

func TestJournalCurrencyFilter(t *testing.T) {
	r, ledger, token := ledgerRouter(t, models.PermTransfersRead)
	for _, tc := range []struct {
		query string
		want  models.Currency
	}{
		{"", ""},
		{"?currency=JPY", models.CurrencyJPY},
		{"?currency=%20usd%20", models.CurrencyUSD},
		{"?currency=EUR", models.CurrencyEUR},
	} {
		w := doWithKey(r, http.MethodGet, "/api/v1/transfers"+tc.query, "", token, "")
		if w.Code != http.StatusOK || ledger.lastJournalQuery.Currency != tc.want {
			t.Fatalf("%s: status %d query %+v body %s", tc.query, w.Code, ledger.lastJournalQuery, w.Body)
		}
	}
	w := doWithKey(r, http.MethodGet, "/api/v1/transfers?currency=GBP", "", token, "")
	if w.Code != http.StatusBadRequest || errorCode(t, w) != "invalid_currency" {
		t.Fatalf("invalid currency: %d %s", w.Code, w.Body)
	}
}
