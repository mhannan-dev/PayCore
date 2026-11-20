package model

import "testing"

func TestRBACPermissions(t *testing.T) {
	perms := []string{PermUsersRead, PermTransfersCreate, PermLedgerRead}

	if !HasPermission(perms, PermUsersRead) {
		t.Fatal("expected users:read to be present")
	}
	if HasPermission(perms, "USERS:READ") || HasPermission([]string{" users:read "}, PermUsersRead) {
		t.Fatal("malformed permission codes must not grant access")
	}
	if HasPermission([]string{""}, "") || HasAllPermissions(perms) {
		t.Fatal("empty permission requirements must deny access")
	}
	if HasPermission(perms, PermUsersWrite) {
		t.Fatal("expected users:write to be absent")
	}

	if !HasAnyPermission(perms, PermUsersWrite, PermTransfersCreate) {
		t.Fatal("expected HasAnyPermission to return true when one matches")
	}
	if HasAnyPermission(perms, PermUsersWrite, PermAuditVerify) {
		t.Fatal("expected HasAnyPermission to return false when none match")
	}

	if !HasAllPermissions(perms, PermUsersRead, PermLedgerRead) {
		t.Fatal("expected HasAllPermissions to return true when all match")
	}
	if HasAllPermissions(perms, PermUsersRead, PermUsersWrite) {
		t.Fatal("expected HasAllPermissions to return false when one is missing")
	}
}

func TestRBACRoleConstants(t *testing.T) {
	roles := []string{RoleAdmin, RoleOperator, RoleLeadOperator, RoleComplianceAuditor, RoleStaffEngineer}
	normalized, err := NormalizeRoles(roles)
	if err != nil {
		t.Fatalf("expected all standard role constants to be valid: %v", err)
	}
	if len(normalized) != 5 {
		t.Fatalf("expected 5 unique normalized roles, got %d", len(normalized))
	}
}
