// Permission codes (mirror api/internal/models/rbac.go). The API enforces them;
// the web app uses them to hide navigation and guard pages.
export const PERMISSIONS = {
 notificationsRead: "notifications:read",
 notificationsRetry: "notifications:retry",
  reconciliationMatch: "reconciliation:match",
  reconciliationRead: "reconciliation:read",
  reconciliationImport: "reconciliation:import",
  usersRead: "users:read",
  usersWrite: "users:write",
  transfersRead: "transfers:read",
  transfersCreate: "transfers:create",
  transfersReverse: "transfers:reverse",
  ledgerRead: "ledger:read",
  auditRead: "audit:read",
  auditVerify: "audit:verify",
  reportsRead: "reports:read",
  reportsExport: "reports:export",
  simulatorRun: "simulator:run",
  systemRead: "system:read",
  systemWrite: "system:write",
  accountsWrite: "accounts:write",
  attachmentsRead: "attachments:read",
  attachmentsWrite: "attachments:write",
  attachmentsDelete: "attachments:delete",
} as const;

export type Permission = (typeof PERMISSIONS)[keyof typeof PERMISSIONS];

export const ROLE_CODES = ["ADMIN", "OPERATOR", "LEAD_OPERATOR", "COMPLIANCE_AUDITOR", "STAFF_ENGINEER"] as const;

/** Permission each route needs; routes not listed are open to every signed-in user. */
export const ROUTE_PERMISSIONS: Record<string, Permission> = {
 "/notifications": PERMISSIONS.notificationsRead,
  "/reconciliation": PERMISSIONS.reconciliationRead,
  "/accounts": PERMISSIONS.ledgerRead,
  "/transfers": PERMISSIONS.transfersRead,
  "/settlements": PERMISSIONS.transfersRead,
  "/ledger": PERMISSIONS.ledgerRead,
  "/audit": PERMISSIONS.auditRead,
  "/reports": PERMISSIONS.reportsRead,
  "/simulator": PERMISSIONS.simulatorRun,
  "/admin/users": PERMISSIONS.usersRead,
  "/admin/roles": PERMISSIONS.usersRead,
  "/admin/audit-log": PERMISSIONS.auditRead,
  "/system/outbox": PERMISSIONS.systemRead,
};

/**
 * True when the permission list grants `permission`. A null list means the
 * permissions are not known yet; protected controls stay unavailable until
 * /auth/me supplies the current grants.
 */
export function hasPermission(permissions: readonly string[] | null, permission: string | undefined) {
  if (permission === undefined) return true;
  return permission.length > 0 && permissions !== null && permissions.includes(permission);
}

/** Nested pages inherit the closest protected parent, using path boundaries. */
export function permissionForRoute(pathname: string): Permission | undefined {
  let required: Permission | undefined;
  let matchedLength = 0;
  for (const [route, permission] of Object.entries(ROUTE_PERMISSIONS)) {
    if ((pathname === route || pathname.startsWith(`${route}/`)) && route.length > matchedLength) {
      required = permission;
      matchedLength = route.length;
    }
  }
  return required;
}

export const canVisit = (permissions: readonly string[] | null, pathname: string) =>
  hasPermission(permissions, permissionForRoute(pathname));

const PERMISSION_GROUP_LABELS: Record<string, string> = {
 notifications: "Notifications",
  reconciliation: "Reconciliation",
  users: "User management",
  transfers: "Transfers",
  ledger: "Ledger",
  audit: "Audit",
  reports: "Reports",
  simulator: "Simulator",
  system: "System",
  attachments: "Own attachments",
  accounts: "Accounts",
};

/** "transfers:create" → "Transfers"; used to group permissions in the matrix. */
export const permissionGroup = (code: string) => {
  const [area = ""] = code.split(":");
  return PERMISSION_GROUP_LABELS[area] ?? area;
};
