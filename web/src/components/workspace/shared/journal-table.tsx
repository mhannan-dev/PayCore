"use client";

import { useState } from "react";
import type { Journal as ApiJournal } from "@/lib/ledger-api";
import { formatMoney } from "@/lib/money";
import { Icon } from "../../icons";
import { Badge, Empty } from "../../ui";

const when = (iso: string) => {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "—" : d.toLocaleString(undefined, { dateStyle: "short", timeStyle: "short" });
};

function statusBadge(j: ApiJournal) {
  if (j.reversal_for_id) return <Badge tone="amber">Reversal</Badge>;
  if (j.reversed_by_id) return <Badge tone="neutral">Reversed</Badge>;
  return <Badge>Posted</Badge>;
}

function PostingDetail({ journal }: { journal: ApiJournal }) {
  return (
    <tr>
      <td colSpan={6} className="expanded-cell">
        <div className="posting-detail">
          {journal.postings.map((p) => (
            <div key={p.id}>
              <span className="mono">{p.account_number}</span>
              <strong>{p.account_name}</strong>
              <Badge tone={p.direction === "DEBIT" ? "neutral" : "green"}>{p.direction}</Badge>
              <span className="money">{formatMoney(p.amount, journal.currency)}</span>
            </div>
          ))}
        </div>
        <p className="footnote">
          Journal <span className="mono">{journal.id}</span> · Ref <span className="mono">{journal.reference_id}</span>
          {journal.created_by_email && <> · by {journal.created_by_email}</>}
        </p>
      </td>
    </tr>
  );
}

function JournalRow({
  journal: j,
  expanded,
  onToggle,
}: {
  journal: ApiJournal;
  expanded: boolean;
  onToggle: () => void;
}) {
  return (
    <>
      <tr>
        <td>
          <strong className="cell-primary">{j.description}</strong>
          <span className="cell-secondary mono">{j.reference_id}</span>
        </td>
        <td>
          <Badge tone="neutral">{j.currency}</Badge>
        </td>
        <td className="money">{formatMoney(j.total, j.currency)}</td>
        <td>{statusBadge(j)}</td>
        <td className="muted">{when(j.posted_at)}</td>
        <td>
          <button
            type="button"
            className="row-button"
            aria-expanded={expanded}
            aria-label={`${expanded ? "Hide" : "View"} ${j.reference_id} postings`}
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

export function JournalTable({
  journals = [],
  loading = false,
  compact = false,
}: {
  journals?: ApiJournal[];
  loading?: boolean;
  compact?: boolean;
}) {
  const [expanded, setExpanded] = useState<string | null>(null);

  if (loading && journals.length === 0) {
    return (
      <div className="table-scroll">
        <Empty>Loading journals from ledger...</Empty>
      </div>
    );
  }

  const rows = compact ? journals.slice(0, 5) : journals;

  return (
    <div className="table-scroll">
      <table>
        <caption className="sr-only">Live journal entries; expand rows to inspect postings</caption>
        <thead>
          <tr>
            <th>JOURNAL / REFERENCE</th>
            <th>CURRENCY</th>
            <th>TOTAL AMOUNT</th>
            <th>STATUS</th>
            <th>POSTED AT</th>
            <th>
              <span className="sr-only">Details</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {rows.map((j) => (
            <JournalRow
              key={j.id}
              journal={j}
              expanded={expanded === j.id}
              onToggle={() => setExpanded(expanded === j.id ? null : j.id)}
            />
          ))}
        </tbody>
      </table>
      {rows.length === 0 && <Empty>No journal entries found in the ledger.</Empty>}
    </div>
  );
}
