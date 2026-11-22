"use client";

import { useWorkspaceCurrency } from "@/lib/workspace-currency";
import { useEffect, useState } from "react";
import { CURRENCIES, fetchZeroSum, type Currency, type ZeroSumReport } from "@/lib/ledger-api";
import { formatMoney } from "@/lib/money";
import { Icon } from "../../icons";
import { Badge, Heading, Notice, Panel, StatGrid } from "../../ui";
import { ApiStatus } from "../shared/api-status";
import { useApi } from "../shared/use-api";
import { ZeroSumStatus } from "./zero-sum-status";
import { zeroSumState, type ZeroSumState } from "./zero-sum-state";

function noticeText(state: ZeroSumState, report: ZeroSumReport | null, checkedAt: string, currency: Currency): string {
  if (state === "balanced" && report) return `Balanced (Δ ${formatMoney(report.difference, report.currency)}): ${report.journals} ${currency} journals checked at ${checkedAt}; every journal has at least two postings and equal debits and credits.`;
  if (state === "stale") return `The last successful check at ${checkedAt} was balanced, but it is older than 60 seconds. Wait for the next refresh or run the audit again.`;
  if (state === "unavailable") return report ? `The newest audit could not be completed; showing the last successful check from ${checkedAt}.` : "No consistent ledger snapshot is available yet.";
  if (state === "unbalanced" && report) return `Not balanced: ${report.unbalanced_journals.length} journal(s) need investigation (checked at ${checkedAt}).`;
  if (state === "forbidden") return "The zero-sum audit needs the audit:verify permission.";
  return "Running a fresh, snapshot-consistent audit…";
}

/** Live zero-sum audit: total debits must equal total credits for every currency. */
export function AuditPage() {
  const { currency, setCurrency } = useWorkspaceCurrency();
  const [run, setRun] = useState(0);
  const [now, setNow] = useState(() => Date.now());
  const audit = useApi(JSON.stringify([currency, run]), (token) => fetchZeroSum(token, currency), 10000);
  const r = audit.data;
  const state = zeroSumState(r, { loading: audit.loading, error: audit.error, forbidden: audit.forbidden, now });
  const checkedAt = r ? new Date(r.checked_at).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "medium" }) : "";

  useEffect(() => {
    const interval = window.setInterval(() => setNow(Date.now()), 5000);
    return () => window.clearInterval(interval);
  }, []);

  return (
    <>
      <Heading
        eyebrow="FINANCIAL INTEGRITY"
        title="Invariant audit"
        description="Checks the live ledger on one database snapshot: every journal balanced, and total debits equal total credits."
        action={
          <div className="adm-heading-actions">
            <label>
              <span className="sr-only">Currency</span>
              <select value={currency} onChange={(e) => setCurrency(CURRENCIES.find((c) => c === e.target.value) ?? "JPY")}>
                {CURRENCIES.map((c) => <option key={c} value={c}>{c}</option>)}
              </select>
            </label>
            <button className="button primary" onClick={() => setRun((n) => n + 1)} disabled={audit.loading}>
              <Icon name="shield" size={17} /> {audit.loading ? "Auditing…" : "Run audit"}
            </button>
          </div>
        }
      />
      <ApiStatus loading={audit.loading && !r} error={audit.error} forbidden={audit.forbidden} onRetry={audit.reload} />
      <ZeroSumStatus report={r} state={state} onRetry={audit.reload} />
      {r && (
        <>
          <StatGrid
            three
            stats={[
              { label: "Total debits", value: formatMoney(r.total_debits, r.currency), foot: `${r.journals} journals · ${r.postings} postings`, icon: "book" },
              { label: "Total credits", value: formatMoney(r.total_credits, r.currency), foot: `All ${r.currency} journals`, icon: "book" },
              { label: "Difference", value: formatMoney(r.difference, r.currency), foot: "Debits minus credits; must be zero", icon: "shield" },
            ]}
          />
          <Notice icon="shield" className="audit-notice" role="status">
            {noticeText(state, r, checkedAt, currency)}
          </Notice>
          <Panel title="Findings" description="Balanced totals do not verify authorization, solvency or external settlement.">
            {r.unbalanced_journals.length === 0 ? (
              <p className="footnote panel-pad"><Badge>Pass</Badge> No unbalanced journals. The database also rejects unbalanced journals at commit, so this should always pass.</p>
            ) : (
              <ul className="audit-findings">
                {r.unbalanced_journals.map((id) => <li key={id}><Badge tone="amber">Review</Badge> <span className="mono">{id}</span></li>)}
              </ul>
            )}
          </Panel>
        </>
      )}
    </>
  );
}
