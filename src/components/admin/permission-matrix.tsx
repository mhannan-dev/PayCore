import type { PermissionDef, RoleDetail } from "@/lib/auth-api";
import { permissionGroup } from "@/lib/rbac";
import { Icon } from "../icons";
import { roleLabel } from "../user-profile/roles";

/** Permissions (rows, grouped by area) × roles (columns). */
export function PermissionMatrix({ roles, permissions }: { roles: RoleDetail[]; permissions: PermissionDef[] }) {
  const groups = new Map<string, PermissionDef[]>();
  for (const p of permissions) {
    const group = permissionGroup(p.code);
    groups.set(group, [...(groups.get(group) ?? []), p]);
  }
  const grants = new Map(roles.map((r) => [r.code, new Set(r.permissions)]));

  return (
    <div className="table-scroll">
      <table className="adm-table adm-matrix">
        <thead>
          <tr>
            <th scope="col">PERMISSION</th>
            {roles.map((r) => <th key={r.code} scope="col" className="adm-matrix-role">{roleLabel(r.code).toUpperCase()}</th>)}
          </tr>
        </thead>
        {[...groups].map(([group, items]) => (
          <tbody key={group}>
            <tr className="adm-matrix-group"><th scope="rowgroup" colSpan={roles.length + 1}>{group}</th></tr>
            {items.map((p) => (
              <tr key={p.code}>
                <th scope="row">
                  <span className="cell-primary">{p.name}</span>
                  <code className="cell-secondary">{p.code}</code>
                </th>
                {roles.map((r) => {
                  const has = grants.get(r.code)?.has(p.code) ?? false;
                  return (
                    <td key={r.code} className={`adm-matrix-cell${has ? " is-granted" : ""}`}>
                      {has ? <span className="adm-tick"><Icon name="check" size={13} /></span> : <span className="adm-dash" aria-hidden="true">—</span>}
                      <span className="sr-only">{has ? `${roleLabel(r.code)} has ${p.code}` : `${roleLabel(r.code)} lacks ${p.code}`}</span>
                    </td>
                  );
                })}
              </tr>
            ))}
          </tbody>
        ))}
      </table>
    </div>
  );
}
