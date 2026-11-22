package models_test

import (
	"paycore/api/internal/models"
	"testing"
)

func TestRBACPermissions(t *testing.T) {
	perms := []string{models.PermUsersRead, models.PermTransfersCreate, models.PermLedgerRead}

	if !models.HasPermission(perms, models.PermUsersRead) {
		t.Fatal("expected users:read to be present")
	}
	if models.HasPermission(perms, "USERS:READ") || models.HasPermission([]string{" users:read "}, models.PermUsersRead) {
		t.Fatal("malformed permission codes must not grant access")
	}
	if models.HasPermission([]string{""}, "") || models.HasAllPermissions(perms) {
		t.Fatal("empty permission requirements must deny access")
	}
	if models.HasPermission(perms, models.PermUsersWrite) {
		t.Fatal("expected users:write to be absent")
	}

	if !models.HasAnyPermission(perms, models.PermUsersWrite, models.PermTransfersCreate) {
		t.Fatal("expected HasAnyPermission to return true when one matches")
	}
	if models.HasAnyPermission(perms, models.PermUsersWrite, models.PermAuditVerify) {
		t.Fatal("expected HasAnyPermission to return false when none match")
	}

	if !models.HasAllPermissions(perms, models.PermUsersRead, models.PermLedgerRead) {
		t.Fatal("expected HasAllPermissions to return true when all match")
	}
	if models.HasAllPermissions(perms, models.PermUsersRead, models.PermUsersWrite) {
		t.Fatal("expected HasAllPermissions to return false when one is missing")
	}
}

func TestRBACRoleConstants(t *testing.T) {
	roles := []string{models.RoleAdmin, models.RoleAccountant, models.RoleComplianceAuditor}
	normalized, err := models.NormalizeRoles(roles)
	if err != nil {
		t.Fatalf("expected all standard role constants to be valid: %v", err)
	}
	if len(normalized) != 3 {
		t.Fatalf("expected 3 unique normalized roles, got %d", len(normalized))
	}
	for _, legacy := range []string{"OPERATOR", "LEAD_OPERATOR", "STAFF_ENGINEER"} {
		if _, err := models.NormalizeRoles([]string{legacy}); err == nil {
			t.Fatalf("legacy role accepted: %s", legacy)
		}
	}
}
