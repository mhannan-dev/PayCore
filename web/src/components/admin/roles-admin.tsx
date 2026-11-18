"use client";

import { Icon } from "../icons";

import { fetchPermissions, fetchRoles, type RoleDetail } from "@/lib/auth-api";
import { Heading, Panel, StatGrid } from "../ui";
import { AdminStatus } from "./admin-status";
import { PermissionMatrix } from "./permission-matrix";
import { useAdminData } from "./use-admin-data";
import "./admin.css";

const loadRolesPage = (token: string) =>
  Promise.all([fetchRoles(token), fetchPermissions(token)]).then(([roles, permissions]) => ({ roles, permissions }));

function RoleCard({ role, total }: { role: RoleDetail; total: number }) {
  const share = total ? Math.round((role.permissions.length / total) * 100) : 0;
  return (
    <article className={`panel adm-role-card${role.code === "ADMIN" ? " is-admin" : ""}`}>
      <header>
        <div>
          <h3>{role.name}</h3>
          <code>{role.code}</code>
        </div>
        <span className="adm-role-users" title="Active users with this role">
          <strong>{role.user_count}</strong> {role.user_count === 1 ? "user" : "users"}
        </span>
      </header>
      {role.description && <p>{role.description}</p>}
      <div className="adm-meter" aria-label={`${role.permissions.length} of ${total} permissions`}>
        <span style={{ width: `${share}%` }} />
      </div>
      <p className="adm-meter-label">{role.permissions.length} of {total} permissions</p>
      <ul className="adm-perm-chips">
        {role.permissions.map((code) => <li key={code}><code>{code}</code></li>)}
      </ul>
    </article>
  );
}

export function RolesAdmin() {
  const { state, reload } = useAdminData(loadRolesPage);
  const heading = (
    <Heading
      eyebrow="ADMINISTRATION"
      title="Roles & permissions"
      description="Each role grants a fixed set of permissions. A user's access is the union of their roles' permissions."
      action={<button type="button" className="button secondary" onClick={reload} disabled={state.status === "loading"}><Icon name="refresh" size={15} />Refresh</button>}
    />
  );
  if (state.status !== "ready") return <div className="adm-page">{heading}<AdminStatus state={state} onRetry={reload} /></div>;

  const { roles, permissions } = state.data;
  return (
    <div className="adm-page">
      {heading}
      <StatGrid three stats={[
        { label: "ROLES", value: String(roles.length), foot: "Assignable to users", icon: "key" },
        { label: "PERMISSIONS", value: String(permissions.length), foot: "Fine-grained capabilities", icon: "shield" },
        { label: "ROLE ASSIGNMENTS", value: String(roles.reduce((n, r) => n + r.user_count, 0)), foot: "Across active users", icon: "users" },
      ]} />
      <div className="adm-role-grid">
        {roles.map((role) => <RoleCard key={role.code} role={role} total={permissions.length} />)}
      </div>
      <Panel title="Permission matrix" description="Which role grants which permission. Role definitions are managed by migrations and the seeder.">
        <PermissionMatrix roles={roles} permissions={permissions} />
      </Panel>
    </div>
  );
}
