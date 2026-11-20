package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/middleware"
	"paycore/api/internal/model"
)

type guardIdentity struct{ user model.UserDTO }

func (s *guardIdentity) CurrentUser(context.Context, string) (model.UserDTO, error) {
	return s.user, nil
}

func TestGuardFailsClosedAndUsesCurrentPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name        string
		requirement model.PermissionCode
		grants      []string
		allowed     bool
	}{
		{"exact", model.PermUsersRead, []string{model.PermUsersRead}, true},
		{"missing", model.PermUsersRead, nil, false},
		{"unrelated", model.PermUsersWrite, []string{model.PermUsersRead}, false},
		{"uppercase grant", model.PermUsersRead, []string{"USERS:READ"}, false},
		{"empty requirement", "", []string{""}, false},
		{"unknown requirement", "users:typo", []string{"users:typo"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			identity := &guardIdentity{user: model.UserDTO{ID: "user", Roles: []string{model.RoleAdmin}, Permissions: tc.grants}}
			r := gin.New()
			r.Use(middleware.Errors(nil))
			called := false
			r.GET("/", NewGuard(identity).Require(tc.requirement), func(c *gin.Context) { called = true; c.Status(204) })
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
			if tc.allowed {
				if w.Code != 204 || !called {
					t.Fatal("expected permitted handler")
				}
			} else if w.Code != 403 || called {
				t.Fatalf("guard bypass: %d %s", w.Code, w.Body)
			}
			if tc.allowed {
				identity.user.Permissions = nil
				w = httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
				if w.Code != 403 {
					t.Fatal("revoked permission remained cached")
				}
			}
		})
	}
}

func TestSessionOnlyRoutesNeedIdentityButNoCapability(t *testing.T) {
	for _, id := range []string{"user", ""} {
		identity := &guardIdentity{user: model.UserDTO{ID: id}}
		r := gin.New()
		r.Use(middleware.Errors(nil))
		called := false
		r.GET("/", NewGuard(identity).Session(), func(c *gin.Context) { called = true; c.Status(204) })
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		if id != "" {
			if w.Code != 204 || !called {
				t.Fatal("self-service routes incorrectly require capability grants")
			}
		} else if called {
			t.Fatal("empty identity reached handler")
		}
	}
}

func TestGuardRequireAll(t *testing.T) {
	for _, grants := range [][]string{nil, {model.PermReconciliationImport}, {model.PermAttachmentsRead}, {model.PermReconciliationImport, model.PermAttachmentsRead}} {
		identity := &guardIdentity{user: model.UserDTO{ID: "user", Permissions: grants}}
		r := gin.New()
		r.Use(middleware.Errors(nil))
		r.GET("/", NewGuard(identity).RequireAll(model.PermReconciliationImport, model.PermAttachmentsRead), func(c *gin.Context) { c.Status(204) })
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		want := 403
		if len(grants) == 2 {
			want = 204
		}
		if w.Code != want {
			t.Fatalf("grants %v got %d want %d", grants, w.Code, want)
		}
	}
	r := gin.New()
	r.Use(middleware.Errors(nil))
	r.GET("/", NewGuard(&guardIdentity{user: model.UserDTO{ID: "user"}}).RequireAll(), func(c *gin.Context) { c.Status(204) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 403 {
		t.Fatal("empty requirements bypassed RBAC")
	}
}
