"use client";

import { useEffect, useState } from "react";
import { ACCOUNT_TYPES, CURRENCIES, fetchAccounts, type AccountType, type Currency, type LedgerAccount } from "@/lib/ledger-api";
import { formatMoney } from "@/lib/money";
import { PERMISSIONS } from "@/lib/rbac";
import { usePermission } from "../../can";
import { Icon } from "../../icons";
import { Badge, Empty, Heading, Panel, StatGrid } from "../../ui";
import { Pagination } from "../../ui/pagination";
import { ApiStatus } from "../shared/api-status";
import { useApi } from "../shared/use-api";
import { AccountPostings } from "./account-postings";
import { NewAccountDialog } from "./new-account-dialog";

const PAGE_SIZE = 25;

function useDebounced(value: string, ms = 300) {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const t = window.setTimeout(() => setDebounced(value.trim()), ms);
    return () => window.clearTimeout(t);
  }, [value, ms]);
  return debounced;
}

/** Totals of JPY balances by account type (up to 200 accounts). */
function AccountStats({ version }: { version: number }) {
  const stats = useApi(`stats#${version}`, (token) => fetchAccounts(token, { currency: "JPY", limit: 200, offset: 0 }));
  const sum = (type: AccountType) =>
    (stats.data?.items ?? []).filter((a) => a.type === type).reduce((s, a) => s + BigInt(a.balance), BigInt(0));
  const value = (type: AccountType) => (stats.data ? formatMoney(sum(type), "JPY") : "…");
  return (
    <StatGrid
      three
      stats={[
        { label: "Assets", value: value("ASSET"), foot: "JPY · e.g. operating bank", icon: "bank" },
        { label: "Liabilities", value: value("LIABILITY"), foot: "JPY · customer wallets, clearing", icon: "wallet" },
        { label: "Revenue", value: value("REVENUE"), foot: "JPY · fees earned", icon: "chart" },
      ]}
    />
  );
}

function AccountRow({ account: a, open, onToggle }: { account: LedgerAccount; open: boolean; onToggle: () => void }) {
  const negative = a.balance.startsWith("-");
  return (
    <tr>
      <td>
        <strong className="cell-primary">{a.name}</strong>
        <span className="cell-secondary mono">{a.account_number}</span>
      </td>
      <td><Badge tone="neutral">{a.type}</Badge></td>
      <td>{a.currency}</td>
      <td className={`money${negative ? " money-negative" : ""}`}>{formatMoney(a.balance, a.currency)}</td>
      <td>
        {!a.is_active && <Badge tone="amber">Inactive</Badge>}
        {a.overdraft_allowed && <Badge tone="neutral">Overdraft</Badge>}
      </td>
      <td>
        <button className="text-link" aria-expanded={open} onClick={onToggle}>
          <Icon name={open ? "close" : "book"} size={14} />{open ? "Close" : "Postings"}
        </button>
      </td>
    </tr>
  );
}

export function AccountsPage() {
  const can = usePermission();
  const [query, setQuery] = useState("");
  const search = useDebounced(query);
  const [type, setType] = useState<AccountType | "">("");
  const [currency, setCurrency] = useState<Currency | "">("");
  const [page, setPage] = useState(0);
  const [selected, setSelected] = useState<LedgerAccount | null>(null);
  const [creating, setCreating] = useState(false);
  const [version, setVersion] = useState(0);

  const list = useApi(JSON.stringify([search, type, currency, page, version]), (token) =>
    fetchAccounts(token, { q: search, type, currency, limit: PAGE_SIZE, offset: page * PAGE_SIZE }));
  const rows = list.data?.items ?? [];
  const total = list.data?.total ?? 0;

  return (
    <>
      <Heading
        eyebrow="LEDGER CORE"
        title="Chart of accounts"
        description="Every account and its balance, derived from posted journals. Select an account to see its postings."
        action={can(PERMISSIONS.accountsWrite) ? (
          <button type="button" className="button primary" onClick={() => setCreating(true)}><Icon name="plus" size={16} /> New account</button>
        ) : undefined}
      />
      <AccountStats version={version} />
      <Panel title="Accounts" action={<Badge tone="neutral">{total} accounts</Badge>}>
        <div className="toolbar">
          <label className="search-field">
            <span className="sr-only">Search accounts</span>
            <input placeholder="Search account name or number…" value={query} maxLength={100} onChange={(e) => { setQuery(e.target.value); setPage(0); }} />
          </label>
          <label>
            <span className="sr-only">Account type</span>
            <select value={type} onChange={(e) => { setType(ACCOUNT_TYPES.find((t) => t === e.target.value) ?? ""); setPage(0); }}>
              <option value="">All types</option>
              {ACCOUNT_TYPES.map((t) => <option key={t} value={t}>{t}</option>)}
            </select>
          </label>
          <label>
            <span className="sr-only">Currency</span>
            <select value={currency} onChange={(e) => { setCurrency(CURRENCIES.find((c) => c === e.target.value) ?? ""); setPage(0); }}>
              <option value="">All currencies</option>
              {CURRENCIES.map((c) => <option key={c} value={c}>{c}</option>)}
            </select>
          </label>
        </div>
        <ApiStatus loading={list.loading && !list.data} error={list.error} forbidden={list.forbidden} onRetry={list.reload} />
        {list.data && (
          <div className={`table-scroll${list.loading ? " is-refreshing" : ""}`}>
            <table>
              <thead><tr><th>ACCOUNT</th><th>TYPE</th><th>CURRENCY</th><th>BALANCE</th><th>FLAGS</th><th><span className="sr-only">Details</span></th></tr></thead>
              <tbody>
                {rows.map((a) => (
                  <AccountRow key={a.id} account={a} open={selected?.id === a.id} onToggle={() => setSelected(selected?.id === a.id ? null : a)} />
                ))}
              </tbody>
            </table>
            {rows.length === 0 && <Empty>{search || type || currency ? "No matching accounts." : "No accounts yet."}</Empty>}
          </div>
        )}
        {total > PAGE_SIZE && (
          <Pagination label="Accounts pages" page={page} pageSize={PAGE_SIZE} total={total} onPageChange={setPage} disabled={list.loading} />
        )}
      </Panel>
      {selected && <AccountPostings account={selected} />}
      {creating && (
        <NewAccountDialog
          onClose={() => setCreating(false)}
          onCreated={() => { setCreating(false); setVersion((v) => v + 1); }}
        />
      )}
    </>
  );
}
