package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"paycore/api/internal/handler"
	"paycore/api/internal/model"
	"paycore/api/internal/server"
	"paycore/api/internal/service"
)

type policyIdentity struct{ user model.UserDTO }

func (s *policyIdentity) CurrentUser(_ context.Context, token string) (model.UserDTO, error) {
	if token != "valid" {
		return model.UserDTO{}, service.ErrSession
	}
	return s.user, nil
}

// Non-nil test dependencies deliberately have no callable implementation. If a
// guard is missing, the request reaches recovery (500), failing the 403 assertion.
type deniedUsers struct{ handler.UserAdmin }
type deniedAttachments struct{ handler.Attachments }
type deniedLedger struct{ handler.Ledger }
type deniedMatching struct{ handler.Matching }
type deniedStatements struct{ handler.Statements }
type deniedAudit struct{ handler.AuditLog }

func TestEveryPermissionRouteDeniesMissingOrUnrelatedGrants(t *testing.T) {
	identity := &policyIdentity{user: model.UserDTO{ID: adminID, Roles: []string{model.RoleAdmin}}}
	guard := handler.NewGuard(identity)
	r := server.New(nil, server.Handlers{
		Matching:    handler.NewMatchingHandler(&deniedMatching{}, guard),
		Statements:  handler.NewStatementHandler(&deniedStatements{}, guard),
		Users:       handler.NewUsersHandler(&deniedUsers{}, guard),
		Attachments: handler.NewAttachmentHandler(guard, &deniedAttachments{}, nil),
		Ledger:      handler.NewLedgerHandler(&deniedLedger{}, guard),
		Audit:       handler.NewAuditHandler(&deniedAudit{}, guard),
	})
	policy := map[string]string{
		"GET /api/v1/reconciliation/imports/:id/matches":          model.PermReconciliationRead,
		"POST /api/v1/reconciliation/imports/:id/matching":        model.PermReconciliationMatch,
		"POST /api/v1/reconciliation/imports/:id/rows/:row/match": model.PermReconciliationMatch,
		"POST /api/v1/reconciliation/preview":                     model.PermReconciliationImport,
		"POST /api/v1/reconciliation/imports":                     model.PermReconciliationImport,
		"GET /api/v1/reconciliation/imports":                      model.PermReconciliationRead,
		"GET /api/v1/reconciliation/imports/:id":                  model.PermReconciliationRead,
		"GET /api/v1/rbac/roles":                                  model.PermUsersRead, "GET /api/v1/rbac/permissions": model.PermUsersRead,
		"GET /api/v1/users": model.PermUsersRead, "POST /api/v1/users": model.PermUsersWrite,
		"POST /api/v1/users/:id/invitation": model.PermUsersWrite, "PUT /api/v1/users/:id/roles": model.PermUsersWrite,
		"POST /api/v1/users/:id/activate": model.PermUsersWrite, "POST /api/v1/users/:id/deactivate": model.PermUsersWrite, "DELETE /api/v1/users/:id": model.PermUsersWrite,
		"GET /api/v1/audit/events": model.PermAuditRead,
		"GET /api/v1/accounts":     model.PermLedgerRead, "POST /api/v1/accounts": model.PermAccountsWrite,
		"GET /api/v1/accounts/:id": model.PermLedgerRead, "GET /api/v1/accounts/:id/postings": model.PermLedgerRead,
		"GET /api/v1/transfers": model.PermTransfersRead, "GET /api/v1/transfers/:id": model.PermTransfersRead,
		"POST /api/v1/transfers": model.PermTransfersCreate, "POST /api/v1/transfers/:id/reverse": model.PermTransfersReverse,
		"GET /api/v1/audit/zero-sum": model.PermAuditVerify,
		"GET /api/v1/attachments":    model.PermAttachmentsRead, "POST /api/v1/attachments": model.PermAttachmentsWrite,
		"GET /api/v1/attachments/:id": model.PermAttachmentsRead, "GET /api/v1/attachments/:id/content": model.PermAttachmentsRead,
		"DELETE /api/v1/attachments/:id": model.PermAttachmentsDelete, "GET /api/v1/auth/photo": model.PermAttachmentsRead, "PUT /api/v1/auth/photo": model.PermAttachmentsWrite,
	}
	// Explicit exceptions: health and token-entry endpoints are public; current-user
	// identity/name/logout operations verify their bearer session inside AuthService.
	exceptions := map[string]bool{
		"GET /api/v1/auth/security":                         true,
		"POST /api/v1/auth/security/recovery-codes":         true,
		"POST /api/v1/auth/security/authenticator":          true,
		"POST /api/v1/auth/security/authenticator/confirm":  true,
		"POST /api/v1/auth/security/sessions/revoke-others": true,
		"POST /api/v1/auth/security/sessions/:id/revoke":    true,
		"POST /api/v1/auth/mfa/verify":                      true,
		"GET /":                                             true, "GET /health/live": true, "GET /health/ready": true,
		"GET /api/v1/auth/captcha": true, "POST /api/v1/auth/login": true,
		"GET /api/v1/auth/me": true, "PATCH /api/v1/auth/me": true, "POST /api/v1/auth/logout": true,
		"POST /api/v1/auth/invitations/inspect": true, "POST /api/v1/auth/invitations/accept": true,
		"POST /api/v1/auth/password-reset/request": true, "POST /api/v1/auth/password-reset/inspect": true, "POST /api/v1/auth/password-reset/confirm": true,
	}
	seen := map[string]bool{}
	for _, route := range r.Routes() {
		key := route.Method + " " + route.Path
		required, ok := policy[key]
		if !ok {
			if !exceptions[key] {
				t.Errorf("route lacks an audited authorization policy: %s", key)
			}
			continue
		}
		seen[key] = true
		t.Run(key, func(t *testing.T) {
			path := strings.ReplaceAll(route.Path, ":id", memberID)
			for _, authenticated := range []bool{false, true} {
				// Supplying every other capability proves this route checks its exact
				// permission, including when ADMIN appears in the role list.
				identity.user.Permissions = nil
				for _, candidate := range policy {
					if candidate != required {
						identity.user.Permissions = append(identity.user.Permissions, candidate)
					}
				}
				req := httptest.NewRequest(route.Method, path, strings.NewReader("{}"))
				if authenticated {
					req.Header.Set("Authorization", "Bearer valid")
				}
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				want := http.StatusUnauthorized
				if authenticated {
					want = http.StatusForbidden
				}
				if w.Code != want {
					t.Fatalf("want %d got %d: %s", want, w.Code, w.Body)
				}
			}
		})
	}
	for key := range policy {
		if !seen[key] {
			t.Errorf("policy references unregistered route: %s", key)
		}
	}
}
