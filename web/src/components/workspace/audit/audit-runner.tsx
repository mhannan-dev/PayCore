"use client";

import { useState } from "react";
import { CURRENCIES, fetchZeroSum, type Currency } from "@/lib/ledger-api";
import { formatMoney } from "@/lib/money";
import { Icon } from "../../icons";
import { Badge, Heading, Notice, Panel, StatGrid } from "../../ui";
import { ApiStatus } from "../shared/api-status";
import { useApi } from "../shared/use-api";

/** Live zero-sum audit: total debits must equal total credits for every currency. */
export function AuditPage() {
  const [currency, setCurrency] = useState<Currency>("JPY");
  const [run, setRun] = useState(0);
  const audit = useApi(JSON.stringify([currency, run]), (token) => fetchZeroSum(token, currency));
  const r = audit.data;
  const checkedAt = r ? new Date(r.checked_at).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "medium" }) : "";

  return (
    <>
      <Heading
        eyebrow="FINANCIAL INTEGRITY"
        title="Invariant audit"
        description="Checks the live ledger: every journal balanced, and total debits equal total credits."
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
            {r.balanced
              ? `Balanced: ${r.journals} ${r.currency} journals checked at ${checkedAt}; every journal has at least two postings and equal debits and credits.`
              : `Not balanced: ${r.unbalanced_journals.length} journal(s) need investigation (checked at ${checkedAt}).`}
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
