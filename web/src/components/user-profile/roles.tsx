const ROLE_LABELS: Record<string, string> = {
  ADMIN: "Admin",
  ACCOUNTANT: "Accountant",
  COMPLIANCE_AUDITOR: "Compliance auditor",
};

/** Human-readable name for a role code, e.g. COMPLIANCE_AUDITOR → "Compliance auditor". */
export function roleLabel(code: string) {
  return ROLE_LABELS[code] ?? code.charAt(0) + code.slice(1).toLowerCase().replace(/_/g, " ");
}

export function RoleChips({ roles }: { roles: string[] }) {
  if (roles.length === 0) return <span className="pf-muted">No roles assigned</span>;
  return (
    <ul className="pf-chips" aria-label="Roles">
      {roles.map((role) => <li key={role} className={`pf-chip${role === "ADMIN" ? " is-admin" : ""}`}>{roleLabel(role)}</li>)}
    </ul>
  );
}
