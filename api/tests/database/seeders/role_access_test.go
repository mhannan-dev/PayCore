package seeders_test

import (
	"slices"
	"testing"

	"paycore/api/database/seeders"
	"paycore/api/internal/models"
)

// TestDefaultRoleAccessMatrix pins the exact default grants of every built-in
// role (migrations 026-028). Changing a role's access must change this test,
// docs/rbac.md and a migration together.
func TestDefaultRoleAccessMatrix(t *testing.T) {
	want := map[string][]string{
		models.RoleAccountant: {
			models.PermAttachmentsDelete, models.PermAttachmentsRead, models.PermAttachmentsWrite,
			models.PermLedgerRead, models.PermNotificationsRead,
			models.PermQuantumRead, models.PermQuantumWrite,
			models.PermReconciliationImport, models.PermReconciliationMatch, models.PermReconciliationRead,
			models.PermReportsRead,
			models.PermTransfersCreate, models.PermTransfersRead,
			models.PermUsersRead,
		},
		models.RoleComplianceAuditor: {
			models.PermAttachmentsDelete, models.PermAttachmentsRead, models.PermAttachmentsWrite,
			models.PermAuditRead, models.PermAuditVerify,
			models.PermLedgerRead, models.PermNotificationsRead,
			models.PermReconciliationRead,
			models.PermReportsExport, models.PermReportsRead,
			models.PermSystemRead,
			models.PermTransfersRead,
			models.PermUsersRead,
		},
	}
	grants := seeders.DefaultRolePermissions()
	for role, expected := range want {
		got := slices.Clone(grants[role])
		slices.Sort(got)
		slices.Sort(expected)
		if !slices.Equal(got, expected) {
			t.Errorf("%s grants:\n got  %v\n want %v", role, got, expected)
		}
	}
}

// TestEveryRoleHasDefaultAccess makes sure no built-in role signs in to an empty console.
func TestEveryRoleHasDefaultAccess(t *testing.T) {
	grants := seeders.DefaultRolePermissions()
	for _, role := range seeders.DefaultRoles() {
		perms := grants[role.Code]
		if !models.HasAllPermissions(perms, models.PermLedgerRead, models.PermAttachmentsRead, models.PermUsersRead) {
			t.Errorf("%s lacks the baseline ledger, own-file and directory read access", role.Code)
		}
	}
	all := seeders.DefaultPermissions()
	for _, p := range all {
		if !models.HasPermission(grants[models.RoleAdmin], p.Code) {
			t.Errorf("ADMIN lacks %s", p.Code)
		}
	}
}

// TestDefaultSeparationOfDuties keeps privileged actions with ADMIN only.
func TestDefaultSeparationOfDuties(t *testing.T) {
	grants := seeders.DefaultRolePermissions()
	adminOnly := []string{
		models.PermUsersWrite, models.PermAccountsWrite, models.PermTransfersReverse,
		models.PermNotificationsRetry, models.PermSystemWrite, models.PermSimulatorRun,
	}
	for _, role := range []string{models.RoleAccountant, models.RoleComplianceAuditor} {
		for _, p := range adminOnly {
			if models.HasPermission(grants[role], p) {
				t.Errorf("%s must not hold admin-only %s", role, p)
			}
		}
	}
	// Auditors stay read-only: no money movement, imports, matching or invoicing.
	for _, p := range []string{models.PermTransfersCreate, models.PermReconciliationImport, models.PermReconciliationMatch, models.PermQuantumWrite} {
		if models.HasPermission(grants[models.RoleComplianceAuditor], p) {
			t.Errorf("COMPLIANCE_AUDITOR must not hold %s", p)
		}
	}
	// Accountants post money but do not read the security audit trail (sign-in IPs).
	if models.HasPermission(grants[models.RoleAccountant], models.PermAuditRead) {
		t.Error("ACCOUNTANT must not read the audit trail")
	}
}
