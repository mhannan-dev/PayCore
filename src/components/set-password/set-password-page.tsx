"use client";

import Link from "next/link";
import { useEffect, useState, useSyncExternalStore, type FormEvent } from "react";
import { AuthApiError, acceptInvitation, confirmPasswordReset, inspectInvitation, inspectPasswordReset } from "@/lib/auth-api";
import { notify } from "@/lib/notify";
import { Icon } from "../icons";
import { FormMessages, PasswordField } from "../login/form-fields";
import { LoginFooter } from "../login/login-header";
import { PASSWORD_LENGTH, passwordLength, validateNewPassword } from "./set-password-schema";
import "./set-password.css";

// The token is in the URL fragment (#token=…): browsers never send fragments to
// servers or in Referer headers, so it stays out of logs.
const subscribeHash = (onChange: () => void) => {
  window.addEventListener("hashchange", onChange);
  return () => window.removeEventListener("hashchange", onChange);
};
const readToken = () => new URLSearchParams(window.location.hash.slice(1)).get("token") ?? "";
const isWellFormed = (token: string) => /^[0-9a-f]{64}$/i.test(token);

type LinkInfo = { email: string; first_name: string; expires_at: string };
type Check = { token: string } & ({ status: "ready"; info: LinkInfo } | { status: "invalid"; message: string });

/** The two emailed-link flows that end in choosing a password. */
export type PasswordFlow = "invite" | "reset";

type FlowConfig = {
  inspect: (token: string) => Promise<LinkInfo>;
  confirm: (token: string, password: string) => Promise<{ email: string }>;
  invalidCode: string;
  invalid: string;
  invalidAction: { href: string; label: string };
  eyebrow: string;
  title: (info: LinkInfo) => string;
  checking: string;
  submit: string;
  doneTitle: string;
  doneText: string;
  toast: string;
};

const FLOWS: Record<PasswordFlow, FlowConfig> = {
  invite: {
    inspect: inspectInvitation,
    confirm: acceptInvitation,
    invalidCode: "invitation_invalid",
    invalid: "This link is invalid, has expired or was already used. Ask your administrator to send a new invitation.",
    invalidAction: { href: "/login", label: "Back to sign in" },
    eyebrow: "ACCOUNT INVITATION",
    title: (info) => `Welcome, ${info.first_name}`,
    checking: "Checking your invitation…",
    submit: "Set password",
    doneTitle: "You're all set",
    doneText: "is saved. Sign in to open your workspace.",
    toast: "Password set. You can sign in now.",
  },
  reset: {
    inspect: inspectPasswordReset,
    confirm: confirmPasswordReset,
    invalidCode: "reset_invalid",
    invalid: "This reset link is invalid, has expired or was already used. Request a new one.",
    invalidAction: { href: "/forgot-password", label: "Request a new link" },
    eyebrow: "PASSWORD RESET",
    title: (info) => `New password for ${info.first_name}`,
    checking: "Checking your reset link…",
    submit: "Save new password",
    doneTitle: "Password changed",
    doneText: "is updated, and you were signed out on every device. Sign in with the new password.",
    toast: "Password changed. Sign in with your new password.",
  },
};

function Shell({ children }: { children: React.ReactNode }) {
  return (
    <main className="login-page">
      <div className="login-wrapper">
        <section className="login-card sp-card" aria-label="Choose a password">
          <div className="login-header">
            <div className="login-brand">
              <span className="brand-mark">p<span>·</span></span>
              PayCore
              <span className="brand-tag">FINANCIAL OS</span>
            </div>
          </div>
          {children}
        </section>
        <LoginFooter />
      </div>
    </main>
  );
}

function Requirement({ met, children }: { met: boolean; children: React.ReactNode }) {
  return (
    <li className={met ? "is-met" : ""}>
      <span className="sp-req-dot" aria-hidden="true">{met && <Icon name="check" size={10} />}</span>
      {children}
      <span className="sr-only">{met ? " (done)" : " (not yet)"}</span>
    </li>
  );
}

export function SetPasswordPage({ flow = "invite" }: { flow?: PasswordFlow }) {
  const config = FLOWS[flow];
  const token = useSyncExternalStore(subscribeHash, readToken, () => "");
  const [check, setCheck] = useState<Check | null>(null);
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const [doneEmail, setDoneEmail] = useState("");

  useEffect(() => {
    if (!isWellFormed(token)) return;
    let cancelled = false;
    config.inspect(token).then(
      (info) => { if (!cancelled) setCheck({ token, status: "ready", info }); },
      (e: unknown) => {
        if (!cancelled) setCheck({ token, status: "invalid", message: e instanceof AuthApiError && e.code !== config.invalidCode ? e.message : config.invalid });
      },
    );
    return () => { cancelled = true; };
  }, [token, config]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    const problem = validateNewPassword(password, confirm);
    setError(problem);
    if (problem) return;
    setSaving(true);
    try {
      const { email } = await config.confirm(token, password);
      // The link is spent; drop it from the address bar and history.
      window.history.replaceState(null, "", window.location.pathname);
      setPassword("");
      setConfirm("");
      setDoneEmail(email);
      notify.success(config.toast);
    } catch (e) {
      if (e instanceof AuthApiError && e.code === config.invalidCode) setCheck({ token, status: "invalid", message: config.invalid });
      else setError(e instanceof AuthApiError ? e.message : "Could not set the password. Try again.");
    } finally {
      setSaving(false);
    }
  }

  if (doneEmail) {
    return (
      <Shell>
        <div className="sp-result">
          <span className="sp-result-icon is-ok"><Icon name="checkCircle" size={30} /></span>
          <h1>{config.doneTitle}</h1>
          <p>Your password for <strong>{doneEmail}</strong> {config.doneText}</p>
          <Link href="/login" className="button primary login-submit-btn">Go to sign in <Icon name="arrow" size={17} /></Link>
        </div>
      </Shell>
    );
  }

  // Missing/malformed token, or the API rejected it.
  const current = check?.token === token ? check : null;
  if (!isWellFormed(token) || current?.status === "invalid") {
    return (
      <Shell>
        <div className="sp-result">
          <span className="sp-result-icon is-bad"><Icon name="lock" size={28} /></span>
          <h1>This link can&apos;t be used</h1>
          <p>{current?.status === "invalid" ? current.message : config.invalid}</p>
          <Link href={config.invalidAction.href} className="button secondary">{config.invalidAction.label}</Link>
        </div>
      </Shell>
    );
  }

  if (!current) {
    return (
      <Shell>
        <div className="sp-result" role="status" aria-live="polite">
          <span className="progress-spinner" aria-hidden="true" />
          <p>{config.checking}</p>
        </div>
      </Shell>
    );
  }

  const { info } = current;
  const length = passwordLength(password);
  const expires = new Date(info.expires_at);
  return (
    <Shell>
      <div className="login-title-group">
        <p className="eyebrow">{config.eyebrow}</p>
        <h1>{config.title(info)}</h1>
        <p className="login-subtitle">
          Choose a password for <strong>{info.email}</strong>.
          {!Number.isNaN(expires.getTime()) && <> This link expires {expires.toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" })}.</>}
        </p>
      </div>

      <form className="login-form" onSubmit={submit} noValidate>
        <FormMessages error={error} />
        {/* Lets password managers save the new credential under the right account. */}
        <input type="email" name="username" autoComplete="username" value={info.email} readOnly hidden />
        <PasswordField
          id="sp-password"
          label="New password"
          value={password}
          onChange={(e) => { setPassword(e.target.value); setError(""); }}
          placeholder={`Exactly ${PASSWORD_LENGTH} characters`}
          required
          autoComplete="new-password"
          autoFocus
        />
        <PasswordField
          id="sp-confirm"
          label="Confirm password"
          value={confirm}
          onChange={(e) => { setConfirm(e.target.value); setError(""); }}
          placeholder="Repeat the password"
          required
          autoComplete="new-password"
        />
        <ul className="sp-requirements" aria-label="Password requirements">
          <Requirement met={length === PASSWORD_LENGTH}>Exactly {PASSWORD_LENGTH} characters ({length}/{PASSWORD_LENGTH})</Requirement>
          <Requirement met={confirm.length > 0 && confirm === password}>Both passwords match</Requirement>
        </ul>
        <button type="submit" className="button primary login-submit-btn" disabled={saving}>
          {saving ? <><span className="spinner" /> Saving password…</> : <>{config.submit} <Icon name="arrow" size={17} /></>}
        </button>
      </form>
    </Shell>
  );
}
