"use client";

import { useState } from "react";
import { fetchAccounts, fetchSettlements, type LedgerAccount, type Settlement } from "@/lib/ledger-api";
import { formatMoney } from "@/lib/money";
import { PERMISSIONS } from "@/lib/rbac";
import { useWorkspaceCurrency } from "@/lib/workspace-currency";
import { usePermission } from "../../can";
import { Icon } from "../../icons";
import { Badge, Empty, Heading, Notice, Panel } from "../../ui";
import { Pagination } from "../../ui/pagination";
import { ApiStatus } from "../shared/api-status";
import { useApi } from "../shared/use-api";
import { PayoutDetails, PayoutStatus } from "./payout-details";
import { PayoutForm } from "./payout-form";
import "./settlements.css";

const PAGE_SIZE = 25;

export function Settlements() {
  const { currency } = useWorkspaceCurrency();
  const can = usePermission();
  const [page, setPage] = useState(0);
  const [version, setVersion] = useState(0);
  const [creating, setCreating] = useState(false);
  const [selected, setSelected] = useState<string | null>(null);
  const canCreate = can(PERMISSIONS.transfersCreate) && can(PERMISSIONS.ledgerRead);
  const list = useApi(JSON.stringify(["payouts", currency, page, version]), (token) => fetchSettlements(token, { currency, limit: PAGE_SIZE, offset: page * PAGE_SIZE }), 15000);
  const accounts = useApi<LedgerAccount[]>(JSON.stringify(["payout-accounts", currency, creating, canCreate, version]), async (token) => {
    if (!creating || !canCreate) return [];
    const items: LedgerAccount[] = [];
    let offset = 0;
    while (true) {
      const result = await fetchAccounts(token, { currency, limit: 200, offset });
      items.push(...result.items);
      offset += result.items.length;
      if (offset >= result.total || result.items.length === 0) return items;
    }
  });
  function updated(payout: Settlement) {
    setSelected(payout.id);
    setVersion((value) => value + 1);
  }
  const rows = list.data?.items ?? [];
  const total = list.data?.total ?? 0;
  return <div className="settlements-page">
    <Heading eyebrow="SETTLEMENT SERVICE" title="Payouts & reconciliation" description={`Reserve, capture and release ${currency} payouts against the live ledger.`}
      action={canCreate ? <button type="button" className="button primary" disabled={creating} onClick={() => setCreating(true)}><Icon name="plus" size={16} /> New payout</button> : undefined} />
    <Notice icon="bank">Sandbox payouts use live ledger balances. Capture records a sandbox completion; no external bank transfer is sent.</Notice>
    {!canCreate && <Notice icon="lock">Creating payouts requires transfers:create and ledger:read to select a source wallet.</Notice>}
    {creating && canCreate && <>
      <ApiStatus loading={accounts.loading} error={accounts.error} forbidden={accounts.forbidden} onRetry={accounts.reload} />
      {accounts.data && !accounts.loading && !accounts.error && <PayoutForm currency={currency} accounts={accounts.data} onClose={() => setCreating(false)} onCreated={(payout) => { setCreating(false); setPage(0); updated(payout); }} />}
      {(accounts.loading || accounts.error) && <button type="button" className="text-link" onClick={() => setCreating(false)}>Cancel new payout</button>}
    </>}
    <Panel title={`${currency} payouts`} description="Newest first. Select a payout to inspect its state and ledger journals." action={<Badge tone="neutral">{list.loading ? "Refreshing…" : `${total} payouts`}</Badge>}>
      <ApiStatus loading={list.loading && !list.data} error={list.error} forbidden={list.forbidden} onRetry={list.reload} />
      {list.data && <div className={`table-scroll${list.loading ? " is-refreshing" : ""}`}><table>
        <caption className="sr-only">Payouts in {currency}</caption>
        <thead><tr><th>REFERENCE / WALLET</th><th>BENEFICIARY</th><th>AMOUNT</th><th>STATUS</th><th>CREATED</th><th>DETAILS</th></tr></thead>
        <tbody>{rows.map((payout) => <tr key={payout.id}>
          <td><strong className="cell-primary">{payout.payout_reference}</strong><span className="cell-secondary">{payout.source_account_name}</span></td>
          <td>{payout.beneficiary_name}<span className="cell-secondary">{payout.beneficiary_bank}</span></td>
          <td className="money">{formatMoney(payout.amount, payout.currency)} <small>{payout.currency}</small></td>
          <td><PayoutStatus status={payout.status} /></td><td>{new Date(payout.created_at).toLocaleString()}</td>
          <td><button type="button" className="text-link" aria-expanded={selected === payout.id} onClick={() => setSelected(selected === payout.id ? null : payout.id)}>{selected === payout.id ? "Close" : "View payout"}</button></td>
        </tr>)}</tbody>
      </table>{rows.length === 0 && <Empty>No {currency} payouts yet.</Empty>}</div>}
      {total > PAGE_SIZE && <Pagination label="Payout pages" page={page} pageSize={PAGE_SIZE} total={total} onPageChange={setPage} disabled={list.loading} />}
    </Panel>
    {selected && <PayoutDetails key={selected} id={selected} version={version} onClose={() => setSelected(null)} onUpdated={updated} />}
  </div>;
}
