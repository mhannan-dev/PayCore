"use client";

import { useEffect, useState } from "react";
import { AUDIT_ACTIONS, type AuditAction, type AuditEvent } from "@/lib/auth-api";
import { Icon } from "../icons";
import { Badge, EmptyHint, Heading, Panel, type BadgeTone } from "../ui";
import { Pagination } from "../ui/pagination";
import { roleLabel } from "../user-profile/roles";
import { useAuditPage } from "./use-audit-page";
import "./admin.css";

const PAGE_SIZES = [25, 50, 100];

const ACTION_INFO: Record<AuditAction, { label: string; tone: BadgeTone }> = {
  login_succeeded: { label: "Signed in", tone: "green" },
  login_failed: { label: "Sign-in failed", tone: "amber" },
  logout: { label: "Signed out", tone: "neutral" },
  user_invited: { label: "User invited", tone: "green" },
  invitation_resent: { label: "Invitation re-sent", tone: "neutral" },
  invitation_accepted: { label: "Invitation accepted", tone: "green" },
  roles_changed: { label: "Roles changed", tone: "amber" },
  user_deactivated: { label: "User deactivated", tone: "amber" },
  user_activated: { label: "User activated", tone: "green" },
  user_deleted: { label: "User deleted", tone: "amber" },
  password_reset_requested: { label: "Reset requested", tone: "neutral" },
  password_reset_completed: { label: "Password reset", tone: "green" },
  mfa_enrolled: { label: "Authenticator enrolled", tone: "green" },
  mfa_verified: { label: "MFA verified", tone: "green" },
  mfa_failed: { label: "MFA failed", tone: "amber" },
  mfa_recovery_used: { label: "Recovery code used", tone: "amber" },
  mfa_recovery_regenerated: { label: "Recovery codes regenerated", tone: "amber" },
  mfa_replacement_started: { label: "Authenticator replacement started", tone: "amber" },
  mfa_replaced: { label: "Authenticator replaced", tone: "amber" },
  statement_matching_configured: { label: "Matching configured", tone: "neutral" },
  statement_match_confirmed: { label: "Statement matched", tone: "neutral" },
  statement_match_rejected: { label: "Match rejected", tone: "neutral" },
  statement_match_reset: { label: "Match reset", tone: "neutral" },
  statement_exception_noted: { label: "Exception noted", tone: "neutral" },
  statement_imported: { label: "Statement imported", tone: "neutral" },
  sessions_revoked: { label: "Sessions revoked", tone: "neutral" },
};

const FAILURE_REASONS: Record<string, string> = {
  unknown_email: "no account with this email",
  wrong_password: "wrong password",
  password_not_set: "invitation not yet accepted",
  account_deactivated: "account is deactivated",
};

const roles = (list: string[] | undefined) => (list && list.length > 0 ? list.map(roleLabel).join(", ") : "none");

/** One-line summary of the event's details. */
function summary(e: AuditEvent): string {
  switch (e.action) {
    case "roles_changed":
      return `${roles(e.details.roles_before)} → ${roles(e.details.roles_after)}`;
    case "user_invited":
      return `Roles: ${roles(e.details.roles_after)}`;
    case "user_deleted":
      return `Had roles: ${roles(e.details.roles_before)}`;
    case "login_failed":
      return e.details.reason ? FAILURE_REASONS[e.details.reason] ?? e.details.reason : "";
    case "mfa_recovery_regenerated": return "Previous recovery codes invalidated";
    case "mfa_replacement_started": return "Current authenticator stays active until confirmation";
    case "mfa_replaced": return `${e.details.count ?? 0} other sessions revoked; old recovery codes invalidated`;
    case "statement_matching_configured":
    case "statement_match_confirmed":
    case "statement_match_rejected":
    case "statement_match_reset":
    case "statement_exception_noted": return e.details.reason ?? "";
    case "statement_imported": return `${e.details.count ?? 0} statement rows imported`;
    case "sessions_revoked": return `${e.details.count ?? 0} sessions revoked`;
    case "mfa_failed": return e.details.reason === "security_verification_failed" ? "Account security verification failed" : "Verification rejected";
    default:
      return "";
  }
}

function when(iso: string) {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? { date: "—", time: "" } : {
    date: d.toLocaleDateString(undefined, { dateStyle: "medium" }),
    time: d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit", second: "2-digit" }),
  };
}

function AuditRow({ event }: { event: AuditEvent }) {
  const info = ACTION_INFO[event.action];
  const at = when(event.occurred_at);
  const self = event.actor_email !== null && event.actor_email === event.target_email;
  return (
    <tr>
      <td><span className="cell-primary">{at.date}</span><span className="cell-secondary mono">{at.time}</span></td>
      <td><Badge tone={info.tone}>{info.label}</Badge></td>
      <td>{event.actor_email ? <span className="cell-primary">{event.actor_email}</span> : <span className="muted">System / anonymous</span>}</td>
      <td>{event.target_email ? (self ? <span className="muted">Themselves</span> : <span className="cell-primary">{event.target_email}</span>) : <span className="muted">—</span>}</td>
      <td className="adm-audit-summary">{summary(event) || <span className="muted">—</span>}</td>
      <td title={event.user_agent ?? undefined}><span className="mono adm-audit-ip">{event.ip ?? "—"}</span></td>
    </tr>
  );
}

/** Admin audit log: append-only record of sign-ins and account administration. */
export function AuditLog() {
  const [query, setQuery] = useState("");
  const [search, setSearch] = useState("");
  const [action, setAction] = useState<AuditAction | "">("");
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(50);
  useEffect(() => {
    const timer = window.setTimeout(() => setSearch(query.trim()), 300);
    return () => window.clearTimeout(timer);
  }, [query]);
  const log = useAuditPage({ action, q: search, page, pageSize });
  const pageCount = Math.max(1, Math.ceil(log.total / pageSize));

  return (
    <div className="adm-page">
      <Heading
        eyebrow="ADMINISTRATION"
        title="Audit log"
        description="Every sign-in and account change, newest first. Entries are permanent: the database refuses to edit or delete them."
        action={<button type="button" className="button secondary" onClick={log.reload} disabled={log.loading}><Icon name="refresh" size={15} />Refresh</button>}
      />
      <Panel
        title="Events"
        description={log.loaded ? `${log.total} ${search || action ? "matching " : ""}${log.total === 1 ? "event" : "events"}` : "Loading…"}
        action={
          <div className="adm-filters">
            <label className="adm-search">
              <Icon name="search" size={15} />
              <span className="sr-only">Search by email</span>
              <input type="search" placeholder="Search actor or target email" maxLength={100} value={query} onChange={(e) => { setQuery(e.target.value); setPage(0); }} />
            </label>
            <label>
              <span className="sr-only">Filter by action</span>
              <select value={action} onChange={(e) => { setAction(AUDIT_ACTIONS.find((a) => a === e.target.value) ?? ""); setPage(0); }}>
                <option value="">All actions</option>
                {AUDIT_ACTIONS.map((a) => <option key={a} value={a}>{ACTION_INFO[a].label}</option>)}
              </select>
            </label>
          </div>
        }
      >
        {log.error && (
          <div className="adm-inline-error" role="alert">
            <span>{log.error}</span>
            <button type="button" className="button secondary adm-edit-btn" onClick={log.reload}><Icon name="refresh" size={14} />Try again</button>
          </div>
        )}
        <div className={`adm-table-wrap${log.loading && log.loaded ? " is-loading" : ""}`} aria-busy={log.loading}>
          {!log.loaded && !log.error ? (
            <div className="adm-status" role="status"><span className="progress-spinner" aria-hidden="true" />Loading events…</div>
          ) : log.events.length === 0 ? (
            <EmptyHint icon="file" title="No events" text={search || action ? "Nothing matches these filters." : "Sign-ins and account changes will appear here."} />
          ) : (
            <div className="table-scroll">
              <table className="adm-table">
                <thead><tr><th>WHEN</th><th>ACTION</th><th>BY</th><th>ACCOUNT</th><th>DETAILS</th><th>IP</th></tr></thead>
                <tbody>{log.events.map((e) => <AuditRow key={e.id} event={e} />)}</tbody>
              </table>
            </div>
          )}
        </div>
        {log.loaded && log.total > 0 && (
          <Pagination
            label="Audit log pages"
            page={Math.min(page, pageCount - 1)}
            pageSize={pageSize}
            total={log.total}
            onPageChange={setPage}
            pageSizes={PAGE_SIZES}
            onPageSizeChange={(size) => { setPageSize(size); setPage(0); }}
            disabled={log.loading}
          />
        )}
      </Panel>
    </div>
  );
}
