package seeders_test

import (
	"paycore/api/database/seeders"
	"paycore/api/internal/models"
	"strings"
	"testing"
)

func TestDefaultUserDirectoryAccess(t *testing.T) {
	grants := seeders.DefaultRolePermissions()
	for _, role := range []string{models.RoleAccountant, models.RoleComplianceAuditor} {
		if !models.HasPermission(grants[role], models.PermUsersRead) {
			t.Errorf("%s must be able to read users and role catalogues", role)
		}
		if models.HasPermission(grants[role], models.PermUsersWrite) {
			t.Errorf("%s must not manage other users", role)
		}
	}
	if !models.HasAllPermissions(grants[models.RoleAdmin], models.PermUsersRead, models.PermUsersWrite) {
		t.Fatal("ADMIN must retain user management access")
	}
}

func TestDefaultRBACDefinitions(t *testing.T) {
	roles := seeders.DefaultRoles()
	if len(roles) != 3 {
		t.Fatalf("expected 3 default roles, got %d", len(roles))
	}
	roleMap := make(map[string]bool)
	for _, r := range roles {
		if r.Code == "" || r.Code != strings.ToUpper(r.Code) {
			t.Fatalf("role code must be uppercase and non-empty: %q", r.Code)
		}
		if r.Name == "" {
			t.Fatalf("role name cannot be empty for %s", r.Code)
		}
		roleMap[r.Code] = true
	}
	for _, legacy := range []string{"OPERATOR", "LEAD_OPERATOR", "STAFF_ENGINEER"} {
		if roleMap[legacy] {
			t.Fatalf("legacy role is still a default: %s", legacy)
		}
	}

	perms := seeders.DefaultPermissions()
	if len(perms) < 10 {
		t.Fatalf("expected at least 10 default permissions, got %d", len(perms))
	}
	permMap := make(map[string]bool)
	for _, p := range perms {
		if p.Code == "" || p.Code != strings.ToLower(p.Code) {
			t.Fatalf("permission code must be lowercase: %q", p.Code)
		}
		if !strings.Contains(p.Code, ":") {
			t.Fatalf("permission code must follow domain:action convention: %q", p.Code)
		}
		permMap[p.Code] = true
	}

	mappings := seeders.DefaultRolePermissions()
	for roleCode, assignedPerms := range mappings {
		if !roleMap[roleCode] {
			t.Fatalf("mapping references unknown role code: %q", roleCode)
		}
		for _, permCode := range assignedPerms {
			if !permMap[permCode] {
				t.Fatalf("mapping for role %s references unknown permission code: %q", roleCode, permCode)
			}
		}
	}
}
