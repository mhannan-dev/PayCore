package controllers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"paycore/api/internal/http/controllers"
	"paycore/api/internal/http/middleware"
	"paycore/api/internal/models"
	"paycore/api/internal/server"
)

type quantumHTTP struct {
	controllers.Quantum
	created   bool
	submitErr error
}

func (q *quantumHTTP) Config() models.QuantumConfig {
	return models.QuantumConfig{Enabled: true, ChainID: 12345, TokenSymbol: "tUSD8"}
}
func (q *quantumHTTP) Checkout(context.Context, string) (models.QuantumCheckout, error) {
	return models.QuantumCheckout{MerchantName: "Public merchant", AmountBaseUnits: 1, Status: models.QuantumPending}, nil
}
func (q *quantumHTTP) CreateInvoice(context.Context, models.UserDTO, string, models.QuantumInvoiceRequest) (models.IdempotentResult, error) {
	q.created = true
	return models.IdempotentResult{Status: http.StatusCreated, Body: []byte(`{"status":"PENDING"}`)}, nil
}
func (q *quantumHTTP) RegisterAttempt(context.Context, string, models.QuantumAttemptRequest) (models.QuantumAttempt, error) {
	return models.QuantumAttempt{Status: models.QuantumCreated}, nil
}
func (q *quantumHTTP) Submit(context.Context, string, string, models.QuantumTransactionRequest) (models.QuantumAttempt, error) {
	return models.QuantumAttempt{Status: models.QuantumBroadcast}, q.submitErr
}

func TestQuantumExpiredIntentResponse(t *testing.T) {
	svc := &quantumHTTP{submitErr: models.ErrQuantumAttemptExpired}
	router := &testRouter{server.New(nil, server.Controllers{Quantum: controllers.NewQuantumController(svc)})}
	w := doWithKey(router, http.MethodPost, "/api/v1/quantum/checkout/payment-link/attempts/attempt-id/tx", `{"tx_hash":"0x1111111111111111111111111111111111111111111111111111111111111111"}`, "", "")
	if w.Code != http.StatusConflict || errorCode(t, w) != "attempt_expired" {
		t.Fatalf("expired intent: %d %s", w.Code, w.Body)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("expired-intent response can be cached")
	}
}
func TestQuantumHTTPAuthorizationAndPublicPrivacy(t *testing.T) {
	for _, tc := range []struct {
		name          string
		permissions   []string
		authenticated bool
		want          int
	}{{"no session", nil, false, http.StatusUnauthorized}, {"no write", []string{models.PermQuantumRead}, true, http.StatusForbidden}, {"missing key", []string{models.PermQuantumWrite}, true, http.StatusBadRequest}} {
		t.Run(tc.name, func(t *testing.T) {
			auth, token := sessionAuth(t, models.User{ID: adminID, Email: "merchant@example.test", IsActive: true, Permissions: tc.permissions})
			svc := &quantumHTTP{}
			router := &testRouter{server.New(nil, server.Controllers{Quantum: controllers.NewQuantumController(svc), AuthMiddleware: middleware.NewAuth(auth)})}
			if !tc.authenticated {
				token = ""
			}
			w := doWithKey(router, http.MethodPost, "/api/v1/quantum/invoices", `{"amount_base_units":"1","description":"Invoice","expires_in_minutes":30}`, token, "")
			if w.Code != tc.want || svc.created {
				t.Fatalf("status %d %s", w.Code, w.Body)
			}
		})
	}
	var logs bytes.Buffer
	before := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(before)
	router := &testRouter{server.New(nil, server.Controllers{Quantum: controllers.NewQuantumController(&quantumHTTP{})})}
	const token = "private-checkout-capability"
	w := doWithKey(router, http.MethodGet, "/api/v1/quantum/checkout/"+token, "", "", "")
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("public checkout: %d %s", w.Code, w.Body)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"id", "merchant_id", "owner_user_id", "email", "checkout_token", "checkout_url"} {
		if _, ok := fields[key]; ok {
			t.Fatal("public checkout leaked " + key)
		}
	}
	if strings.Contains(logs.String(), token) {
		t.Fatal("checkout capability leaked into logs")
	}
	for n := 0; n < 21; n++ {
		w = doWithKey(router, http.MethodPost, "/api/v1/quantum/checkout/"+token+"/attempts", `{"payer_address":"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`, "", "")
	}
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
		t.Fatal("public writes are not rate limited")
	}
}
func TestQuantumDisabledDoesNotBreakPayCore(t *testing.T) {
	router := &testRouter{server.New(nil, server.Controllers{})}
	w := doWithKey(router, http.MethodGet, "/api/v1/quantum/config", "", "", "")
	var cfg models.QuantumConfig
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil || w.Code != http.StatusOK || cfg.Enabled || cfg.ChainID != 0 {
		t.Fatalf("disabled config: %d %s", w.Code, w.Body)
	}
	w = doWithKey(router, http.MethodGet, "/api/v1/quantum/checkout/unknown", "", "", "")
	if w.Code != http.StatusServiceUnavailable || errorCode(t, w) != "quantum_unavailable" {
		t.Fatalf("readiness: %d %s", w.Code, w.Body)
	}
}
