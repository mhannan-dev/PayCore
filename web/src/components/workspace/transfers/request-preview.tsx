import type { Journal } from "@/lib/ledger-api";
import { formatMoney } from "@/lib/money";
import { Badge, EmptyHint, Panel } from "../../ui";

/** The journal created by the last posted transfer. */
export function TransferResult({ journal }: { journal: Journal | null }) {
  return (
    <Panel title="Posted journal" description="What the ledger recorded: one journal, both sides equal.">
      {journal ? (
        <>
          <div className="preview-status" role="status">
            <Badge>Posted</Badge>
            <span className="mono">{journal.reference_id}</span>
            <span>{formatMoney(journal.total, journal.currency)}</span>
          </div>
          <div className="posting-detail result-postings">
            {journal.postings.map((p) => (
              <div key={p.id}>
                <span className="mono">{p.account_number}</span>
                <strong>{p.account_name}</strong>
                <Badge tone={p.direction === "DEBIT" ? "neutral" : "green"}>{p.direction}</Badge>
                <span className="money">{formatMoney(p.amount, journal.currency)}</span>
              </div>
            ))}
          </div>
          <p className="footnote">Journal <span className="mono">{journal.id}</span></p>
        </>
      ) : (
        <EmptyHint icon="arrows" title="Your next movement starts here." text="Post a transfer to see the journal it creates." />
      )}
    </Panel>
  );
}
