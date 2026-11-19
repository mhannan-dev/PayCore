// Permission codes (mirror api/internal/model/rbac.go). The API enforces them;
// the web app uses them to hide navigation and guard pages.
export const PERMISSIONS = {
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
  accountsWrite: "accounts:write",
} as const;

export type Permission = (typeof PERMISSIONS)[keyof typeof PERMISSIONS];

export const ROLE_CODES = ["ADMIN", "OPERATOR", "LEAD_OPERATOR", "COMPLIANCE_AUDITOR", "STAFF_ENGINEER"] as const;

/** Permission each route needs; routes not listed are open to every signed-in user. */
export const ROUTE_PERMISSIONS: Record<string, Permission> = {
  "/accounts": PERMISSIONS.ledgerRead,
  "/transfers": PERMISSIONS.transfersRead,
  "/ledger": PERMISSIONS.ledgerRead,
  "/audit": PERMISSIONS.auditRead,
  "/reports": PERMISSIONS.reportsRead,
  "/simulator": PERMISSIONS.simulatorRun,
  "/admin/users": PERMISSIONS.usersRead,
  "/admin/roles": PERMISSIONS.usersRead,
  "/admin/audit-log": PERMISSIONS.auditRead,
};

/**
 * True when the permission list grants `permission`. A null list means the
 * permissions are not known yet (an older stored session that /auth/me has not
 * refreshed); it is treated as allowed so the UI does not flash "access denied".
 */
export function hasPermission(permissions: readonly string[] | null, permission: string | undefined) {
  if (!permission || permissions === null) return true;
  return permissions.includes(permission);
}

export const canVisit = (permissions: readonly string[] | null, pathname: string) =>
  hasPermission(permissions, ROUTE_PERMISSIONS[pathname]);

const PERMISSION_GROUP_LABELS: Record<string, string> = {
  users: "User management",
  transfers: "Transfers",
  ledger: "Ledger",
  audit: "Audit",
  reports: "Reports",
  simulator: "Simulator",
  system: "System",
};

/** "transfers:create" → "Transfers"; used to group permissions in the matrix. */
export const permissionGroup = (code: string) => {
  const [area = ""] = code.split(":");
  return PERMISSION_GROUP_LABELS[area] ?? area;
};
