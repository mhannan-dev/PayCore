package seeder

import (
	"strings"
	"testing"
)

func TestDefaultRBACDefinitions(t *testing.T) {
	roles := DefaultRoles()
	if len(roles) != 5 {
		t.Fatalf("expected 5 default roles, got %d", len(roles))
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

	perms := DefaultPermissions()
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

	mappings := DefaultRolePermissions()
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
