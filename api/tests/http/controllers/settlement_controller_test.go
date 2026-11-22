package controllers_test

import (
	"context"
	"net/http"
	"testing"

	"paycore/api/internal/http/controllers"
	"paycore/api/internal/http/middleware"
	"paycore/api/internal/models"
	"paycore/api/internal/server"
)

type settlementTransport struct {
	controllers.Settlements
	key       string
	query     models.SettlementQuery
	actor     models.UserDTO
	actionErr error
}

func (s *settlementTransport) List(_ context.Context, q models.SettlementQuery) ([]models.Settlement, int, error) {
	s.query = q
	return nil, 0, nil
}
func (s *settlementTransport) Reserve(_ context.Context, actor models.UserDTO, key string, req models.CreatePayoutRequest) (models.IdempotentResult, error) {
	s.key, s.actor = key, actor
	if err := req.Normalize(); err != nil {
		return models.IdempotentResult{}, err
	}
	return models.IdempotentResult{Status: http.StatusCreated, Body: []byte(`{"status":"RESERVED"}`), Replayed: true}, nil
}
func (s *settlementTransport) Capture(_ context.Context, actor models.UserDTO, key, _ string) (models.IdempotentResult, error) {
	s.key, s.actor = key, actor
	if s.actionErr != nil {
		return models.IdempotentResult{}, s.actionErr
	}
	return models.IdempotentResult{Status: http.StatusOK, Body: []byte(`{"status":"CAPTURED"}`)}, nil
}
func (s *settlementTransport) Release(ctx context.Context, actor models.UserDTO, key, id string) (models.IdempotentResult, error) {
	return s.Capture(ctx, actor, key, id)
}

func TestSettlementTransport(t *testing.T) {
	caller := models.User{ID: adminID, Email: "operator@example.test", IsActive: true, Permissions: []string{models.PermTransfersRead, models.PermTransfersCreate, models.PermTransfersReverse}}
	auth, token := sessionAuth(t, caller)
	svc := &settlementTransport{}
	r := &testRouter{server.New(nil, server.Controllers{Settlement: controllers.NewSettlementController(svc), AuthMiddleware: middleware.NewAuth(auth)})}
	w := doWithKey(r, http.MethodGet, "/api/v1/settlements?currency=usd&limit=10&offset=20", "", token, "")
	if w.Code != http.StatusOK || svc.query.Currency != models.CurrencyUSD || svc.query.Limit != 10 || svc.query.Offset != 20 {
		t.Fatalf("query: %+v %d %s", svc.query, w.Code, w.Body)
	}
	for _, query := range []string{"currency=GBP", "limit=0", "offset=abc"} {
		if w := doWithKey(r, http.MethodGet, "/api/v1/settlements?"+query, "", token, ""); w.Code != http.StatusBadRequest {
			t.Fatalf("bad query %s: %d", query, w.Code)
		}
	}
	const body = `{"source_account_id":"22222222-2222-4222-8222-222222222222","amount":"25000","currency":"JPY","beneficiary_name":"Hana","beneficiary_bank":"Bank","beneficiary_account":"001234"}`
	for _, route := range []string{"/api/v1/settlements", "/api/v1/settlements/22222222-2222-4222-8222-222222222222/capture", "/api/v1/settlements/22222222-2222-4222-8222-222222222222/release"} {
		if w := doWithKey(r, http.MethodPost, route, body, token, ""); w.Code != http.StatusBadRequest || errorCode(t, w) != "idempotency_key_required" {
			t.Fatalf("missing key %s: %d %s", route, w.Code, w.Body)
		}
		w := doWithKey(r, http.MethodPost, route, body, token, "payout-key-0001")
		if w.Code >= http.StatusBadRequest || svc.key != "payout-key-0001" || svc.actor.ID != adminID {
			t.Fatalf("mutation %s: %d %s", route, w.Code, w.Body)
		}
		if route == "/api/v1/settlements" && w.Header().Get("Idempotent-Replayed") != "true" {
			t.Fatal("replay header missing")
		}
	}
	if w := doWithKey(r, http.MethodPost, "/api/v1/settlements", `{"amount":25000}`, token, "payout-key-0002"); w.Code != http.StatusBadRequest {
		t.Fatalf("numeric money accepted: %d", w.Code)
	}
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{models.ErrPayoutNotFound, http.StatusNotFound, "settlement_not_found"},
		{models.ErrPayoutState, http.StatusConflict, "invalid_settlement_state"},
		{models.ErrPayoutProvider, http.StatusConflict, "unsupported_provider"},
		{models.ErrPayoutSource, http.StatusBadRequest, "invalid_source_account"},
		{models.ErrPayoutBank, http.StatusConflict, "operating_bank_missing"},
		{models.ErrPayoutClearing, http.StatusConflict, "clearing_account_missing"},
	} {
		svc.actionErr = tc.err
		w := doWithKey(r, http.MethodPost, "/api/v1/settlements/22222222-2222-4222-8222-222222222222/capture", "", token, "payout-error-key")
		if w.Code != tc.status || errorCode(t, w) != tc.code {
			t.Fatalf("%s: %d %s", tc.code, w.Code, w.Body)
		}
	}
}
