package controllers_test

import (
	"context"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"paycore/api/internal/http/controllers"
	"paycore/api/internal/http/middleware"
	"paycore/api/internal/models"
	"paycore/api/internal/repositories"
	"paycore/api/internal/server"
	"paycore/api/internal/services"
)

// fakeLedger records calls and returns scripted results.
type fakeLedger struct {
	lastJournalQuery models.JournalQuery
	transferErr      error
	lastKey          string
	lastActor        models.UserDTO
	replay           bool
}

func (f *fakeLedger) CreateAccount(_ context.Context, actor models.UserDTO, req models.CreateAccountRequest) (models.Account, error) {
	if err := req.Normalize(); err != nil {
		return models.Account{}, err
	}
	return models.Account{ID: "acc", AccountNumber: req.AccountNumber, Balance: "0"}, nil
}
func (f *fakeLedger) Account(context.Context, string) (models.Account, error) {
	return models.Account{}, repositories.ErrAccountNotFound
}
func (f *fakeLedger) Accounts(context.Context, models.AccountQuery) ([]models.Account, int, error) {
	return []models.Account{{ID: "a", Balance: "-5"}}, 1, nil
}
func (f *fakeLedger) AccountPostings(context.Context, string, int, int) ([]models.AccountPosting, int, error) {
	return nil, 0, nil
}
func (f *fakeLedger) Journal(context.Context, string) (models.Journal, error) {
	return models.Journal{}, repositories.ErrJournalNotFound
}
func (f *fakeLedger) Journals(_ context.Context, q models.JournalQuery) ([]models.Journal, int, error) {
	f.lastJournalQuery = q
	return nil, 0, nil
}
func (f *fakeLedger) ZeroSum(_ context.Context, c models.Currency) (models.ZeroSumReport, error) {
	return models.ZeroSumReport{Currency: c, Balanced: true, Difference: "0"}, nil
}
func (f *fakeLedger) Transfer(_ context.Context, actor models.UserDTO, key string, req models.TransferRequest) (models.IdempotentResult, error) {
	f.lastKey, f.lastActor = key, actor
	if err := req.Normalize(); err != nil {
		return models.IdempotentResult{}, err
	}
	if f.transferErr != nil {
		return models.IdempotentResult{}, f.transferErr
	}
	return models.IdempotentResult{Status: 201, Body: []byte(`{"id":"j1"}`), Replayed: f.replay}, nil
}
func (f *fakeLedger) Reverse(_ context.Context, _ models.UserDTO, key, id string, _ models.ReverseRequest) (models.IdempotentResult, error) {
	f.lastKey = key
	return models.IdempotentResult{}, services.ErrCannotReverseReversal
}

func ledgerRouter(t *testing.T, permissions ...string) (*testRouter, *fakeLedger, string) {
	t.Helper()
	caller := models.User{ID: adminID, Email: "op@example.test", Roles: []string{"OPERATOR"}, IsActive: true, Permissions: permissions}
	auth, token := sessionAuth(t, caller)
	ledger := &fakeLedger{}
	router := server.New(nil, server.Controllers{Ledger: controllers.NewLedgerController(ledger), AuthMiddleware: middleware.NewAuth(auth)})
	return &testRouter{router}, ledger, token
}

// sessionAuth returns an AuthService whose only valid session is caller's.
func sessionAuth(t *testing.T, caller models.User) (*services.AuthService, string) {
	t.Helper()
	token := strings.Repeat("b", 64)
	digest := sha256.Sum256([]byte(token))
	auth, err := services.NewAuthService(&testStore{user: caller, hash: digest[:]}, &testCaptcha{})
	if err != nil {
		t.Fatal(err)
	}
	return auth, token
}

const validTransfer = `{"reference_id":"T-1","description":"pay","currency":"jpy","legs":[
	{"account_id":"11111111-1111-4111-8111-111111111111","direction":"debit","amount":"500"},
	{"account_id":"22222222-2222-4222-8222-222222222222","direction":"CREDIT","amount":"500"}]}`

func doWithKey(r *testRouter, method, path, body, token, key string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	r.h.ServeHTTP(w, req)
	return w
}

func TestLedgerHTTP(t *testing.T) {
	r, ledger, token := ledgerRouter(t, models.PermTransfersCreate, models.PermTransfersReverse, models.PermLedgerRead, models.PermAuditVerify)

	if w := doWithKey(r, http.MethodPost, "/api/v1/transfers", validTransfer, token, ""); w.Code != 400 || errorCode(t, w) != "idempotency_key_required" {
		t.Fatalf("missing key: %d %s", w.Code, w.Body)
	}
	w := doWithKey(r, http.MethodPost, "/api/v1/transfers", validTransfer, token, "key-00000001")
	if w.Code != 201 || w.Body.String() != `{"id":"j1"}` || ledger.lastKey != "key-00000001" || ledger.lastActor.ID != adminID {
		t.Fatalf("create: %d %s %+v", w.Code, w.Body, ledger)
	}
	ledger.replay = true
	if w := doWithKey(r, http.MethodPost, "/api/v1/transfers", validTransfer, token, "key-00000001"); w.Header().Get("Idempotent-Replayed") != "true" {
		t.Fatalf("replay header missing")
	}
	numberAmount := strings.Replace(validTransfer, `"500"}`, `500}`, 1)
	if w := doWithKey(r, http.MethodPost, "/api/v1/transfers", numberAmount, token, "key-00000002"); w.Code != 400 || errorCode(t, w) != "invalid_transfer" {
		t.Fatalf("numeric amount must be rejected: %d %s", w.Code, w.Body)
	}
	unbalanced := strings.Replace(validTransfer, `"CREDIT","amount":"500"`, `"CREDIT","amount":"499"`, 1)
	if w := doWithKey(r, http.MethodPost, "/api/v1/transfers", unbalanced, token, "key-00000003"); w.Code != 400 || errorCode(t, w) != "unbalanced_transfer" {
		t.Fatalf("unbalanced: %d %s", w.Code, w.Body)
	}

	cases := []struct {
		err    error
		status int
		code   string
	}{
		{repositories.InsufficientFundsError{AccountNumber: "2001", Balance: "10", Change: "-500"}, 422, "insufficient_funds"},
		{repositories.ErrIdempotencyMismatch, 422, "idempotency_key_reused"},
		{repositories.ErrIdempotencyInFlight, 409, "idempotency_in_flight"},
		{repositories.ErrLedgerBusy, 503, "ledger_busy"},
		{repositories.ErrCurrencyMismatch, 422, "currency_mismatch"},
		{repositories.ErrAccountNotFound, 404, "account_not_found"},
	}
	for _, tc := range cases {
		ledger.transferErr, ledger.replay = tc.err, false
		w := doWithKey(r, http.MethodPost, "/api/v1/transfers", validTransfer, token, "key-00000009")
		if w.Code != tc.status || errorCode(t, w) != tc.code {
			t.Errorf("%v: got %d %s", tc.err, w.Code, w.Body)
		}
		if tc.code == "idempotency_in_flight" && w.Header().Get("Retry-After") == "" {
			t.Errorf("in-flight needs Retry-After")
		}
	}

	if w := doWithKey(r, http.MethodPost, "/api/v1/transfers/33333333-3333-4333-8333-333333333333/reverse", "", token, "key-00000010"); w.Code != 409 || errorCode(t, w) != "cannot_reverse_reversal" {
		t.Fatalf("reverse: %d %s", w.Code, w.Body)
	}
	if w := doWithKey(r, http.MethodGet, "/api/v1/accounts/33333333-3333-4333-8333-333333333333", "", token, ""); w.Code != 404 || errorCode(t, w) != "account_not_found" {
		t.Fatalf("account 404: %d %s", w.Code, w.Body)
	}
	if w := doWithKey(r, http.MethodGet, "/api/v1/audit/zero-sum?currency=usd", "", token, ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"currency":"USD"`) {
		t.Fatalf("zero-sum: %d %s", w.Code, w.Body)
	}
	// accounts:write is not granted to this caller.
	if w := doWithKey(r, http.MethodPost, "/api/v1/accounts", `{"account_number":"9","name":"x","type":"ASSET","currency":"JPY"}`, token, ""); w.Code != 403 {
		t.Fatalf("create account without accounts:write: %d", w.Code)
	}
}
