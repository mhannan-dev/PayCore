"use client";

import { useEffect, useId, useRef } from "react";
import { Icon } from "./icons";
import "./confirmation-modal.css";

type ConfirmationModalProps = {
  open: boolean;
  title: string;
  description: string;
  confirmLabel: string;
  pendingLabel?: string;
  pending?: boolean;
  onCancel: () => void;
  onConfirm: () => void;
};

export function ConfirmationModal({ open, title, description, confirmLabel, pendingLabel = "Please wait…", pending = false, onCancel, onConfirm }: ConfirmationModalProps) {
  const dialog = useRef<HTMLDialogElement>(null);
  const cancelButton = useRef<HTMLButtonElement>(null);
  const id = useId();

  useEffect(() => {
    const element = dialog.current;
    if (open && element && !element.open) {
      element.showModal();
      cancelButton.current?.focus();
    } else if (!open && element?.open) {
      element.close();
    }
  }, [open]);

  return (
    <dialog
      ref={dialog}
      className="confirmation-modal"
      aria-labelledby={`${id}-title`}
      aria-describedby={`${id}-description`}
      aria-busy={pending}
      onCancel={(event) => {
        event.preventDefault();
        if (!pending) onCancel();
      }}
    >
      <button type="button" className="confirmation-close" aria-label="Close confirmation" disabled={pending} onClick={onCancel}>
        <Icon name="close" size={18} />
      </button>
      <div className="confirmation-content">
        <div className="confirmation-icon"><Icon name="logout" size={24} /></div>
        <h2 id={`${id}-title`} className="confirmation-title">{title}</h2>
        <p id={`${id}-description`} className="confirmation-description">{description}</p>
      </div>
      <div className="confirmation-actions">
        <button ref={cancelButton} type="button" className="confirmation-cancel" disabled={pending} onClick={onCancel}><Icon name="close" size={16} />Cancel</button>
        <button type="button" className="confirmation-confirm" disabled={pending} onClick={onConfirm}>
          {pending ? <span className="confirmation-spinner" aria-hidden="true" /> : <Icon name="logout" size={16} />}
          {pending ? pendingLabel : confirmLabel}
        </button>
      </div>
    </dialog>
  );
}

