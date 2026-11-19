"use client";

import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { AuthApiError } from "@/lib/auth-api";
import { Icon } from "../icons";

type Props = {
  title: string;
  children: ReactNode;
  icon: string;
  tone: "danger" | "warning";
  confirmLabel: string;
  pendingLabel: string;
  /** Runs the action; a thrown error is shown in the dialog, success closes it. */
  onConfirm: () => Promise<void>;
  onClose: () => void;
};

/** Centered confirmation modal for admin actions. Mount it only while open. */
export function ConfirmDialog({ title, children, icon, tone, confirmLabel, pendingLabel, onConfirm, onClose }: Props) {
  const dialog = useRef<HTMLDialogElement>(null);
  const id = useId();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    const element = dialog.current;
    if (element && !element.open) element.showModal();
  }, []);

  async function confirm() {
    setPending(true);
    setError("");
    try {
      await onConfirm();
    } catch (e) {
      setError(e instanceof AuthApiError ? e.message : "Something went wrong. Try again.");
      setPending(false);
    }
  }

  return (
    <dialog
      ref={dialog}
      className={`adm-dialog adm-dialog-danger is-${tone}`}
      aria-labelledby={`${id}-title`}
      aria-describedby={`${id}-desc`}
      aria-busy={pending}
      onCancel={(e) => { e.preventDefault(); if (!pending) onClose(); }}
    >
      <div className="adm-danger-body">
        <span className="adm-danger-icon" aria-hidden="true"><Icon name={icon} size={24} /></span>
        <h2 id={`${id}-title`}>{title}</h2>
        <div id={`${id}-desc`} className="adm-confirm-text">{children}</div>
        {error && <p className="error-message" role="alert">{error}</p>}
      </div>
      <footer className="adm-dialog-foot">
        <button type="button" className="button secondary" disabled={pending} onClick={onClose} autoFocus>Cancel</button>
        <button type="button" className={`button ${tone === "danger" ? "adm-btn-danger" : "adm-btn-warning"}`} disabled={pending} onClick={() => void confirm()}>
          {pending ? <span className="progress-spinner adm-btn-spinner" aria-hidden="true" /> : <Icon name={icon} size={15} />}
          {pending ? pendingLabel : confirmLabel}
        </button>
      </footer>
    </dialog>
  );
}
