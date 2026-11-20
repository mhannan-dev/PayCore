"use client";

import Image from "next/image";
import { useEffect, useState } from "react";
import {
  AuthApiError, beginAuthenticatorReplacement, confirmAuthenticatorReplacement, fetchAccountSecurity,
  regenerateRecoveryCodes, revokeOtherSessions, revokeSecuritySession,
  type ReplacementSetup, type SecurityOverview, type SecurityProof,
} from "@/lib/auth-api";
import { useAuthSession, validateSession } from "@/lib/auth-session";
import { notify } from "@/lib/notify";
import { Badge, Heading, Panel } from "../ui";
import { Icon } from "../icons";
import { ProofDialog } from "./proof-dialog";
import "./security.css";

type Action = { kind: "codes" | "replace" | "others" } | { kind: "session"; id: string } | { kind: "confirm"; setup: ReplacementSetup };
const formatDate = (value: string) => new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));

export function AccountSecurity() {
  const { session } = useAuthSession();
  return <AccountSecurityContent key={session.token} token={session.token} />;
}

function AccountSecurityContent({ token }: { token: string }) {
  const [data, setData] = useState<SecurityOverview | null>(null);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);
  const [action, setAction] = useState<Action | null>(null);
  const [codes, setCodes] = useState<string[] | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    if (token) void fetchAccountSecurity(token, controller.signal).then((overview) => { if (!controller.signal.aborted) { setData(overview); setError(""); } }).catch((err: unknown) => {
      if (controller.signal.aborted) return;
      setError(err instanceof AuthApiError ? err.message : "Could not load account security.");
      if (err instanceof AuthApiError && err.code === "invalid_credentials") void validateSession();
    });
    return () => controller.abort();
  }, [token, revision]);

  async function confirm(proof: SecurityProof) {
    if (!action) return;
    try {
      switch (action.kind) {
        case "codes": { const result = await regenerateRecoveryCodes(token, proof); setCodes(result.recovery_codes); notify.success("Recovery codes regenerated. Previous codes no longer work."); break; }
        case "replace": { const setup = await beginAuthenticatorReplacement(token, proof); setAction({ kind: "confirm", setup }); return; }
        case "confirm": { const result = await confirmAuthenticatorReplacement(token, proof); setCodes(result.recovery_codes); notify.success(`Authenticator replaced. ${result.revoked_count} other sessions revoked.`); break; }
        case "others": { const result = await revokeOtherSessions(token, proof); notify.success(`${result.revoked_count} other sessions revoked.`); break; }
        case "session": await revokeSecuritySession(token, action.id, proof); notify.success("Session revoked."); break;
      }
      setAction(null); setRevision((value) => value + 1);
    } catch (err) { if (err instanceof AuthApiError && err.code === "invalid_credentials") void validateSession(); throw err; }
  }

  function downloadCodes() {
    if (!codes) return;
    const url = URL.createObjectURL(new Blob(["PayCore recovery codes — each works once. Keep these private.\n\n" + codes.join("\n")], { type: "text/plain" }));
    const link = document.createElement("a"); link.href = url; link.download = "paycore-recovery-codes.txt"; link.click(); window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  const disabled = codes !== null || action !== null;
  return (
    <div className="security-page">
      <Heading eyebrow="YOUR ACCOUNT" title="Account security" description="Manage your authenticator, recovery codes and active sessions." action={<button type="button" className="button secondary" onClick={() => setRevision((value) => value + 1)}><Icon name="refresh" size={16} />Refresh</button>} />
      {error && <p role="alert" className="error-message">{error}</p>}
      {!data && !error && <p role="status">Loading account security…</p>}
      {codes && <Panel title="Save your new recovery codes" description="These replace every previous code and are shown only now. Each works once.">
        <textarea className="security-codes" aria-label="New recovery codes" value={codes.join("\n")} readOnly rows={8} spellCheck={false} />
        <div className="security-actions"><button type="button" className="button secondary" onClick={downloadCodes}><Icon name="download" size={16} />Download codes</button><button type="button" className="button" onClick={() => setCodes(null)}><Icon name="check" size={16} />I’ve saved my codes</button></div>
      </Panel>}
      {data && <>
        <div className="security-grid">
          <Panel title="Authenticator" description={data.mfa_enforced ? "Admin accounts require MFA." : "MFA is temporarily disabled by system configuration."} action={<Badge tone={data.mfa_enforced && data.mfa_enabled ? "green" : "amber"}>{!data.mfa_enforced ? "Paused" : data.mfa_enabled ? "Enabled" : "Not enrolled"}</Badge>}>
            <p>{data.enabled_at ? `Enrolled ${formatDate(data.enabled_at)}.` : data.mfa_enforced ? "Admin enrollment happens during sign in." : "Sign in uses your password and CAPTCHA while MFA is paused."}</p>
            <p>{data.mfa_enforced ? "Your current authenticator keeps working until you confirm the replacement. Confirmation revokes other sessions and replaces recovery codes." : "Your saved authenticator and recovery codes are preserved. MFA settings will be available when enforcement is restored."}</p>
            <button type="button" className="button secondary" disabled={disabled || !data.mfa_enabled || !data.mfa_enforced} onClick={() => setAction({ kind: "replace" })}><Icon name="shield" size={16} />Replace authenticator</button>
          </Panel>
          <Panel title="Recovery codes" description="A private backup when you cannot access your authenticator.">
            <p className="security-count">{data.recovery_codes_remaining}<span>unused codes</span></p>
            <p>Regenerating immediately invalidates all previous codes. Store the new set somewhere private.</p>
            <button type="button" className="button secondary" disabled={disabled || !data.mfa_enabled || !data.mfa_enforced} onClick={() => setAction({ kind: "codes" })}><Icon name="key" size={16} />Regenerate recovery codes</button>
          </Panel>
        </div>
        <Panel title="Active sessions" description="Only your active sessions appear here. The current session is preserved when revoking others." action={<button type="button" className="button secondary" disabled={disabled || !data.sessions.some((item) => !item.current)} onClick={() => setAction({ kind: "others" })}><Icon name="logout" size={16} />Revoke other sessions</button>}>
          <ul className="security-sessions">{data.sessions.map((item) => <li key={item.id}>
            <span className="security-session-icon"><Icon name="shield" size={20} /></span>
            <div><strong>{item.current ? "This session" : "Another signed-in session"}</strong><span>Started {formatDate(item.created_at)}</span><span>Expires {formatDate(item.expires_at)} · {item.mfa_verified ? "MFA verified" : "Password session"}</span></div>
            {item.current ? <Badge>Current</Badge> : <button type="button" className="button secondary" disabled={disabled} onClick={() => setAction({ kind: "session", id: item.id })}><Icon name="logout" size={15} />Revoke</button>}
          </li>)}</ul>
          {data.more_sessions && <p>Showing the current session and the newest other sessions, up to 100 total. “Revoke other sessions” also covers sessions not shown.</p>}
        </Panel>
      </>}
      {action && data && <ProofDialog key={action.kind} title={action.kind === "codes" ? "Regenerate recovery codes?" : action.kind === "replace" ? "Verify your current authenticator" : action.kind === "confirm" ? "Confirm the new authenticator" : "Revoke signed-in sessions?"}
        description={action.kind === "confirm" ? "Scan the new QR code, then enter its code. This setup expires after five minutes. Your old authenticator stays active until confirmation." : "Confirm your current password and, when enrolled, a fresh authenticator or unused recovery code."}
        requireMFA={data.mfa_enforced && data.mfa_enabled} totpOnly={action.kind === "confirm"} confirmLabel={action.kind === "confirm" ? "Confirm replacement" : "Verify and continue"} onConfirm={confirm} onClose={() => setAction(null)}>
        {action.kind === "confirm" && <div className="security-enrollment"><Image src={action.setup.enrollment.qr_image} alt="New authenticator setup QR code" width={232} height={232} unoptimized /><details><summary>Manual setup key</summary><code>{action.setup.enrollment.secret}</code></details></div>}
      </ProofDialog>}
    </div>
  );
}
