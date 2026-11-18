import type { ReactNode } from "react";
import { Icon } from "../icons";
import { Panel } from "../ui";
import { RoleChips } from "./roles";

function Row({ icon, label, children }: { icon: string; label: string; children: ReactNode }) {
  return (
    <div className="pf-row">
      <span className="pf-row-icon"><Icon name={icon} size={16} /></span>
      <div className="pf-row-text">
        <span className="pf-row-label">{label}</span>
        <div className="pf-row-value">{children}</div>
      </div>
    </div>
  );
}

/** Read-only account facts. */
export function AccountCard({ roles, permissions }: { roles: string[]; permissions: string[] | null }) {
  return (
    <Panel title="Account" description="Managed by your PayCore administrator.">
      <div className="pf-rows">
        <Row icon="key" label="Roles"><RoleChips roles={roles} /></Row>
        <Row icon="grid" label="Workspace">PayCore</Row>
        <Row icon="code" label="Environment">Sandbox preview</Row>
        <Row icon="shield" label={permissions ? `Permissions (${permissions.length})` : "Permissions"}>
          {permissions === null
            ? <span className="pf-muted">Loading…</span>
            : permissions.length === 0
              ? <span className="pf-muted">No permissions granted</span>
              : <ul className="pf-perms">{permissions.map((p) => <li key={p}><code>{p}</code></li>)}</ul>}
        </Row>
      </div>
    </Panel>
  );
}
