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

type statementHTTPFake struct {
	controllers.Statements
	called int
}

func (f *statementHTTPFake) Preview(_ context.Context, owner string, req models.StatementRequest) (models.StatementPreview, error) {
	f.called++
	return models.StatementPreview{AccountID: req.AccountID, AttachmentID: req.AttachmentID, Rows: []models.StatementRow{}}, nil
}
func TestStatementHTTPRequiresEveryCapabilityAndStrictJSON(t *testing.T) {
	identity := &policyIdentity{user: models.UserDTO{ID: adminID}}
	svc := &statementHTTPFake{}
	router := server.New(nil, server.Controllers{Statements: controllers.NewStatementController(svc), AuthMiddleware: middleware.NewAuth(identity)})
	all := []string{models.PermReconciliationImport, models.PermAttachmentsRead, models.PermLedgerRead}
	body := `{"account_id":"` + adminID + `","attachment_id":"` + memberID + `"}`
	request := func(data string) int {
		req := httptest.NewRequest("POST", "/api/v1/reconciliation/preview", strings.NewReader(data))
		req.Header.Set("Authorization", "Bearer valid")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code
	}
	for omit := range all {
		identity.user.Permissions = nil
		for i, p := range all {
			if i != omit {
				identity.user.Permissions = append(identity.user.Permissions, p)
			}
		}
		if got := request(body); got != 403 {
			t.Fatalf("missing %s status %d", all[omit], got)
		}
	}
	if svc.called != 0 {
		t.Fatal("unauthorized request reached service")
	}
	identity.user.Permissions = all
	for _, invalid := range []string{`{}`, `{"account_id":"bad","attachment_id":"bad"}`, strings.TrimSuffix(body, "}") + `,"extra":true}`, body + ` {}`, strings.Repeat(" ", 2049) + body} {
		if got := request(invalid); got != 400 {
			t.Fatalf("invalid payload status %d", got)
		}
	}
	if svc.called != 0 {
		t.Fatal("invalid request reached service")
	}
	if got := request(body); got != 200 || svc.called != 1 {
		t.Fatalf("valid request status %d calls %d", got, svc.called)
	}
}

func TestStatementRoutesUnavailableWithoutDependencies(t *testing.T) {
	router := server.New(nil, server.Controllers{})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/reconciliation/imports", nil))
	if w.Code != 503 {
		t.Fatalf("unconfigured status %d", w.Code)
	}
}
