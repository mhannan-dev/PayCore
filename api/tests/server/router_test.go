package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"paycore/api/internal/server"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type pingFunc func(context.Context) error

func (f pingFunc) Ping(ctx context.Context) error { return f(ctx) }

func TestReadiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name string
		db   server.DatabasePinger
		code int
	}{
		{"offline route discovery", nil, http.StatusServiceUnavailable},
		{"database available", pingFunc(func(ctx context.Context) error {
			if _, ok := ctx.Deadline(); !ok {
				t.Error("database ping must have a deadline")
			}
			return nil
		}), http.StatusOK},
		{"database unavailable", pingFunc(func(context.Context) error { return errors.New("private database connection details") }), http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := server.SetupRouter(tc.db)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
			if w.Code != tc.code {
				t.Fatalf("got %d, want %d", w.Code, tc.code)
			}
			if strings.Contains(w.Body.String(), "private") {
				t.Fatal("readiness leaks database error")
			}
		})
	}
}

func TestLivenessDoesNotRequireDatabase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := server.SetupRouter(pingFunc(func(context.Context) error { t.Fatal("liveness must not ping database"); return nil }))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", w.Code)
	}
}

func TestContainerBuildsRouter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	container := server.NewContainer(server.Controllers{})
	r := container.Router(nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", w.Code)
	}
}

func TestAuthRateLimitIgnoresUntrustedForwardedIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := server.SetupRouter(nil)
	for i := 0; i < 21; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/captcha", nil)
		req.RemoteAddr = "192.0.2.10:1234"
		req.Header.Set("X-Forwarded-For", "198.51.100."+fmt.Sprint(i))
		r.ServeHTTP(w, req)
		if i == 20 && (w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "") {
			t.Fatal("spoofed proxy headers bypassed auth rate limit")
		}
	}
}

func TestUnlistedBrowserOriginIsRejected(t *testing.T) {
	t.Setenv("WEB_ORIGINS", "http://localhost:3000")
	r := server.SetupRouter(nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
	req.Header.Set("Origin", "https://untrusted.example")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatal("unlisted origin accepted")
	}
}

func TestDevelopmentDefaultAllowsAnyLoopbackPort(t *testing.T) {
	cases := []struct {
		env, webOrigins, origin string
		allowed                 bool
	}{
		{"development", "", "http://localhost:3001", true},
		{"development", "", "http://127.0.0.1:4000", true},
		{"development", "", "http://[::1]:3000", true},
		{"development", "", "https://localhost:3001", false},
		{"development", "", "http://localhost.evil.example:3001", false},
		{"development", "", "http://192.168.1.10:3000", false},
		{"production", "", "http://localhost:3001", false},
		{"development", "http://localhost:3000", "http://localhost:3001", false},
	}
	for _, tc := range cases {
		t.Setenv("ENV", tc.env)
		t.Setenv("WEB_ORIGINS", tc.webOrigins)
		r := server.SetupRouter(nil)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/captcha", nil)
		req.Header.Set("Origin", tc.origin)
		r.ServeHTTP(w, req)
		if got := w.Code != http.StatusForbidden; got != tc.allowed {
			t.Errorf("ENV=%q WEB_ORIGINS=%q origin=%q: allowed=%v, want %v", tc.env, tc.webOrigins, tc.origin, got, tc.allowed)
		}
	}
}

func TestUnknownRoutesAndMethodsReturnJSONEnvelope(t *testing.T) {
	r := server.SetupRouter(nil)
	cases := []struct {
		method, path string
		status       int
		code         string
	}{
		{http.MethodGet, "/api/v1/does-not-exist", http.StatusNotFound, "route_not_found"},
		{http.MethodDelete, "/api/v1/auth/captcha", http.StatusMethodNotAllowed, "method_not_allowed"},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		var body struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if w.Code != tc.status || json.Unmarshal(w.Body.Bytes(), &body) != nil || body.Error != tc.code || body.Message == "" {
			t.Errorf("%s %s: got %d %q", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
	// Browser preflight for a real route must still succeed despite method-not-allowed handling.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/me", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "PATCH")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent || w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Fatalf("preflight broken: %d %v", w.Code, w.Header())
	}
}
