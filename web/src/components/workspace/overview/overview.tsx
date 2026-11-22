"use client";

import { useWorkspaceCurrency } from "@/lib/workspace-currency";
import Link from "next/link";
import { useCallback } from "react";
import {
  fetchAccounts,
  fetchJournals,
  fetchZeroSum,
  type LedgerAccount,
  type Journal,
  type ZeroSumReport,
} from "@/lib/ledger-api";
import { formatMoney } from "@/lib/money";
import { Icon } from "../../icons";
import { Badge, Heading, Panel, StatGrid } from "../../ui";
import { ApiStatus } from "../shared/api-status";
import { JournalTable } from "../shared/journal-table";
import { useApi } from "../shared/use-api";
import { ActivityChart } from "./activity-chart";
import { BalanceCard } from "./balance-card";
import { QuickLinks } from "./quick-links";

type OverviewData = {
  accounts: LedgerAccount[];
  accountsTotal: number;
  journals: Journal[];
  journalsTotal: number;
  audit: ZeroSumReport | null;
};

export function Overview() {
  const { currency } = useWorkspaceCurrency();
  const loadOverview = useCallback(async (token: string): Promise<OverviewData> => {
    const [accountsPage, journalsPage, auditReport] = await Promise.all([
      fetchAccounts(token, { currency, limit: 100, offset: 0 }),
      fetchJournals(token, { currency, limit: 10, offset: 0 }),
      fetchZeroSum(token, currency).catch((): ZeroSumReport | null => null),
    ]);

    return {
      accounts: accountsPage.items,
      accountsTotal: accountsPage.total,
      journals: journalsPage.items,
      journalsTotal: journalsPage.total,
      audit: auditReport,
    };
  }, [currency]);

  const { data, loading, error, forbidden, reload } = useApi<OverviewData>(`overview-dashboard-${currency}`, loadOverview);

  const audit = data?.audit ?? null;
  const totalDebits = audit?.total_debits ?? "0";
  const totalCredits = audit?.total_credits ?? "0";
  const difference = audit?.difference ?? "0";
  const auditAvailable = audit !== null;
  const isBalanced = audit?.balanced === true;
  const accountsCount = data?.accountsTotal ?? 0;
  const journalsCount = data?.journalsTotal ?? 0;
  const recentJournals = data?.journals ?? [];

  return (
    <>
      <Heading
        eyebrow="YOUR FINANCIAL WORKSPACE"
        title="A clear view of every movement."
        description="Monitor your live ledger, track postings in real-time, and verify mathematical balance."
        action={
          <Link className="button primary" href="/transfers">
            <Icon name="plus" size={17} /> New transfer
          </Link>
        }
      />

      <ApiStatus
        loading={loading}
        error={error}
        forbidden={forbidden}
        onRetry={reload}
      />

      <StatGrid
        stats={[
          {
            label: "Total Debit Volume",
            value: loading ? "..." : formatMoney(totalDebits, currency),
            foot: `Audited across ${journalsCount} journal entries`,
            icon: "arrows",
          },
          {
            label: "Chart of Accounts",
            value: loading ? "..." : String(accountsCount).padStart(2, "0"),
            foot: "Assets, liabilities, equity, revenue, expense",
            icon: "wallet",
          },
          {
            label: "Journal Entries",
            value: loading ? "..." : String(journalsCount).padStart(2, "0"),
            foot: `Double-entry immutable records`,
            icon: "book",
          },
          {
            label: "Zero-Sum Invariant",
            value: loading ? "..." : formatMoney(difference, currency),
            foot: !auditAvailable ? "Audit unavailable · open /audit" : isBalanced ? "Live ledger · Mathematically balanced" : "Discrepancy detected in ledger",
            icon: "shield",
          },
        ]}
      />

      <div className="overview-grid">
        <Panel
          title="Recent transaction volume"
          description="Chronological journal posting volume"
          action={
            <Badge tone={!auditAvailable ? "neutral" : isBalanced ? "green" : "amber"}>
              {!auditAvailable ? "Invariant unavailable" : isBalanced ? `${currency} Invariant Verified` : "Review Invariant"}
            </Badge>
          }
        >
          <div className="chart-summary">
            <strong>{loading ? "..." : formatMoney(totalDebits, currency)}</strong>
            <span>Live ledger</span>
          </div>
          <ActivityChart currency={currency} journals={recentJournals} />
          <div className="chart-legend">
            <span>
              <i />
              Posted transfers
            </span>
            <span>Real-time PostgreSQL engine</span>
          </div>
        </Panel>

        <BalanceCard
          currency={currency}
          totalDebits={totalDebits}
          totalCredits={totalCredits}
          difference={difference}
          balanced={isBalanced}
          loading={loading}
        />
      </div>

      <Panel
        title="Recent journal entries"
        description="Append-only immutable record of financial movements."
        action={
          <Link href="/ledger" className="text-link">
            View all entries <Icon name="arrow" size={16} />
          </Link>
        }
      >
        <JournalTable journals={recentJournals} loading={loading} compact />
      </Panel>

      <QuickLinks />
    </>
  );
}
