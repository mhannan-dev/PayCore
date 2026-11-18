import Link from "next/link";
import { accounts, balance, money } from "@/lib/demo-data";
import { Icon } from "../../icons";
import { Badge, Panel } from "../../ui";
import { ReportExport } from "./report-export";

function BalanceSummary() {
  return (
    <Panel title="Account balance summary" description="Normal balances include assets, liabilities, and revenue.">
      <div className="summary-list">
        {accounts.map((a) => (
          <div key={a.id}>
            <span><strong>{a.name}</strong><small>{a.type} · JPY</small></span>
            <strong className="money">{money(balance(a))}</strong>
          </div>
        ))}
      </div>
    </Panel>
  );
}

const FACTS = [["Sample date", "15 Nov 2026"], ["Journals / postings", "6 / 12"], ["Currency", "JPY"]];

function ReportingCoverage() {
  return (
    <Panel title="Reporting coverage" description="Preview uses the same fixed sample as the Core pages.">
      <div className="report-copy">
        <Badge tone="amber">Not connected</Badge>
        <h3>Ready for a reporting service.</h3>
        <p>Live projections, background exports, and freshness metrics will appear after backend integration.</p>
        <dl>{FACTS.map(([term, value]) => <div key={term}><dt>{term}</dt><dd>{value}</dd></div>)}</dl>
        <Link href="/audit" className="text-link">Inspect sample audit <Icon name="arrow" size={16} /></Link>
      </div>
    </Panel>
  );
}

export function Reports() {
  return (
    <>
      <ReportExport />
      <div className="overview-grid">
        <BalanceSummary />
        <ReportingCoverage />
      </div>
    </>
  );
}
