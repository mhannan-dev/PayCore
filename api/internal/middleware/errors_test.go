package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
)

var errKnown = errors.New("known domain error")

func errorRouter(handlerErr error, withFallback bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID(), Errors(apperr.Table{{Target: errKnown, Error: apperr.New(http.StatusConflict, "known", "Known problem.")}}))
	chain := []gin.HandlerFunc{}
	if withFallback {
		chain = append(chain, ErrorFallback(apperr.Unavailable("area_unavailable", "Area unavailable.")))
	}
	chain = append(chain, func(c *gin.Context) { _ = c.Error(handlerErr) })
	r.GET("/x", chain...)
	return r
}

func call(t *testing.T, r *gin.Engine, header string) (int, map[string]string, string) {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	if header != "" {
		req.Header.Set(RequestIDHeader, header)
	}
	r.ServeHTTP(w, req)
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q: %v", w.Body.String(), err)
	}
	return w.Code, body, w.Header().Get(RequestIDHeader)
}

func TestErrorsMiddleware(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		fallback bool
		status   int
		code     string
	}{
		{"apperr passes through", apperr.BadRequest("bad_shape", "Bad shape."), true, 400, "bad_shape"},
		{"table match, even wrapped", errors.Join(errors.New("context"), errKnown), true, 409, "known"},
		{"unconfigured uses group fallback", apperr.ErrNotConfigured, true, 503, "area_unavailable"},
		{"unknown ignores group fallback", errors.New("db exploded: password=secret"), true, 500, "internal_error"},
		{"unknown without fallback is 500", errors.New("boom"), false, 500, "internal_error"},
	}
	for _, tc := range cases {
		status, body, _ := call(t, errorRouter(tc.err, tc.fallback), "")
		if status != tc.status || body["error"] != tc.code {
			t.Errorf("%s: got %d %v", tc.name, status, body)
		}
		if body["message"] == "" {
			t.Errorf("%s: message must not be empty", tc.name)
		}
	}
	// Internal details never reach the client.
	_, body, _ := call(t, errorRouter(errors.New("db exploded: password=secret"), true), "")
	if body["message"] != "Internal Server Error" {
		t.Fatalf("leaked internals: %v", body)
	}
}

func TestRequestID(t *testing.T) {
	r := errorRouter(errKnown, false)
	if _, _, id := call(t, r, ""); len(id) != 24 {
		t.Fatalf("generated id: %q", id)
	}
	if _, _, id := call(t, r, "upstream-req-123"); id != "upstream-req-123" {
		t.Fatalf("well-formed incoming id should be kept: %q", id)
	}
	if _, _, id := call(t, r, "bad id\nInjected: x"); id == "bad id\nInjected: x" || len(id) != 24 {
		t.Fatalf("malformed incoming id must be replaced: %q", id)
	}
}
