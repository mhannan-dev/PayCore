"use client";

import { useWorkspaceCurrency } from "@/lib/workspace-currency";
import { useEffect, useState } from "react";
import { useAuthSession } from "@/lib/auth-session";
import { fetchJournals, reverseJournal, type Journal } from "@/lib/ledger-api";
import { formatMoney } from "@/lib/money";
import { notify } from "@/lib/notify";
import { PERMISSIONS } from "@/lib/rbac";
import { ConfirmDialog } from "../../admin/confirm-dialog";
import { usePermission } from "../../can";
import { Icon } from "../../icons";
import { Badge, Empty } from "../../ui";
import { Pagination } from "../../ui/pagination";
import { ApiStatus } from "../shared/api-status";
import { useApi } from "../shared/use-api";

const PAGE_SIZE = 20;

const when = (iso: string) => {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "—" : d.toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" });
};

function status(j: Journal) {
  if (j.reversal_for_id) return <Badge tone="amber">Reversal</Badge>;
  if (j.reversed_by_id) return <Badge tone="neutral">Reversed</Badge>;
  return <Badge>Posted</Badge>;
}

function Postings({ journal }: { journal: Journal }) {
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
          Journal <span className="mono">{journal.id}</span>
          {journal.created_by_email && <> · posted by {journal.created_by_email}</>}
          {journal.reversal_for_id && <> · reverses <span className="mono">{journal.reversal_for_id}</span></>}
          {journal.reversed_by_id && <> · reversed by <span className="mono">{journal.reversed_by_id}</span></>}
        </p>
      </td>
    </tr>
  );
}

export function LedgerSearch() {
  const { currency } = useWorkspaceCurrency();
  const { session } = useAuthSession();
  const can = usePermission();
  const canReverse = can(PERMISSIONS.transfersReverse);
  const [query, setQuery] = useState("");
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  const [expanded, setExpanded] = useState<string | null>(null);
  const [reversing, setReversing] = useState<{ journal: Journal; key: string } | null>(null);
  const [version, setVersion] = useState(0);
  useEffect(() => {
    const t = window.setTimeout(() => setSearch(query.trim()), 300);
    return () => window.clearTimeout(t);
  }, [query]);

  const list = useApi(JSON.stringify([currency, search, page, version]), (token) => fetchJournals(token, { currency, q: search, limit: PAGE_SIZE, offset: page * PAGE_SIZE }));
  const rows = list.data?.items ?? [];
  const total = list.data?.total ?? 0;

  return (
    <>
      <div className="toolbar">
        <label className="search-field">
          <span className="sr-only">Search ledger</span>
          <input value={query} maxLength={100} onChange={(e) => { setQuery(e.target.value); setPage(0); }} placeholder="Search reference or description…" />
        </label>
        <span className="muted">{total} journals · newest first</span>
      </div>
      <ApiStatus loading={list.loading && !list.data} error={list.error} forbidden={list.forbidden} onRetry={list.reload} />
      {list.data && (
        <div className={`table-scroll${list.loading ? " is-refreshing" : ""}`}>
          <table>
            <caption className="sr-only">Journals; expand a row to inspect its postings</caption>
            <thead><tr><th>JOURNAL / DESCRIPTION</th><th>AMOUNT</th><th>STATUS</th><th>POSTED</th><th><span className="sr-only">Actions</span></th><th><span className="sr-only">Details</span></th></tr></thead>
            <tbody>
              {rows.map((j) => (
                <JournalRow
                  key={j.id}
                  journal={j}
                  expanded={expanded === j.id}
                  onToggle={() => setExpanded(expanded === j.id ? null : j.id)}
                  onReverse={canReverse && !j.reversal_for_id && !j.reversed_by_id ? () => setReversing({ journal: j, key: crypto.randomUUID() }) : undefined}
                />
              ))}
            </tbody>
          </table>
          {rows.length === 0 && <Empty>{search ? "No journals match your search." : "No journals yet. Post one in the Transfer studio."}</Empty>}
        </div>
      )}
      {total > PAGE_SIZE && <Pagination label="Journal pages" page={page} pageSize={PAGE_SIZE} total={total} onPageChange={setPage} disabled={list.loading} />}
      {reversing && (
        <ConfirmDialog
          key={reversing.key}
          title={`Reverse ${reversing.journal.reference_id}?`}
          icon="refresh"
          tone="warning"
          confirmLabel="Post reversal"
          pendingLabel="Reversing…"
          onClose={() => setReversing(null)}
          onConfirm={async () => {
            // The key is fixed for this dialog, so a retry after a timeout cannot reverse twice.
            const reversal = await reverseJournal(session.token, reversing.key, reversing.journal.id);
            setReversing(null);
            setVersion((v) => v + 1);
            notify.success(`Reversal ${reversal.reference_id} posted.`);
          }}
        >
          <p>
            A new journal moves {formatMoney(reversing.journal.total, reversing.journal.currency)} back, posting every line of
            <strong> {reversing.journal.reference_id}</strong> on the opposite side. The original stays in the ledger unchanged.
            It fails if an account no longer has the funds.
          </p>
        </ConfirmDialog>
      )}
    </>
  );
}

function JournalRow({ journal: j, expanded, onToggle, onReverse }: { journal: Journal; expanded: boolean; onToggle: () => void; onReverse?: () => void }) {
  return (
    <>
      <tr>
        <td>
          <strong className="cell-primary">{j.description}</strong>
          <span className="cell-secondary mono">{j.reference_id}</span>
        </td>
        <td className="money">{formatMoney(j.total, j.currency)}</td>
        <td>{status(j)}</td>
        <td className="muted">{when(j.posted_at)}</td>
        <td>{onReverse && <button type="button" className="text-link" onClick={onReverse}><Icon name="refresh" size={14} />Reverse</button>}</td>
        <td>
          <button className="row-button" aria-expanded={expanded} aria-label={`${expanded ? "Hide" : "View"} ${j.reference_id} postings`} onClick={onToggle}>
            <Icon name={expanded ? "minus" : "plus"} size={14} />
          </button>
        </td>
      </tr>
      {expanded && <Postings journal={j} />}
    </>
  );
}
