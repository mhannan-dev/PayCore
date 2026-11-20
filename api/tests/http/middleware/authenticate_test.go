package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"paycore/api/internal/http/middleware"
	"testing"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/models"
)

type authIdentity struct{ user models.UserDTO }

func (s *authIdentity) CurrentUser(context.Context, string) (models.UserDTO, error) {
	return s.user, nil
}

func TestAuthFailsClosedAndUsesCurrentPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name        string
		requirement models.PermissionCode
		grants      []string
		allowed     bool
	}{
		{"exact", models.PermUsersRead, []string{models.PermUsersRead}, true},
		{"missing", models.PermUsersRead, nil, false},
		{"unrelated", models.PermUsersWrite, []string{models.PermUsersRead}, false},
		{"uppercase grant", models.PermUsersRead, []string{"USERS:READ"}, false},
		{"empty requirement", "", []string{""}, false},
		{"unknown requirement", "users:typo", []string{"users:typo"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			identity := &authIdentity{user: models.UserDTO{ID: "user", Roles: []string{models.RoleAdmin}, Permissions: tc.grants}}
			r := gin.New()
			r.Use(middleware.Errors(nil))
			called := false
			r.GET("/", middleware.NewAuth(identity).Can(tc.requirement), func(c *gin.Context) { called = true; c.Status(204) })
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
			if tc.allowed {
				if w.Code != 204 || !called {
					t.Fatal("expected permitted controller")
				}
			} else if w.Code != 403 || called {
				t.Fatalf("auth bypass: %d %s", w.Code, w.Body)
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
		identity := &authIdentity{user: models.UserDTO{ID: id}}
		r := gin.New()
		r.Use(middleware.Errors(nil))
		called := false
		r.GET("/", middleware.NewAuth(identity).Authenticate(), func(c *gin.Context) { called = true; c.Status(204) })
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		if id != "" {
			if w.Code != 204 || !called {
				t.Fatal("self-service routes incorrectly require capability grants")
			}
		} else if called {
			t.Fatal("empty identity reached controller")
		}
	}
}

func TestAuthRequireAll(t *testing.T) {
	for _, grants := range [][]string{nil, {models.PermReconciliationImport}, {models.PermAttachmentsRead}, {models.PermReconciliationImport, models.PermAttachmentsRead}} {
		identity := &authIdentity{user: models.UserDTO{ID: "user", Permissions: grants}}
		r := gin.New()
		r.Use(middleware.Errors(nil))
		r.GET("/", middleware.NewAuth(identity).CanAll(models.PermReconciliationImport, models.PermAttachmentsRead), func(c *gin.Context) { c.Status(204) })
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
	r.GET("/", middleware.NewAuth(&authIdentity{user: models.UserDTO{ID: "user"}}).CanAll(), func(c *gin.Context) { c.Status(204) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 403 {
		t.Fatal("empty requirements bypassed RBAC")
	}
}
