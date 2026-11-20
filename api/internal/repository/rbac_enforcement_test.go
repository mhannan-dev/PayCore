package repository_test

import (
	"context"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/handler"
	"paycore/api/internal/middleware"
	"paycore/api/internal/model"
	"paycore/api/internal/repository"
	"paycore/api/internal/seeder"
	"paycore/api/internal/server"
	"paycore/api/internal/service"
)

func TestRBACRevocationAndRoleChangesAffectExistingSession(t *testing.T) {
	pool := isolatedPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	users := repository.NewUserRepository(pool)
	rbac := repository.NewRBACRepository(pool)
	if _, err := users.Create(ctx, "Admin", nil, "policy-admin@example.test", "fixture hash", []string{model.RoleAdmin}); err != nil {
		t.Fatal(err)
	}
	u, err := users.Create(ctx, "Authorization", nil, "authorization@example.test", "fixture hash", []string{model.RoleOperator})
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("a", 64)
	hash := sha256.Sum256([]byte(token))
	if err = users.CreateSession(ctx, u.ID, u.PasswordHash, hash[:], time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	auth, err := service.NewAuthService(users, nil)
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.Errors(server.ErrorTable))
	called := 0
	router.GET("/", handler.NewGuard(auth).Require(model.PermAttachmentsRead), func(c *gin.Context) { called++; c.Status(204) })
	check := func(want int) {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, request)
		if w.Code != want {
			t.Fatalf("wanted %d got %d: %s", want, w.Code, w.Body)
		}
	}
	check(204)
	if _, err = pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=(SELECT id FROM roles WHERE code='OPERATOR') AND permission_id=(SELECT id FROM permissions WHERE code='attachments:read')`); err != nil {
		t.Fatal(err)
	}
	check(403) // Same session; no re-login or cached grant.
	if _, err = rbac.SetUserRoles(ctx, u.ID, []string{model.RoleOperator, model.RoleComplianceAuditor}); err != nil {
		t.Fatal(err)
	}
	check(204) // Grants are the union of the current roles.
	if _, err = rbac.SetUserActive(ctx, u.ID, false); err != nil {
		t.Fatal(err)
	}
	check(401)
	if called != 2 {
		t.Fatal("unauthorized request reached the use case")
	}
}

func TestRBACPermissionSourcesAndDefaultMatrix(t *testing.T) {
	pool := isolatedPool(t)
	ctx := context.Background()
	rbac := repository.NewRBACRepository(pool)
	for role, expected := range seeder.DefaultRolePermissions() {
		rows, err := pool.Query(ctx, `SELECT p.code FROM permissions p JOIN role_permissions rp ON rp.permission_id=p.id JOIN roles r ON r.id=rp.role_id WHERE r.code=$1 ORDER BY p.code`, role)
		if err != nil {
			t.Fatal(err)
		}
		var actual []string
		for rows.Next() {
			var code string
			if err = rows.Scan(&code); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			actual = append(actual, code)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			t.Fatal(err)
		}
		want := slices.Clone(expected)
		slices.Sort(want)
		if !slices.Equal(actual, want) {
			t.Fatalf("migration/seeder policy drift for %s: actual=%v expected=%v", role, actual, want)
		}
	}
	u, err := repository.NewUserRepository(pool).Create(ctx, "Source", nil, "source@example.test", "fixture hash", []string{model.RoleOperator})
	if err != nil {
		t.Fatal(err)
	}
	// A stale legacy mirror must never silently elevate or disagree with auth.
	if _, err = pool.Exec(ctx, `DELETE FROM user_roles WHERE user_id=$1;`, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO user_roles(user_id,role_id) SELECT $1,id FROM roles WHERE code='ADMIN'`, u.ID); err != nil {
		t.Fatal(err)
	}
	grants, err := rbac.FindPermissionsByUserID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if model.HasPermission(grants, model.PermUsersWrite) || !model.HasPermission(grants, model.PermTransfersCreate) {
		t.Fatalf("legacy mirror changed effective permissions: %v", grants)
	}
	roles, err := rbac.FindRolesByUserID(ctx, u.ID)
	if err != nil || len(roles) != 1 || roles[0].Code != model.RoleOperator {
		t.Fatalf("legacy mirror changed effective roles: %v %v", roles, err)
	}
	for _, definition := range seeder.DefaultPermissions() {
		if !model.PermissionCode(definition.Code).Valid() {
			t.Fatalf("route catalog missing %s", definition.Code)
		}
	}
}
