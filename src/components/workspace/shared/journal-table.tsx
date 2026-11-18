"use client";

import { Icon } from "../../icons";

import { useState } from "react";
import { accountName, journals, money, type Journal } from "@/lib/demo-data";
import { Badge, Empty } from "../../ui";

function matches(journal: Journal, query: string) {
  const haystack = `${journal.id} ${journal.description} ${journal.category} ${journal.postings
    .map((p) => `${p.accountId} ${accountName(p.accountId)}`)
    .join(" ")}`;
  return haystack.toLowerCase().includes(query.toLowerCase());
}

function PostingDetail({ journal }: { journal: Journal }) {
  return (
    <tr>
      <td colSpan={6} className="expanded-cell">
        <div className="posting-detail">
          {journal.postings.map((p) => (
            <div key={`${p.accountId}-${p.direction}`}>
              <span className="mono">{p.accountId}</span>
              <strong>{accountName(p.accountId)}</strong>
              <Badge tone={p.direction === "DEBIT" ? "neutral" : "green"}>{p.direction}</Badge>
              <span className="money">{money(p.amount)}</span>
            </div>
          ))}
        </div>
      </td>
    </tr>
  );
}

function JournalRow({ journal: j, expanded, onToggle }: { journal: Journal; expanded: boolean; onToggle: () => void }) {
  return (
    <>
      <tr>
        <td>
          <strong className="cell-primary">{j.description}</strong>
          <span className="cell-secondary mono">{j.id}</span>
        </td>
        <td>{j.category}</td>
        <td className="money">{money(j.postings[0]?.amount ?? "0")}</td>
        <td><Badge>Posted</Badge></td>
        <td className="muted">{j.time}</td>
        <td>
          <button
            className="row-button"
            aria-expanded={expanded}
            aria-label={`${expanded ? "Hide" : "View"} ${j.id} postings`}
            onClick={onToggle}
          >
            <Icon name={expanded ? "minus" : "plus"} size={14} />
          </button>
        </td>
      </tr>
      {expanded && <PostingDetail journal={j} />}
    </>
  );
}

export function JournalTable({ query = "", compact = false }: { query?: string; compact?: boolean }) {
  const [expanded, setExpanded] = useState<string | null>(null);
  const rows = journals.filter((j) => matches(j, query));
  return (
    <div className="table-scroll">
      <table>
        <caption className="sr-only">Sample journals; expand rows to inspect postings</caption>
        <thead>
          <tr>
            <th>JOURNAL / DESCRIPTION</th><th>TYPE</th><th>AMOUNT</th><th>STATUS</th><th>TIME (JST)</th>
            <th><span className="sr-only">Details</span></th>
          </tr>
        </thead>
        <tbody>
          {rows.slice(0, compact ? 4 : rows.length).map((j) => (
            <JournalRow
              key={j.id}
              journal={j}
              expanded={expanded === j.id}
              onToggle={() => setExpanded(expanded === j.id ? null : j.id)}
            />
          ))}
        </tbody>
      </table>
      {rows.length === 0 && <Empty>No entries match your search.</Empty>}
    </div>
  );
}
