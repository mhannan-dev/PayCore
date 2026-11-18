import { Icon } from "../icons";

import type { RolePreset } from "./demo-identities";

function RolePill({ role, selected, onSelect }: { role: RolePreset; selected: boolean; onSelect: (role: RolePreset) => void }) {
  return (
    <button
      type="button"
      onClick={() => onSelect(role)}
      className={`role-pill ${selected ? "selected" : ""}`}
      aria-pressed={selected}
    >
      <div className="role-pill-top">
        <strong className="role-pill-name"><Icon name="user" size={14} />{role.name}</strong>
        <span className={`badge ${role.badge}`}>{role.roleTitle}</span>
      </div>
      <span className="role-pill-dept">
        {role.email} · {role.roles.join(", ")}
      </span>
    </button>
  );
}

export function DemoIdentityPicker({
  roles,
  selectedEmail,
  hasPassword,
  onSelect,
}: {
  roles: RolePreset[];
  selectedEmail: string;
  hasPassword: boolean;
  onSelect: (role: RolePreset) => void;
}) {
  return (
    <div className="role-selection-section">
      <div className="role-selection-header">
        <span>QUICK-FILL DEMO IDENTITIES</span>
        <small>{hasPassword ? "Local seed credentials · click to fill" : "Seeded users · enter your seed password"}</small>
      </div>
      <div className="role-grid">
        {roles.map((role) => (
          <RolePill key={role.id} role={role} selected={selectedEmail === role.email} onSelect={onSelect} />
        ))}
      </div>
    </div>
  );
}
