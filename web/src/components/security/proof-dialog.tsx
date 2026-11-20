"use client";

import { useEffect, useId, useRef, useState, type FormEvent, type ReactNode } from "react";
import * as yup from "yup";
import { AuthApiError, type SecurityProof } from "@/lib/auth-api";
import { Icon } from "../icons";

type Props = { title: string; description: string; requireMFA: boolean; totpOnly?: boolean; children?: ReactNode; confirmLabel: string; onConfirm: (proof: SecurityProof) => Promise<void>; onClose: () => void };

export function ProofDialog({ title, description, requireMFA, totpOnly = false, children, confirmLabel, onConfirm, onClose }: Props) {
  const dialog = useRef<HTMLDialogElement>(null);
  const id = useId();
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => { const element = dialog.current; if (element && !element.open) element.showModal(); }, []);

  async function submit(event: FormEvent) {
    event.preventDefault(); if (pending) return;
    const codePattern = totpOnly ? /^[0-9]{6}$/ : /^(?:[0-9]{6}|[a-fA-F0-9]{32})$/;
    const codeMessage = totpOnly ? "Enter the new authenticator’s six-digit code." : "Enter a fresh six-digit authenticator code or an unused recovery code.";
    const schema = yup.object({ password: yup.string().required("Enter your current password.").max(72), code: yup.string().defined().trim().test("verification-code", codeMessage, (value) => !requireMFA || codePattern.test(value)) });
    setPending(true); setError("");
    try { const proof = await schema.validate({ password, code }); await onConfirm(proof); }
    catch (err) {
      const rejectedProof = err instanceof AuthApiError && err.code === "mfa_code_invalid";
      setError(rejectedProof
        ? requireMFA ? "Your password or verification code is incorrect or was already used. Check your password and use a fresh code." : "Your current password is incorrect."
        : err instanceof AuthApiError || err instanceof yup.ValidationError ? err.message : "This change could not be completed. Try again.");
      setCode("");
    }
    finally { setPending(false); }
  }

  return (
    <dialog ref={dialog} className="security-dialog" aria-labelledby={`${id}-title`} onCancel={(event) => { event.preventDefault(); if (!pending) onClose(); }}>
      <form onSubmit={submit} noValidate>
        <header><span className="security-dialog-icon"><Icon name="shield" size={24} /></span><h2 id={`${id}-title`}>{title}</h2><p>{description}</p></header>
        <div className="security-dialog-body">
          {children}
          {error && <p role="alert" className="error-message">{error}</p>}
          <label htmlFor={`${id}-password`}>Current password</label>
          <input id={`${id}-password`} type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} disabled={pending} required maxLength={72} />
          {requireMFA && <>
            <label htmlFor={`${id}-code`}>{totpOnly ? "Code from the new authenticator" : "Fresh authenticator or recovery code"}</label>
            <input id={`${id}-code`} autoComplete="one-time-code" autoCapitalize="none" spellCheck={false} value={code} onChange={(event) => setCode(event.target.value)} disabled={pending} maxLength={64} required />
            <small>Wait for a fresh code if you just used one to sign in or make another change.</small>
          </>}
        </div>
        <footer><button type="button" className="button secondary" onClick={onClose} disabled={pending}><Icon name="close" size={15} />Cancel</button><button type="submit" className="button" disabled={pending}><Icon name="shield" size={15} />{pending ? "Verifying…" : confirmLabel}</button></footer>
      </form>
    </dialog>
  );
}
