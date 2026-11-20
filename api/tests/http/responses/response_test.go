package responses_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"paycore/api/internal/http/responses"
	"testing"

	"github.com/gin-gonic/gin"
)

func run(t *testing.T, controller gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", controller)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	return w
}

func decodeError(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %q", w.Body.String())
	}
	return body
}

func TestErrorEnvelope(t *testing.T) {
	w := run(t, func(c *gin.Context) { responses.BadRequest(c, "invalid_limit", "limit must be 1 to 100.") })
	body := decodeError(t, w)
	if w.Code != http.StatusBadRequest || body["error"] != "invalid_limit" || body["message"] != "limit must be 1 to 100." {
		t.Fatalf("unexpected envelope %d %v", w.Code, body)
	}
	if _, ok := body["details"]; ok {
		t.Fatal("details must be omitted when empty")
	}
}

func TestErrorDefaults(t *testing.T) {
	w := run(t, func(c *gin.Context) { responses.Error(c, http.StatusConflict, "", "") })
	body := decodeError(t, w)
	if body["error"] != "conflict" || body["message"] != "Conflict" {
		t.Fatalf("defaults not applied: %v", body)
	}
	w = run(t, func(c *gin.Context) { responses.Error(c, http.StatusBadGateway, "", "") })
	if body = decodeError(t, w); body["error"] != "internal_error" || body["message"] != "Bad Gateway" {
		t.Fatalf("5xx default code wrong: %v", body)
	}
}

func TestErrorWithDetails(t *testing.T) {
	w := run(t, func(c *gin.Context) {
		responses.ErrorWithDetails(c, http.StatusBadRequest, "validation_failed", "Check the fields.", map[string]string{"first_name": "required"})
	})
	body := decodeError(t, w)
	details, ok := body["details"].(map[string]any)
	if !ok || details["first_name"] != "required" {
		t.Fatalf("details missing: %v", body)
	}
}

func TestAbortStopsChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	reached := false
	r.GET("/x", func(c *gin.Context) { responses.Abort(c, http.StatusTooManyRequests, "rate_limited", "Slow down.") }, func(c *gin.Context) { reached = true })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if reached || w.Code != http.StatusTooManyRequests || decodeError(t, w)["error"] != "rate_limited" {
		t.Fatal("Abort must write the envelope and stop later controllers")
	}
}

func TestSuccessHelpers(t *testing.T) {
	if w := run(t, func(c *gin.Context) { responses.Created(c, gin.H{"id": "1"}) }); w.Code != http.StatusCreated || w.Body.String() != `{"id":"1"}` {
		t.Fatalf("Created wrote %d %s", w.Code, w.Body.String())
	}
	if w := run(t, func(c *gin.Context) { responses.NoContent(c) }); w.Code != http.StatusNoContent || w.Body.Len() != 0 {
		t.Fatal("NoContent must have no body")
	}
	w := run(t, func(c *gin.Context) { responses.List[string](c, nil, 20, 0) })
	if w.Code != http.StatusOK || w.Body.String() != `{"items":[],"limit":20,"offset":0}` {
		t.Fatalf("List wrote %s", w.Body.String())
	}
}
