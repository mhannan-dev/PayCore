"use client";

import { useEffect, useId, useRef, useState } from "react";
import { AuthApiError, deleteUser, type ManagedUser } from "@/lib/auth-api";
import { useAuthSession } from "@/lib/auth-session";
import { notify } from "@/lib/notify";
import { Icon } from "../icons";
import { displayName } from "./users-table";

type Props = { user: ManagedUser; onClose: () => void; onDeleted: (user: ManagedUser) => void };

/** Confirms and permanently deletes a user. Mount it only while open. */
export function DeleteUserDialog({ user, onClose, onDeleted }: Props) {
  const { session } = useAuthSession();
  const dialog = useRef<HTMLDialogElement>(null);
  const id = useId();
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState("");
  const name = displayName(user);

  useEffect(() => {
    const element = dialog.current;
    if (element && !element.open) element.showModal();
  }, []);

  async function confirm() {
    setDeleting(true);
    setError("");
    try {
      await deleteUser(session.token, user.id);
      notify.success(`${name} was deleted.`);
      onDeleted(user);
    } catch (e) {
      // Already gone: treat it as deleted so the list catches up.
      if (e instanceof AuthApiError && e.code === "user_not_found") return onDeleted(user);
      setError(e instanceof AuthApiError ? e.message : "Could not delete the user. Try again.");
      setDeleting(false);
    }
  }

  return (
    <dialog
      ref={dialog}
      className="adm-dialog adm-dialog-danger"
      aria-labelledby={`${id}-title`}
      aria-describedby={`${id}-desc`}
      aria-busy={deleting}
      onCancel={(e) => { e.preventDefault(); if (!deleting) onClose(); }}
    >
      <div className="adm-danger-body">
        <span className="adm-danger-icon" aria-hidden="true"><Icon name="trash" size={24} /></span>
        <h2 id={`${id}-title`}>Delete {name}?</h2>
        <p id={`${id}-desc`}>
          <strong>{user.email}</strong> will be removed permanently. They are signed out everywhere and lose all access;
          their roles, pending invitation, contact details and uploaded-file records are deleted too.
          This cannot be undone.
        </p>
        {error && <p className="error-message" role="alert">{error}</p>}
      </div>
      <footer className="adm-dialog-foot">
        <button type="button" className="button secondary" disabled={deleting} onClick={onClose} autoFocus>Cancel</button>
        <button type="button" className="button adm-btn-danger" disabled={deleting} onClick={confirm}>
          {deleting ? <span className="progress-spinner adm-btn-spinner" aria-hidden="true" /> : <Icon name="trash" size={15} />}
          {deleting ? "Deleting…" : "Delete user"}
        </button>
      </footer>
    </dialog>
  );
}
