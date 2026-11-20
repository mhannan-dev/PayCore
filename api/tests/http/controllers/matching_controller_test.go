package controllers_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"paycore/api/internal/http/controllers"
	"paycore/api/internal/http/middleware"
	"paycore/api/internal/models"
	"paycore/api/internal/server"
)

type matchingHTTPFake struct {
	controllers.Matching
	calls int
}

func (f *matchingHTTPFake) Configure(context.Context, string, string, models.MatchingConfigRequest) error {
	f.calls++
	return nil
}
func (f *matchingHTTPFake) Review(context.Context, string, string, int, models.MatchingReviewRequest) error {
	f.calls++
	return nil
}
func TestMatchingHTTPRequiresAllPermissionsAndStrictInput(t *testing.T) {
	identity := &policyIdentity{user: models.UserDTO{ID: adminID}}
	svc := &matchingHTTPFake{}
	router := server.New(nil, server.Controllers{Matching: controllers.NewMatchingController(svc), AuthMiddleware: middleware.NewAuth(identity)})
	grants := []string{models.PermReconciliationRead, models.PermReconciliationMatch, models.PermLedgerRead}
	request := func(path, body string) int {
		r := httptest.NewRequest("POST", "/api/v1/reconciliation/imports/"+memberID+path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer valid")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w.Code
	}
	for omit := range grants {
		identity.user.Permissions = nil
		for i, p := range grants {
			if i != omit {
				identity.user.Permissions = append(identity.user.Permissions, p)
			}
		}
		for _, path := range []string{"/matching", "/rows/2/match"} {
			if got := request(path, `{}`); got != 403 {
				t.Fatalf("missing %s status %d", grants[omit], got)
			}
		}
	}
	if svc.calls != 0 {
		t.Fatal("unauthorized controller reached")
	}
	identity.user.Permissions = grants
	for _, body := range []string{`{}`, `{"direction_mode":"UNKNOWN"}`, `{"direction_mode":"SAME","extra":true}`, `{"direction_mode":"SAME"} {}`, strings.Repeat(" ", 2049) + `{}`} {
		if got := request("/matching", body); got != 400 {
			t.Fatalf("invalid config status %d", got)
		}
	}
	for _, body := range []string{`{}`, `{"action":"CONFIRM","posting_id":"bad"}`, `{"action":"NOTE","reason":"abc","expected_version":-1}`} {
		if got := request("/rows/2/match", body); got != 400 {
			t.Fatalf("invalid review status %d", got)
		}
	}
	if svc.calls != 0 {
		t.Fatal("invalid input reached service")
	}
	if got := request("/matching", `{"direction_mode":"SAME","expected_version":0}`); got != 204 {
		t.Fatalf("valid config status %d", got)
	}
	if got := request("/rows/2/match", `{"action":"NOTE","reason":"Review bank fee","expected_version":1}`); got != 204 {
		t.Fatalf("valid note status %d", got)
	}
}
