package handler_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"paycore/api/internal/handler"
	"paycore/api/internal/model"
	"paycore/api/internal/server"
)

type statementHTTPFake struct {
	handler.Statements
	called int
}

func (f *statementHTTPFake) Preview(_ context.Context, owner string, req model.StatementRequest) (model.StatementPreview, error) {
	f.called++
	return model.StatementPreview{AccountID: req.AccountID, AttachmentID: req.AttachmentID, Rows: []model.StatementRow{}}, nil
}
func TestStatementHTTPRequiresEveryCapabilityAndStrictJSON(t *testing.T) {
	identity := &policyIdentity{user: model.UserDTO{ID: adminID}}
	svc := &statementHTTPFake{}
	router := server.New(nil, server.Handlers{Statements: handler.NewStatementHandler(svc, handler.NewGuard(identity))})
	all := []string{model.PermReconciliationImport, model.PermAttachmentsRead, model.PermLedgerRead}
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
	router := server.New(nil, server.Handlers{})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/reconciliation/imports", nil))
	if w.Code != 503 {
		t.Fatalf("unconfigured status %d", w.Code)
	}
}
