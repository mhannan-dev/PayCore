"use client";

import { Icon } from "../../icons";

import { useState } from "react";
import { accountName, accounts, balance, money, type Account } from "@/lib/demo-data";
import { Badge, Empty, Panel } from "../../ui";
import { JournalTable } from "../shared/journal-table";

const ACCOUNT_TYPES = ["All types", "ASSET", "LIABILITY", "REVENUE"] as const;

function AccountsToolbar({ query, type, onQuery, onType }: { query: string; type: string; onQuery: (v: string) => void; onType: (v: string) => void }) {
  return (
    <div className="toolbar">
      <label className="search-field">
        <span className="sr-only">Search accounts</span>
        <input placeholder="Search account name or number…" value={query} onChange={(e) => onQuery(e.target.value)} />
      </label>
      <label>
        <span className="sr-only">Account type</span>
        <select value={type} onChange={(e) => onType(e.target.value)}>
          {ACCOUNT_TYPES.map((t) => <option key={t}>{t}</option>)}
        </select>
      </label>
    </div>
  );
}

function AccountRow({ account: a, open, onToggle }: { account: Account; open: boolean; onToggle: () => void }) {
  return (
    <tr>
      <td>
        <strong className="cell-primary">{a.name}</strong>
        <span className="cell-secondary mono">ACC-{a.id}</span>
      </td>
      <td><Badge tone="neutral">{a.type}</Badge></td>
      <td>JPY</td>
      <td className="money">{money(balance(a))}</td>
      <td>
        <button className="text-link" aria-expanded={open} onClick={onToggle}><Icon name={open ? "close" : "book"} size={14} />{open ? "Close" : "View postings"}</button>
      </td>
    </tr>
  );
}

export function AccountsTable() {
  const [query, setQuery] = useState("");
  const [type, setType] = useState("All types");
  const [selected, setSelected] = useState<string | null>(null);
  const rows = accounts.filter(
    (a) => `${a.id} ${a.name}`.toLowerCase().includes(query.toLowerCase()) && (type === "All types" || a.type === type),
  );
  return (
    <>
      <Panel title="All accounts" action={<Badge tone="neutral">{rows.length} accounts</Badge>}>
        <AccountsToolbar query={query} type={type} onQuery={setQuery} onType={setType} />
        <div className="table-scroll">
          <table>
            <thead><tr><th>ACCOUNT</th><th>TYPE</th><th>CURRENCY</th><th>BALANCE</th><th>DETAILS</th></tr></thead>
            <tbody>
              {rows.map((a) => (
                <AccountRow key={a.id} account={a} open={selected === a.id} onToggle={() => setSelected(selected === a.id ? null : a.id)} />
              ))}
            </tbody>
          </table>
          {!rows.length && <Empty>No matching accounts.</Empty>}
        </div>
      </Panel>
      {selected && (
        <Panel title={`${accountName(selected)} · Postings`}>
          <JournalTable query={selected} />
        </Panel>
      )}
    </>
  );
}
