"use client";

import { useState } from "react";
import { fetchAccountPostings, type LedgerAccount } from "@/lib/ledger-api";
import { formatMoney } from "@/lib/money";
import { Badge, Empty, Panel } from "../../ui";
import { Pagination } from "../../ui/pagination";
import { ApiStatus } from "../shared/api-status";
import { useApi } from "../shared/use-api";

const PAGE_SIZE = 20;

const when = (iso: string) => {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "—" : d.toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" });
};

/** Paged postings of one account, newest first. */
export function AccountPostings({ account }: { account: LedgerAccount }) {
  const [page, setPage] = useState(0);
  const list = useApi(JSON.stringify([account.id, page]), (token) => fetchAccountPostings(token, account.id, PAGE_SIZE, page * PAGE_SIZE));
  const rows = list.data?.items ?? [];
  return (
    <Panel title={`${account.name} · Postings`} description={`${account.account_number} · ${account.type} · balance ${formatMoney(account.balance, account.currency)}`}>
      <ApiStatus loading={list.loading && !list.data} error={list.error} forbidden={list.forbidden} onRetry={list.reload} />
      {list.data && (
        <div className="table-scroll">
          <table>
            <thead><tr><th>WHEN</th><th>REFERENCE</th><th>DESCRIPTION</th><th>SIDE</th><th>AMOUNT</th></tr></thead>
            <tbody>
              {rows.map((p) => (
                <tr key={p.id}>
                  <td className="muted">{when(p.created_at)}</td>
                  <td className="mono">{p.reference_id}{p.reversal_for_id && <> <Badge tone="amber">Reversal</Badge></>}</td>
                  <td>{p.description}</td>
                  <td><Badge tone={p.direction === "DEBIT" ? "neutral" : "green"}>{p.direction}</Badge></td>
                  <td className="money">{formatMoney(p.amount, account.currency)}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {rows.length === 0 && <Empty>No postings yet.</Empty>}
        </div>
      )}
      {(list.data?.total ?? 0) > PAGE_SIZE && (
        <Pagination label="Postings pages" page={page} pageSize={PAGE_SIZE} total={list.data?.total ?? 0} onPageChange={setPage} disabled={list.loading} />
      )}
    </Panel>
  );
}
