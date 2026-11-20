"use client";

import Image from "next/image";
import { useEffect, useState, type FormEvent } from "react";
import * as yup from "yup";
import { AuthApiError, verifyMFA, type LoginResponse, type MFAChallenge, type MFALoginResult } from "@/lib/auth-api";
import { Icon } from "../icons";
import { FormMessages } from "./form-fields";

const codeSchema = yup.object({ code: yup.string().trim().required("Enter an authenticator or recovery code.").matches(/^(?:[0-9]{6}|[a-fA-F0-9]{32})$/, "Enter six authenticator digits or a 32-character recovery code.") });

export function MfaStep({ challenge, onComplete, onCancel }: { challenge: MFAChallenge; onComplete: (session: LoginResponse) => void; onCancel: () => void }) {
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [expired, setExpired] = useState(false);
  const [recovery, setRecovery] = useState<MFALoginResult | null>(null);
  useEffect(() => { const timer = window.setTimeout(() => setExpired(true), challenge.expires_in * 1000); return () => window.clearTimeout(timer); }, [challenge.expires_in]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    try { await codeSchema.validate({ code }); } catch (err) { setError(err instanceof yup.ValidationError ? err.message : "Check the code."); return; }
    if (expired || busy) return;
    setBusy(true); setError("");
    try {
      const result = await verifyMFA(challenge.challenge_token, code.trim());
      setCode("");
      if (result.recovery_codes.length > 0) setRecovery(result);
      else onComplete(result.session);
    } catch (err) {
      setError(err instanceof AuthApiError ? err.message : "Verification failed. Try again.");
      if (err instanceof AuthApiError && (err.code === "mfa_locked" || err.code === "mfa_challenge_invalid" || err.code === "mfa_disabled")) setExpired(true);
    } finally { setBusy(false); }
  }

  function downloadCodes(codes: string[]) {
    const url = URL.createObjectURL(new Blob(["PayCore recovery codes — each works once. Keep these private.\n\n" + codes.join("\n")], { type: "text/plain" }));
    const link = document.createElement("a"); link.href = url; link.download = "paycore-recovery-codes.txt"; link.click();
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  if (recovery) return (
    <section aria-labelledby="mfa-recovery-title" className="login-form">
      <h2 id="mfa-recovery-title">Save your recovery codes</h2>
      <p>Your authenticator is enabled. Each code works once if you lose access to your app. These codes are shown only now.</p>
      <textarea aria-label="Recovery codes" readOnly rows={8} value={recovery.recovery_codes.join("\n")} spellCheck={false} />
      <button type="button" className="button secondary" onClick={() => downloadCodes(recovery.recovery_codes)}><Icon name="download" size={16} />Download codes</button>
      <button type="button" className="button" onClick={() => onComplete(recovery.session)}><Icon name="check" size={16} />I’ve saved my codes — continue</button>
    </section>
  );

  return (
    <section aria-labelledby="mfa-title" className="login-form">
      <h2 id="mfa-title">{challenge.enrollment ? "Set up your authenticator" : "Verify your identity"}</h2>
      {challenge.enrollment ? <>
        <p>Admin accounts require MFA. Scan this QR code in your authenticator app, then enter its six-digit code.</p>
        <Image src={challenge.enrollment.qr_image} alt="Authenticator enrollment QR code" width={232} height={232} unoptimized style={{ margin: "0 auto", maxWidth: "100%", height: "auto" }} />
        <details><summary>Enter the setup key manually</summary><code style={{ overflowWrap: "anywhere" }}>{challenge.enrollment.secret}</code></details>
      </> : <p>Enter a fresh code from your authenticator app, or one unused recovery code.</p>}
      <form onSubmit={submit} noValidate className="login-form">
        <FormMessages error={error || (expired ? "This challenge is unavailable. Return to sign in to try again." : "")} />
        <label htmlFor="mfa-code">{challenge.enrollment ? "Authenticator code" : "Authenticator or recovery code"}</label>
        <input id="mfa-code" value={code} onChange={(event) => setCode(event.target.value)} autoComplete="one-time-code" autoCapitalize="none" spellCheck={false} maxLength={64} disabled={busy || expired} required />
        <button type="submit" className="button" disabled={busy || expired}><Icon name="shield" size={16} />{busy ? "Verifying…" : "Verify and sign in"}</button>
        <button type="button" className="button secondary" onClick={onCancel} disabled={busy}><Icon name="logout" size={16} />Back to sign in</button>
      </form>
    </section>
  );
}
