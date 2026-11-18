import { money, total } from "@/lib/demo-data";
import { Badge, Panel, StatGrid } from "../../ui";
import { journalChecks, type JournalCheck } from "./audit-data";
import { AuditRunner } from "./audit-runner";

function JournalCheckRow({ check: j }: { check: JournalCheck }) {
  return (
    <tr>
      <td className="mono">{j.id}</td>
      <td className="money">{money(j.debit)}</td>
      <td className="money">{money(j.credit)}</td>
      <td>{money(j.debit - j.credit)}</td>
      <td><Badge tone={j.balanced ? "green" : "amber"}>{j.balanced ? "Balanced" : "Review"}</Badge></td>
    </tr>
  );
}

export function Audit() {
  const checks = journalChecks();
  return (
    <>
      <AuditRunner
        allBalanced={checks.every((j) => j.balanced)}
        journalCount={checks.length}
        stats={
          <StatGrid
            three
            stats={[
              { label: "Total debits", value: money(total("DEBIT")), foot: "All sample journals · JPY", icon: "book" },
              { label: "Total credits", value: money(total("CREDIT")), foot: "All sample journals · JPY", icon: "book" },
              { label: "Difference", value: money(total("DEBIT") - total("CREDIT")), foot: "Debits minus credits", icon: "shield" },
            ]}
          />
        }
      />
      <Panel title="Per-journal verification" description="Balanced totals do not verify authorization, solvency, or external settlement.">
        <div className="table-scroll">
          <table>
            <thead><tr><th>JOURNAL</th><th>DEBIT</th><th>CREDIT</th><th>DIFFERENCE</th><th>RESULT</th></tr></thead>
            <tbody>{checks.map((j) => <JournalCheckRow key={j.id} check={j} />)}</tbody>
          </table>
        </div>
      </Panel>
    </>
  );
}
