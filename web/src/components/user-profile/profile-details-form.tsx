"use client";

import { useState, type FormEvent } from "react";
import { AuthApiError } from "@/lib/auth-api";
import { updateProfile } from "@/lib/auth-session";
import { notify } from "@/lib/notify";
import { Icon } from "../icons";
import { Panel } from "../ui";
import { validateProfile } from "./profile-schema";

type Props = { firstName: string; lastName: string; email: string };

/** Personal information card. Remounts (via key) when the saved names change, so fields reset to them. */
export function ProfileDetailsForm(props: Props) {
  return <DetailsForm key={`${props.firstName}\u0000${props.lastName}`} {...props} />;
}

function DetailsForm({ firstName, lastName, email }: Props) {
  const [first, setFirst] = useState(firstName);
  const [last, setLast] = useState(lastName);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const dirty = first.trim() !== firstName || last.trim() !== lastName;

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const result = await validateProfile({ firstName: first, lastName: last });
    if (!result.values) return setError(result.error ?? "");
    setError("");
    setSaving(true);
    try {
      await updateProfile(result.values.firstName, result.values.lastName || null);
      notify.success("Profile updated.");
    } catch (err) {
      notify.error(err instanceof AuthApiError ? err.message : "Could not save your profile. Try again.");
    } finally {
      setSaving(false);
    }
  }

  const discard = () => {
    setFirst(firstName);
    setLast(lastName);
    setError("");
  };

  return (
    <Panel title="Personal information" description="How your name appears across PayCore.">
      <form className="pf-form" onSubmit={save} noValidate>
        <div className="pf-field-grid">
          <label className="pf-field">
            <span>First name <em aria-hidden="true">*</em></span>
            <input
              name="firstName"
              value={first}
              onChange={(e) => { setFirst(e.target.value); setError(""); }}
              maxLength={80}
              required
              autoComplete="given-name"
              aria-invalid={error !== "" && !first.trim()}
            />
          </label>
          <label className="pf-field">
            <span>Last name <small>Optional</small></span>
            <input name="lastName" value={last} onChange={(e) => { setLast(e.target.value); setError(""); }} maxLength={80} autoComplete="family-name" />
          </label>
        </div>

        <div className="pf-field">
          <span>Email address</span>
          <div className="pf-readonly">
            <Icon name="lock" size={14} />
            <span>{email}</span>
          </div>
          <small className="pf-help">Your sign-in email can only be changed by an administrator.</small>
        </div>

        {error && <p role="alert" className="error-message">{error}</p>}

        <div className="pf-form-footer">
          <span className="pf-muted" aria-live="polite">{dirty ? "You have unsaved changes." : "All changes saved."}</span>
          <div className="pf-form-actions">
            <button className="button secondary" type="button" onClick={discard} disabled={saving || !dirty}><Icon name="refresh" size={15} />Discard</button>
            <button className="button primary" type="submit" disabled={saving || !dirty} aria-busy={saving}>
              <Icon name="save" size={15} />{saving ? "Saving…" : "Save changes"}
            </button>
          </div>
        </div>
      </form>
    </Panel>
  );
}
