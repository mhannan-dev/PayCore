package controllers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"paycore/api/internal/http/controllers"
	"paycore/api/internal/http/middleware"
	"paycore/api/internal/models"
	"paycore/api/internal/server"
)

type securityTransportStub struct {
	controllers.AccountSecurity
	owner, token string
	calls        int
}

func (s *securityTransportStub) Overview(ctx context.Context, owner, token string) (models.SecurityOverview, error) {
	s.calls++
	s.owner = owner
	s.token = token
	if _, ok := ctx.Deadline(); !ok {
		panic("missing request deadline")
	}
	return models.SecurityOverview{Sessions: []models.SessionInfo{}}, nil
}
func TestSecurityRoutesRequireOnlyTheirOwnSession(t *testing.T) {
	stub := &securityTransportStub{}
	identity := &policyIdentity{user: models.UserDTO{ID: memberID, Roles: []string{models.RoleAccountant}}}
	router := server.New(nil, server.Controllers{Security: controllers.NewSecurityController(stub), AuthMiddleware: middleware.NewAuth(identity)})
	for _, route := range []struct{ method, path string }{
		{"GET", "/api/v1/auth/security"}, {"POST", "/api/v1/auth/security/recovery-codes"},
		{"POST", "/api/v1/auth/security/authenticator"}, {"POST", "/api/v1/auth/security/authenticator/confirm"},
		{"POST", "/api/v1/auth/security/sessions/revoke-others"}, {"POST", "/api/v1/auth/security/sessions/" + adminID + "/revoke"},
	} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(route.method, route.path, strings.NewReader(`{}`)))
		if w.Code != 401 {
			t.Fatalf("anonymous %s %s: %d", route.method, route.path, w.Code)
		}
	}
	// Self-service remains available to an account with no capability grants.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/security", nil)
	req.Header.Set("Authorization", "Bearer valid")
	router.ServeHTTP(w, req)
	if w.Code != 200 || stub.owner != memberID || stub.token != "valid" || stub.calls != 1 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("self-service routing failed: %d %s", w.Code, w.Body)
	}
	// A client-supplied identity/action cannot reach a mutation use case.
	for _, body := range []string{`{}`, `{"password":"secret","code":"123456","user_id":"` + adminID + `"}`} {
		w = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/security/recovery-codes", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer valid")
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		if w.Code != 400 {
			t.Fatalf("unsafe proof accepted: %d %s", w.Code, w.Body)
		}
	}
}
