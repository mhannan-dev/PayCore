"use client";

import { useEffect, useId, useRef, useState } from "react";
import { AuthApiError, setUserRoles, type ManagedUser, type RoleDetail } from "@/lib/auth-api";
import { useAuthSession } from "@/lib/auth-session";
import { notify } from "@/lib/notify";
import { Icon } from "../icons";
import { roleLabel } from "../user-profile/roles";
import { displayName } from "./users-table";

type Props = {
  user: ManagedUser;
  roles: RoleDetail[];
  isSelf: boolean;
  onClose: () => void;
  onSaved: (user: ManagedUser) => void;
};

const sameSet = (a: string[], b: Set<string>) => a.length === b.size && a.every((x) => b.has(x));

/** Modal for replacing a user's roles. Mount it only while editing. */
export function RoleEditorDialog({ user, roles, isSelf, onClose, onSaved }: Props) {
  const { session } = useAuthSession();
  const dialog = useRef<HTMLDialogElement>(null);
  const id = useId();
  const [selected, setSelected] = useState(() => new Set(user.roles));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    const element = dialog.current;
    if (element && !element.open) element.showModal();
  }, []);

  // You may not drop your own Admin role (the API rejects it too).
  const lockedAdmin = isSelf && user.roles.includes("ADMIN");
  const granted = [...new Set(roles.filter((r) => selected.has(r.code)).flatMap((r) => r.permissions))].sort();
  const unchanged = sameSet(user.roles, selected);

  function toggle(code: string) {
    setError("");
    setSelected((current) => {
      const next = new Set(current);
      if (next.has(code)) next.delete(code);
      else next.add(code);
      return next;
    });
  }

  async function save() {
    if (selected.size === 0) return setError("Choose at least one role.");
    setSaving(true);
    setError("");
    try {
      // Keep the catalogue order so the stored list is predictable.
      const ordered = roles.map((r) => r.code).filter((code) => selected.has(code));
      const updated = await setUserRoles(session.token, user.id, ordered);
      notify.success(`Roles updated for ${displayName(updated)}.`);
      onSaved(updated);
    } catch (e) {
      setError(e instanceof AuthApiError ? e.message : "Could not save the roles. Try again.");
      setSaving(false);
    }
  }

  return (
    <dialog
      ref={dialog}
      className="adm-dialog"
      aria-labelledby={`${id}-title`}
      aria-busy={saving}
      onCancel={(e) => { e.preventDefault(); if (!saving) onClose(); }}
    >
      <header className="adm-dialog-head">
        <div>
          <p className="eyebrow">EDIT ACCESS</p>
          <h2 id={`${id}-title`}>{displayName(user)}</h2>
          <p className="adm-dialog-sub">{user.email}</p>
        </div>
        <button type="button" className="adm-icon-btn" aria-label="Close" disabled={saving} onClick={onClose}>
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" aria-hidden="true"><path d="m6 6 12 12M18 6 6 18" /></svg>
        </button>
      </header>

      <fieldset className="adm-role-options" disabled={saving}>
        <legend>Roles</legend>
        {roles.map((role) => {
          const checked = selected.has(role.code);
          const locked = lockedAdmin && role.code === "ADMIN";
          return (
            <label key={role.code} className={`adm-role-option${checked ? " is-checked" : ""}${locked ? " is-locked" : ""}`}>
              <input type="checkbox" checked={checked} disabled={locked} onChange={() => toggle(role.code)} />
              <span className="adm-role-check" aria-hidden="true"><Icon name="check" size={13} /></span>
              <span className="adm-role-text">
                <strong>{role.name || roleLabel(role.code)}</strong>
                <code>{role.code}</code>
                {role.description && <small>{role.description}</small>}
                {locked && <small className="adm-locked-note"><Icon name="lock" size={11} /> You can&apos;t remove your own Admin role.</small>}
              </span>
              <span className="adm-role-count">{role.permissions.length} perms</span>
            </label>
          );
        })}
      </fieldset>

      <section className="adm-grant-preview" aria-live="polite">
        <h3>Effective permissions <span>{granted.length}</span></h3>
        {granted.length === 0
          ? <p className="pf-muted">No roles selected.</p>
          : <ul>{granted.map((code) => <li key={code}><code>{code}</code></li>)}</ul>}
      </section>

      {error && <p className="error-message" role="alert">{error}</p>}

      <footer className="adm-dialog-foot">
        <button type="button" className="button secondary" disabled={saving} onClick={onClose}><Icon name="close" size={15} />Cancel</button>
        <button type="button" className="button primary" disabled={saving || unchanged || selected.size === 0} onClick={save}>
          {saving ? <span className="progress-spinner adm-btn-spinner" aria-hidden="true" /> : <Icon name="save" size={15} />}
          {saving ? "Saving…" : "Save roles"}
        </button>
      </footer>
    </dialog>
  );
}
