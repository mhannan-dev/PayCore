"use client";

import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { AuthApiError, createUser, type InvitationResult, type RoleDetail } from "@/lib/auth-api";
import { useAuthSession } from "@/lib/auth-session";
import { notify } from "@/lib/notify";
import { Icon } from "../icons";
import { roleLabel } from "../user-profile/roles";
import { validateNewUser, type NewUserErrors, type NewUserValues } from "./new-user-schema";

type Props = { roles: RoleDetail[]; onClose: () => void; onCreated: (result: InvitationResult) => void };

const EMPTY: NewUserValues = { firstName: "", lastName: "", email: "", roles: ["OPERATOR"] };

// API error codes that belong to one field.
const FIELD_FOR_CODE: Record<string, keyof NewUserValues> = { email_taken: "email" };

function Field({ id, label, error, hint, optional, children }: { id: string; label: string; error?: string; hint?: string; optional?: boolean; children: ReactNode }) {
  return (
    <div className="adm-field">
      <label htmlFor={id}>
        {label} {optional ? <small>Optional</small> : <em aria-hidden="true">*</em>}
      </label>
      {children}
      {error ? <p id={`${id}-error`} className="adm-field-error" role="alert">{error}</p> : hint && <p className="adm-field-hint">{hint}</p>}
    </div>
  );
}

/** Modal form for inviting a user; they set their own password from the emailed link. Mount it only while open. */
export function CreateUserDialog({ roles, onClose, onCreated }: Props) {
  const { session } = useAuthSession();
  const dialog = useRef<HTMLDialogElement>(null);
  const id = useId();
  const [values, setValues] = useState<NewUserValues>(EMPTY);
  const [errors, setErrors] = useState<NewUserErrors>({});
  const [submitted, setSubmitted] = useState(false);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState("");

  useEffect(() => {
    const element = dialog.current;
    if (element && !element.open) element.showModal();
  }, []);

  // Errors appear after the first submit, then update as the user types.
  function change(next: Partial<NewUserValues>) {
    const merged = { ...values, ...next };
    setValues(merged);
    setFormError("");
    if (submitted) setErrors(validateNewUser(merged));
  }

  function toggleRole(code: string) {
    change({ roles: values.roles.includes(code) ? values.roles.filter((r) => r !== code) : [...values.roles, code] });
  }

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setSubmitted(true);
    const found = validateNewUser(values);
    setErrors(found);
    if (Object.keys(found).length > 0) return;
    setSaving(true);
    try {
      const ordered = roles.map((r) => r.code).filter((code) => values.roles.includes(code));
      const result = await createUser(session.token, {
        first_name: values.firstName.trim(),
        last_name: values.lastName.trim() || null,
        email: values.email.trim(),
        roles: ordered,
      });
      if (result.email_status === "not_configured") {
        notify.warning(`${result.user.first_name} was added, but no mail server is configured, so no email was sent. Use "Resend invite" once email is set up.`);
      } else {
        notify.success(`${result.user.first_name} was added. The invitation email is being sent to ${result.user.email}; its status shows in the list.`);
      }
      onCreated(result);
    } catch (error) {
      const field = error instanceof AuthApiError ? FIELD_FOR_CODE[error.code] : undefined;
      const message = error instanceof AuthApiError ? error.message : "Could not add the user. Try again.";
      if (field) setErrors((current) => ({ ...current, [field]: message }));
      else setFormError(message);
      setSaving(false);
    }
  }

  const invalid = (field: keyof NewUserValues) => (errors[field] ? { "aria-invalid": true, "aria-describedby": `${id}-${field}-error` } : {});

  return (
    <dialog
      ref={dialog}
      className="adm-dialog adm-dialog-wide"
      aria-labelledby={`${id}-title`}
      aria-busy={saving}
      onCancel={(e) => { e.preventDefault(); if (!saving) onClose(); }}
    >
      <form noValidate onSubmit={submit} className="adm-dialog-form">
        <header className="adm-dialog-head">
          <div>
            <p className="eyebrow">NEW ACCOUNT</p>
            <h2 id={`${id}-title`}>Add user</h2>
            <p className="adm-dialog-sub">We&apos;ll email them a link to set their own password.</p>
          </div>
          <button type="button" className="adm-icon-btn" aria-label="Close" disabled={saving} onClick={onClose}>
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" aria-hidden="true"><path d="m6 6 12 12M18 6 6 18" /></svg>
          </button>
        </header>

        <fieldset className="adm-form-body" disabled={saving}>
          <div className="adm-form-grid">
            <Field id={`${id}-firstName`} label="First name" error={errors.firstName}>
              <input id={`${id}-firstName`} autoComplete="off" maxLength={80} value={values.firstName} onChange={(e) => change({ firstName: e.target.value })} {...invalid("firstName")} autoFocus />
            </Field>
            <Field id={`${id}-lastName`} label="Last name" optional error={errors.lastName}>
              <input id={`${id}-lastName`} autoComplete="off" maxLength={80} value={values.lastName} onChange={(e) => change({ lastName: e.target.value })} {...invalid("lastName")} />
            </Field>
          </div>

          <Field id={`${id}-email`} label="Email" error={errors.email} hint="Used to sign in. It must be unique.">
            <input id={`${id}-email`} type="email" autoComplete="off" maxLength={254} value={values.email} onChange={(e) => change({ email: e.target.value })} {...invalid("email")} />
          </Field>


          <div className="adm-field">
            <span className="adm-field-label" id={`${id}-roles`}>Roles <em aria-hidden="true">*</em></span>
            <div className="adm-role-pills" role="group" aria-labelledby={`${id}-roles`}>
              {roles.map((role) => {
                const checked = values.roles.includes(role.code);
                return (
                  <label key={role.code} className={`adm-role-pill${checked ? " is-checked" : ""}`} title={role.description}>
                    <input type="checkbox" checked={checked} onChange={() => toggleRole(role.code)} />
                    <span className="adm-role-check" aria-hidden="true"><Icon name="check" size={11} /></span>
                    {roleLabel(role.code)}
                  </label>
                );
              })}
            </div>
            {errors.roles && <p className="adm-field-error" role="alert">{errors.roles}</p>}
          </div>
          <div className="adm-invite-note">
            <Icon name="mail" size={18} />
            <p>
              <strong>Password is set by the user.</strong> An invitation email with a one-time link goes to this
              address. The link expires in 72 hours; you can re-send it from the user list.
            </p>
          </div>
        </fieldset>

        {formError && <p className="error-message" role="alert">{formError}</p>}

        <footer className="adm-dialog-foot">
          <button type="button" className="button secondary" disabled={saving} onClick={onClose}><Icon name="close" size={15} />Cancel</button>
          <button type="submit" className="button primary" disabled={saving}>
            {saving ? <span className="progress-spinner adm-btn-spinner" aria-hidden="true" /> : <Icon name="mail" size={15} />}
            {saving ? "Sending invite…" : "Send invite"}
          </button>
        </footer>
      </form>
    </dialog>
  );
}
