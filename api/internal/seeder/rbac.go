package seeder

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"paycore/api/internal/model"
)

type RoleDefinition struct {
	Code        string
	Name        string
	Description string
}

type PermissionDefinition struct {
	Code        string
	Name        string
	Description string
}

func DefaultRoles() []RoleDefinition {
	return []RoleDefinition{
		{Code: model.RoleAdmin, Name: "System Administrator", Description: "Full system administrative, configuration, and user management control"},
		{Code: model.RoleOperator, Name: "Standard Operator", Description: "Basic financial operations and ledger reading"},
		{Code: model.RoleLeadOperator, Name: "Lead Operator", Description: "High-value ledger transfers, batch operations, and transaction reversals"},
		{Code: model.RoleComplianceAuditor, Name: "Compliance Auditor", Description: "Financial audit, zero-sum invariant verification, and compliance reporting"},
		{Code: model.RoleStaffEngineer, Name: "Staff SRE / Engineer", Description: "Infrastructure diagnostics, request simulator, and system telemetry"},
	}
}

func DefaultPermissions() []PermissionDefinition {
	return []PermissionDefinition{
		{Code: model.PermReconciliationMatch, Name: "Review Own Statement Matches", Description: "Confirm and reject statement matches and record exception notes"},
		{Code: model.PermReconciliationRead, Name: "Read Own Statements", Description: "View owned statement imports"},
		{Code: model.PermReconciliationImport, Name: "Import Own Statements", Description: "Validate and import owned CSV statements"},
		{Code: model.PermUsersRead, Name: "Read Users", Description: "View user profiles, roles, and status"},
		{Code: model.PermUsersWrite, Name: "Manage Users", Description: "Create, update, and assign roles to users"},
		{Code: model.PermTransfersRead, Name: "Read Transfers", Description: "View transfer history and journal entries"},
		{Code: model.PermTransfersCreate, Name: "Create Transfers", Description: "Execute double-entry ledger transfers"},
		{Code: model.PermTransfersReverse, Name: "Reverse Transfers", Description: "Issue compensating reversal entries"},
		{Code: model.PermLedgerRead, Name: "Read Ledger", Description: "View append-only ledger entries and balances"},
		{Code: model.PermAuditRead, Name: "Read Audit", Description: "Inspect system audit trails and outbox events"},
		{Code: model.PermAuditVerify, Name: "Verify Invariants", Description: "Run zero-sum balance and consistency checks"},
		{Code: model.PermReportsRead, Name: "Read Reports", Description: "View financial statements and reconciliation summaries"},
		{Code: model.PermReportsExport, Name: "Export Reports", Description: "Export compliance and transaction reports"},
		{Code: model.PermSimulatorRun, Name: "Run Simulator", Description: "Execute idempotency and load simulation tools"},
		{Code: model.PermSystemRead, Name: "Read System Status", Description: "View system health, database, and broker metrics"},
		{Code: model.PermAccountsWrite, Name: "Manage Accounts", Description: "Create ledger accounts in the chart of accounts"},
		{Code: model.PermAttachmentsRead, Name: "Read Own Attachments", Description: "List and download attachments owned by the signed-in user"},
		{Code: model.PermAttachmentsWrite, Name: "Upload Own Attachments", Description: "Upload files and assign the signed-in user photo"},
		{Code: model.PermAttachmentsDelete, Name: "Delete Own Attachments", Description: "Soft-delete attachments owned by the signed-in user"},
	}
}

func DefaultRolePermissions() map[string][]string {
	return map[string][]string{
		model.RoleAdmin: {
			model.PermReconciliationMatch,
			model.PermReconciliationRead, model.PermReconciliationImport,
			model.PermAttachmentsRead, model.PermAttachmentsWrite, model.PermAttachmentsDelete,
			model.PermUsersRead, model.PermUsersWrite,
			model.PermTransfersRead, model.PermTransfersCreate, model.PermTransfersReverse,
			model.PermLedgerRead,
			model.PermAuditRead, model.PermAuditVerify,
			model.PermReportsRead, model.PermReportsExport,
			model.PermSimulatorRun,
			model.PermSystemRead,
			model.PermAccountsWrite,
		},
		model.RoleLeadOperator: {
			model.PermReconciliationMatch,
			model.PermReconciliationRead, model.PermReconciliationImport,
			model.PermAttachmentsRead, model.PermAttachmentsWrite, model.PermAttachmentsDelete,
			model.PermAccountsWrite,
			model.PermUsersRead,
			model.PermTransfersRead, model.PermTransfersCreate, model.PermTransfersReverse,
			model.PermLedgerRead,
			model.PermAuditRead,
			model.PermReportsRead,
		},
		model.RoleOperator: {
			model.PermReconciliationMatch,
			model.PermReconciliationRead, model.PermReconciliationImport,
			model.PermAttachmentsRead, model.PermAttachmentsWrite, model.PermAttachmentsDelete,
			model.PermTransfersRead, model.PermTransfersCreate,
			model.PermLedgerRead,
		},
		model.RoleComplianceAuditor: {
			model.PermReconciliationRead,
			model.PermAttachmentsRead, model.PermAttachmentsWrite, model.PermAttachmentsDelete,
			model.PermTransfersRead,
			model.PermLedgerRead,
			model.PermAuditRead, model.PermAuditVerify,
			model.PermReportsRead, model.PermReportsExport,
		},
		model.RoleStaffEngineer: {
			model.PermAttachmentsRead, model.PermAttachmentsWrite, model.PermAttachmentsDelete,
			model.PermLedgerRead,
			model.PermAuditRead,
			model.PermSimulatorRun,
			model.PermSystemRead,
		},
	}
}

// RBAC inserts or updates the default roles, permissions, and role-permission mappings.
func RBAC(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	// Upsert roles
	for _, role := range DefaultRoles() {
		query := `
			INSERT INTO roles (code, name, description)
			VALUES ($1, $2, $3)
			ON CONFLICT (code) DO UPDATE SET
				name = EXCLUDED.name,
				description = EXCLUDED.description,
				updated_at = NOW()
		`
		if _, err := tx.Exec(ctx, query, role.Code, role.Name, role.Description); err != nil {
			return err
		}
	}

	// Upsert permissions
	for _, p := range DefaultPermissions() {
		query := `
			INSERT INTO permissions (code, name, description)
			VALUES ($1, $2, $3)
			ON CONFLICT (code) DO UPDATE SET
				name = EXCLUDED.name,
				description = EXCLUDED.description,
				updated_at = NOW()
		`
		if _, err := tx.Exec(ctx, query, p.Code, p.Name, p.Description); err != nil {
			return err
		}
	}

	// Map role permissions
	for roleCode, permCodes := range DefaultRolePermissions() {
		for _, permCode := range permCodes {
			query := `
				INSERT INTO role_permissions (role_id, permission_id)
				SELECT r.id, p.id
				FROM roles r, permissions p
				WHERE r.code = $1 AND p.code = $2
				ON CONFLICT DO NOTHING
			`
			if _, err := tx.Exec(ctx, query, roleCode, permCode); err != nil {
				return err
			}
		}
	}

	// Include custom permissions as well as the built-in permission definitions.
	if _, err := tx.Exec(ctx, `INSERT INTO role_permissions (role_id, permission_id)
        SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
        WHERE r.code = 'ADMIN' ON CONFLICT DO NOTHING`); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
