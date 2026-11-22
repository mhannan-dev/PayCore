"use client";

import { useWorkspaceCurrency } from "@/lib/workspace-currency";
import { useCallback, useState } from "react";
import { useAuthSession } from "@/lib/auth-session";
import { fetchAccountPostings, fetchAccounts, type AccountPosting, type LedgerAccount } from "@/lib/ledger-api";
import { notify } from "@/lib/notify";
import { Icon } from "../../icons";
import { Heading } from "../../ui";

function downloadCsv(filename: string, content: string) {
  const url = URL.createObjectURL(new Blob([content], { type: "text/csv;charset=utf-8" }));
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

/** Page heading with real CSV export buttons; the result is announced as a toast. */
export function ReportExport() {
  const { currency } = useWorkspaceCurrency();
  const { session } = useAuthSession();
  const token = session.token;
  const [exporting, setExporting] = useState(false);

  const exportAccountsCsv = useCallback(async () => {
    if (!token) return;
    setExporting(true);
    try {
      const page = await fetchAccounts(token, { currency, limit: 100, offset: 0 });
      const header = ["id", "account_number", "name", "type", "currency", "balance_minor_units", "overdraft_allowed", "is_active", "created_at"];
      const rows = page.items.map((a: LedgerAccount) => [
        a.id,
        a.account_number,
        `"${a.name.replace(/"/g, '""')}"`,
        a.type,
        a.currency,
        a.balance,
        a.overdraft_allowed ? "true" : "false",
        a.is_active ? "true" : "false",
        a.created_at,
      ]);
      const csv = [header, ...rows].map((r) => r.join(",")).join("\r\n");
      downloadCsv(`paycore-accounts-${currency}-${new Date().toISOString().slice(0, 10)}.csv`, csv);
      notify.success(`Exported ${page.items.length} accounts to CSV.`);
    } catch {
      notify.error("Failed to export accounts CSV. Check server connection.");
    } finally {
      setExporting(false);
    }
  }, [token, currency]);

  const exportPostingsCsv = useCallback(async () => {
    if (!token) return;
    setExporting(true);
    try {
      // Fetch accounts first to query all postings
      const accountsPage = await fetchAccounts(token, { currency, limit: 100, offset: 0 });
      const allPostings: AccountPosting[] = [];

      for (const acc of accountsPage.items) {
        const postingsPage = await fetchAccountPostings(token, acc.id, 100, 0);
        allPostings.push(...postingsPage.items);
      }

      const header = ["posting_id", "journal_id", "reference_id", "account_number", "account_name", "direction", "currency", "amount_minor_units", "posted_at"];
      const rows = allPostings.map((p) => [
        p.id,
        p.journal_id,
        p.reference_id,
        p.account_number,
        `"${p.account_name.replace(/"/g, '""')}"`,
        p.direction,
        currency,
        p.amount,
        p.created_at,
      ]);
      const csv = [header, ...rows].map((r) => r.join(",")).join("\r\n");
      downloadCsv(`paycore-postings-${currency}-${new Date().toISOString().slice(0, 10)}.csv`, csv);
      notify.success(`Exported ${allPostings.length} ledger postings to CSV.`);
    } catch {
      notify.error("Failed to export postings CSV. Check server connection.");
    } finally {
      setExporting(false);
    }
  }, [token, currency]);

  return (
    <Heading
      eyebrow="REPORTING & AUDIT"
      title="Reports & statements"
      description="Live ledger financial statements, trial balances, and audit-grade posting exports."
      action={
        <div style={{ display: "flex", gap: "8px" }}>
          <button
            type="button"
            className="button secondary"
            disabled={exporting}
            onClick={exportAccountsCsv}
          >
            <Icon name="download" size={17} /> Export Accounts CSV
          </button>
          <button
            type="button"
            className="button primary"
            disabled={exporting}
            onClick={exportPostingsCsv}
          >
            <Icon name="download" size={17} /> Export Postings CSV
          </button>
        </div>
      }
    />
  );
}
