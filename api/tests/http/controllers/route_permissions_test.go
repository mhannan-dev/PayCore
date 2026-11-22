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
	"paycore/api/internal/services"
)

type policyIdentity struct{ user models.UserDTO }

func (s *policyIdentity) CurrentUser(_ context.Context, token string) (models.UserDTO, error) {
	if token != "valid" {
		return models.UserDTO{}, services.ErrSession
	}
	return s.user, nil
}

// Non-nil test dependencies deliberately have no callable implementation. If a
// guard is missing, the request reaches recovery (500), failing the 403 assertion.
type deniedUsers struct{ controllers.UserAdmin }
type deniedAttachments struct{ controllers.Attachments }
type deniedLedger struct{ controllers.Ledger }
type deniedMatching struct{ controllers.Matching }
type deniedStatements struct{ controllers.Statements }
type deniedAudit struct{ controllers.AuditLog }
type deniedOutbox struct{ controllers.OutboxService }
type deniedNotifications struct{ controllers.Notifications }

type deniedQuantum struct{ controllers.Quantum }

func (d *deniedQuantum) Config() models.QuantumConfig { return models.QuantumConfig{Enabled: true} }

type deniedSettlement struct{ controllers.Settlements }

func TestEveryPermissionRouteDeniesMissingOrUnrelatedGrants(t *testing.T) {
	identity := &policyIdentity{user: models.UserDTO{ID: adminID, Roles: []string{models.RoleAdmin}}}
	guard := middleware.NewAuth(identity)
	r := server.New(nil, server.Controllers{
		AuthMiddleware: guard,
		Matching:       controllers.NewMatchingController(&deniedMatching{}),
		Statements:     controllers.NewStatementController(&deniedStatements{}),
		Users:          controllers.NewUsersController(&deniedUsers{}),
		Attachments:    controllers.NewAttachmentController(&deniedAttachments{}, nil),
		Ledger:         controllers.NewLedgerController(&deniedLedger{}),
		Audit:          controllers.NewAuditController(&deniedAudit{}),
		Outbox:         controllers.NewOutboxController(&deniedOutbox{}),
		Settlement:     controllers.NewSettlementController(&deniedSettlement{}),
		Quantum:        controllers.NewQuantumController(&deniedQuantum{}),
		Notifications:  controllers.NewNotificationController(&deniedNotifications{}),
	})
	policy := map[string]string{
		"GET /api/v1/quantum/merchants/me": models.PermQuantumRead, "POST /api/v1/quantum/merchants": models.PermQuantumWrite,
		"GET /api/v1/quantum/invoices": models.PermQuantumRead, "POST /api/v1/quantum/invoices": models.PermQuantumWrite,
		"GET /api/v1/quantum/invoices/:id": models.PermQuantumRead, "POST /api/v1/quantum/invoices/:id/cancel": models.PermQuantumWrite,
		"GET /api/v1/notifications": models.PermNotificationsRead, "GET /api/v1/notifications/metrics": models.PermNotificationsRead,
		"GET /api/v1/notifications/:id": models.PermNotificationsRead, "POST /api/v1/notifications/:id/retry": models.PermNotificationsRetry,
		"GET /api/v1/reconciliation/imports/:id/matches":          models.PermReconciliationRead,
		"POST /api/v1/reconciliation/imports/:id/matching":        models.PermReconciliationMatch,
		"POST /api/v1/reconciliation/imports/:id/rows/:row/match": models.PermReconciliationMatch,
		"POST /api/v1/reconciliation/preview":                     models.PermReconciliationImport,
		"POST /api/v1/reconciliation/imports":                     models.PermReconciliationImport,
		"GET /api/v1/reconciliation/imports":                      models.PermReconciliationRead,
		"GET /api/v1/reconciliation/imports/:id":                  models.PermReconciliationRead,
		"GET /api/v1/rbac/roles":                                  models.PermUsersRead, "GET /api/v1/rbac/permissions": models.PermUsersRead,
		"GET /api/v1/users": models.PermUsersRead, "POST /api/v1/users": models.PermUsersWrite,
		"POST /api/v1/users/:id/invitation": models.PermUsersWrite, "PUT /api/v1/users/:id/roles": models.PermUsersWrite,
		"POST /api/v1/users/:id/activate": models.PermUsersWrite, "POST /api/v1/users/:id/deactivate": models.PermUsersWrite, "DELETE /api/v1/users/:id": models.PermUsersWrite,
		"GET /api/v1/audit/events": models.PermAuditRead,
		"GET /api/v1/accounts":     models.PermLedgerRead, "POST /api/v1/accounts": models.PermAccountsWrite,
		"GET /api/v1/accounts/:id": models.PermLedgerRead, "GET /api/v1/accounts/:id/postings": models.PermLedgerRead,
		"GET /api/v1/transfers": models.PermTransfersRead, "GET /api/v1/transfers/:id": models.PermTransfersRead,
		"POST /api/v1/transfers": models.PermTransfersCreate, "POST /api/v1/transfers/:id/reverse": models.PermTransfersReverse,
		"GET /api/v1/audit/zero-sum": models.PermAuditVerify,
		"GET /api/v1/outbox/metrics": models.PermSystemRead, "POST /api/v1/outbox/replay": models.PermSystemWrite,
		"GET /api/v1/settlements": models.PermTransfersRead, "GET /api/v1/settlements/:id": models.PermTransfersRead,
		"POST /api/v1/settlements": models.PermTransfersCreate, "POST /api/v1/settlements/:id/capture": models.PermTransfersReverse, "POST /api/v1/settlements/:id/release": models.PermTransfersReverse,
		"GET /api/v1/attachments": models.PermAttachmentsRead, "POST /api/v1/attachments": models.PermAttachmentsWrite,
		"GET /api/v1/attachments/:id": models.PermAttachmentsRead, "GET /api/v1/attachments/:id/content": models.PermAttachmentsRead,
		"DELETE /api/v1/attachments/:id": models.PermAttachmentsDelete, "GET /api/v1/auth/photo": models.PermAttachmentsRead, "PUT /api/v1/auth/photo": models.PermAttachmentsWrite,
	}
	// Explicit exceptions: health and token-entry endpoints are public; current-user
	// identity/name/logout operations verify their bearer session inside AuthService.
	exceptions := map[string]bool{
		"GET /api/v1/quantum/config": true, "GET /api/v1/quantum/checkout/:token": true, "GET /api/v1/quantum/checkout/:token/status": true,
		"POST /api/v1/quantum/checkout/:token/attempts": true, "POST /api/v1/quantum/checkout/:token/attempts/:id/tx": true,
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
