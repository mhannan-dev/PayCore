package seeders

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"paycore/api/internal/models"
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
		{Code: models.RoleAdmin, Name: "System Administrator", Description: "Full system administrative, configuration, and user management control"},
		{Code: models.RoleOperator, Name: "Standard Operator", Description: "Basic financial operations and ledger reading"},
		{Code: models.RoleLeadOperator, Name: "Lead Operator", Description: "High-value ledger transfers, batch operations, and transaction reversals"},
		{Code: models.RoleComplianceAuditor, Name: "Compliance Auditor", Description: "Financial audit, zero-sum invariant verification, and compliance reporting"},
		{Code: models.RoleStaffEngineer, Name: "Staff SRE / Engineer", Description: "Infrastructure diagnostics, request simulator, and system telemetry"},
	}
}

func DefaultPermissions() []PermissionDefinition {
	return []PermissionDefinition{
		{Code: models.PermQuantumRead, Name: "Read Own Quantum Invoices", Description: "Read merchant-owned testnet invoices"},
		{Code: models.PermQuantumWrite, Name: "Manage Own Quantum Invoices", Description: "Create and cancel merchant-owned testnet invoices"},
		{Code: models.PermNotificationsRead, Name: "Read Notification Deliveries", Description: "Inspect sandbox notification outcomes"},
		{Code: models.PermNotificationsRetry, Name: "Retry Notification Deliveries", Description: "Requeue failed notification events"},
		{Code: models.PermReconciliationMatch, Name: "Review Own Statement Matches", Description: "Confirm and reject statement matches and record exception notes"},
		{Code: models.PermReconciliationRead, Name: "Read Own Statements", Description: "View owned statement imports"},
		{Code: models.PermReconciliationImport, Name: "Import Own Statements", Description: "Validate and import owned CSV statements"},
		{Code: models.PermUsersRead, Name: "Read Users", Description: "View user profiles, roles, and status"},
		{Code: models.PermUsersWrite, Name: "Manage Users", Description: "Create, update, and assign roles to users"},
		{Code: models.PermTransfersRead, Name: "Read Transfers", Description: "View transfer history and journal entries"},
		{Code: models.PermTransfersCreate, Name: "Create Transfers", Description: "Execute double-entry ledger transfers"},
		{Code: models.PermTransfersReverse, Name: "Reverse Transfers", Description: "Issue compensating reversal entries"},
		{Code: models.PermLedgerRead, Name: "Read Ledger", Description: "View append-only ledger entries and balances"},
		{Code: models.PermAuditRead, Name: "Read Audit", Description: "Inspect system audit trails and outbox events"},
		{Code: models.PermAuditVerify, Name: "Verify Invariants", Description: "Run zero-sum balance and consistency checks"},
		{Code: models.PermReportsRead, Name: "Read Reports", Description: "View financial statements and reconciliation summaries"},
		{Code: models.PermReportsExport, Name: "Export Reports", Description: "Export compliance and transaction reports"},
		{Code: models.PermSimulatorRun, Name: "Run Simulator", Description: "Execute idempotency and load simulation tools"},
		{Code: models.PermSystemRead, Name: "Read System Status", Description: "View system health, database, and broker metrics"},
		{Code: models.PermSystemWrite, Name: "Manage System Delivery", Description: "Replay failed outbox events and retry broker delivery"},
		{Code: models.PermAccountsWrite, Name: "Manage Accounts", Description: "Create ledger accounts in the chart of accounts"},
		{Code: models.PermAttachmentsRead, Name: "Read Own Attachments", Description: "List and download attachments owned by the signed-in user"},
		{Code: models.PermAttachmentsWrite, Name: "Upload Own Attachments", Description: "Upload files and assign the signed-in user photo"},
		{Code: models.PermAttachmentsDelete, Name: "Delete Own Attachments", Description: "Soft-delete attachments owned by the signed-in user"},
	}
}

func DefaultRolePermissions() map[string][]string {
	return map[string][]string{
		models.RoleAdmin: {
			models.PermQuantumRead, models.PermQuantumWrite,
			models.PermNotificationsRetry,
			models.PermNotificationsRead,
			models.PermReconciliationMatch,
			models.PermReconciliationRead, models.PermReconciliationImport,
			models.PermAttachmentsRead, models.PermAttachmentsWrite, models.PermAttachmentsDelete,
			models.PermUsersRead, models.PermUsersWrite,
			models.PermTransfersRead, models.PermTransfersCreate, models.PermTransfersReverse,
			models.PermLedgerRead,
			models.PermAuditRead, models.PermAuditVerify,
			models.PermReportsRead, models.PermReportsExport,
			models.PermSimulatorRun,
			models.PermSystemRead, models.PermSystemWrite,
			models.PermAccountsWrite,
		},
		models.RoleLeadOperator: {
			models.PermNotificationsRead,
			models.PermReconciliationMatch,
			models.PermReconciliationRead, models.PermReconciliationImport,
			models.PermAttachmentsRead, models.PermAttachmentsWrite, models.PermAttachmentsDelete,
			models.PermAccountsWrite,
			models.PermUsersRead,
			models.PermTransfersRead, models.PermTransfersCreate, models.PermTransfersReverse,
			models.PermLedgerRead,
			models.PermAuditRead,
			models.PermReportsRead,
		},
		models.RoleOperator: {
			models.PermNotificationsRead,
			models.PermReconciliationMatch,
			models.PermReconciliationRead, models.PermReconciliationImport,
			models.PermAttachmentsRead, models.PermAttachmentsWrite, models.PermAttachmentsDelete,
			models.PermTransfersRead, models.PermTransfersCreate,
			models.PermLedgerRead,
		},
		models.RoleComplianceAuditor: {
			models.PermNotificationsRead,
			models.PermReconciliationRead,
			models.PermAttachmentsRead, models.PermAttachmentsWrite, models.PermAttachmentsDelete,
			models.PermTransfersRead,
			models.PermLedgerRead,
			models.PermAuditRead, models.PermAuditVerify,
			models.PermReportsRead, models.PermReportsExport,
		},
		models.RoleStaffEngineer: {
			models.PermNotificationsRetry,
			models.PermNotificationsRead,
			models.PermAttachmentsRead, models.PermAttachmentsWrite, models.PermAttachmentsDelete,
			models.PermLedgerRead,
			models.PermAuditRead,
			models.PermSimulatorRun,
			models.PermSystemRead, models.PermSystemWrite,
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
