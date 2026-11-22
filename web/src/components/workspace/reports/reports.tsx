"use client";

import { useWorkspaceCurrency } from "@/lib/workspace-currency";
import Link from "next/link";
import { useCallback } from "react";
import {
  fetchAccounts,
  fetchZeroSum,
  type Currency,
  type LedgerAccount,
  type ZeroSumReport,
} from "@/lib/ledger-api";
import { formatMoney } from "@/lib/money";
import { Icon } from "../../icons";
import { Badge, Empty, Panel } from "../../ui";
import { ApiStatus } from "../shared/api-status";
import { useApi } from "../shared/use-api";
import { ReportExport } from "./report-export";

function BalanceSummary({
  accounts,
  loading,
}: {
  accounts: LedgerAccount[];
  loading: boolean;
}) {
  return (
    <Panel
      title="Live Chart of Accounts balance"
      description="Aggregated balances derived from append-only ledger postings."
    >
      <div className="summary-list">
        {loading && accounts.length === 0 && (
          <Empty>Loading account balances...</Empty>
        )}
        {!loading && accounts.length === 0 && (
          <Empty>No ledger accounts registered yet.</Empty>
        )}
        {accounts.map((a) => (
          <div key={a.id}>
            <span>
              <strong>{a.name}</strong>
              <small>
                {a.account_number} · {a.type} · {a.currency}
              </small>
            </span>
            <strong className="money">{formatMoney(a.balance, a.currency)}</strong>
          </div>
        ))}
      </div>
    </Panel>
  );
}

function ReportingCoverage({
  currency,
  accountsCount,
  audit,
  loading,
}: {
  currency: Currency;
  accountsCount: number;
  audit: ZeroSumReport | null;
  loading: boolean;
}) {
  const auditAvailable = audit !== null;
  const isBalanced = audit?.balanced === true;
  const totalDebits = audit?.total_debits ?? "0";
  const totalCredits = audit?.total_credits ?? "0";
  const difference = audit?.difference ?? "0";
  const checkedAt = audit?.checked_at
    ? new Date(audit.checked_at).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" })
    : "—";

  const facts: [string, string][] = [
    ["Accounts in Ledger", loading ? "..." : String(accountsCount)],
    ["Total Journals Audited", loading ? "..." : String(audit?.journals ?? 0)],
    ["Total Postings Audited", loading ? "..." : String(audit?.postings ?? 0)],
    ["Audited Debit Volume", loading ? "..." : formatMoney(totalDebits, currency)],
    ["Audited Credit Volume", loading ? "..." : formatMoney(totalCredits, currency)],
    ["Net Invariant Difference", loading ? "..." : formatMoney(difference, currency)],
    ["Last Verified At", checkedAt],
  ];

  return (
    <Panel
      title="Trial Balance & Audit Status"
      description="Real-time invariant verification across all ledger accounts."
      action={
        <Badge tone={!auditAvailable ? "neutral" : isBalanced ? "green" : "amber"}>
          {!auditAvailable ? "Audit unavailable" : isBalanced ? "Balanced (Zero-Sum)" : "Discrepancy"}
        </Badge>
      }
    >
      <div className="report-copy">
        <Badge tone="green">Connected to Ledger Core</Badge>
        <h3>Audit-Ready Ledger Invariants</h3>
        <p>
          Balances are calculated on-the-fly from immutable double-entry journal entries. Every
          credit strictly mirrors a debit.
        </p>
        <dl>
          {facts.map(([term, value]) => (
            <div key={term}>
              <dt>{term}</dt>
              <dd>
                <strong>{value}</strong>
              </dd>
            </div>
          ))}
        </dl>
        <Link href="/audit" className="text-link">
          Run deep invariant audit <Icon name="arrow" size={16} />
        </Link>
      </div>
    </Panel>
  );
}

type ReportsData = {
  accounts: LedgerAccount[];
  audit: ZeroSumReport | null;
};

export function Reports() {
  const { currency } = useWorkspaceCurrency();
  const loadReports = useCallback(async (token: string): Promise<ReportsData> => {
    const [accountsPage, auditReport] = await Promise.all([
      fetchAccounts(token, { currency, limit: 100, offset: 0 }),
      fetchZeroSum(token, currency).catch((): ZeroSumReport | null => null),
    ]);
    return {
      accounts: accountsPage.items,
      audit: auditReport,
    };
  }, [currency]);

  const { data, loading, error, forbidden, reload } = useApi<ReportsData>(`reports-data-${currency}`, loadReports);

  const accounts = data?.accounts ?? [];
  const audit = data?.audit ?? null;

  return (
    <>
      <ReportExport />
      <ApiStatus
        loading={loading}
        error={error}
        forbidden={forbidden}
        onRetry={reload}
      />
      <div className="overview-grid">
        <BalanceSummary accounts={accounts} loading={loading} />
        <ReportingCoverage currency={currency} accountsCount={accounts.length} audit={audit} loading={loading} />
      </div>
    </>
  );
}
