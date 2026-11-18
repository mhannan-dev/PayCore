import Link from "next/link";
import { balanceOf, money } from "@/lib/demo-data";
import { Icon } from "../../icons";
import { Badge, Heading, Notice, Panel, StatGrid } from "../../ui";

function PayoutSummary() {
  return (
    <Panel title="STL-0001 · Hana payout" action={<Badge>Sample completed</Badge>}>
      <div className="summary-list">
        <div><span>Source wallet</span><strong>Hana · Wallet</strong></div>
        <div><span>Payout amount</span><strong>{money("25000")}</strong></div>
        <div><span>Provider reference</span><code>SANDBOX-0001</code></div>
        <div><span>Clearing balance after capture</span><strong>{money(balanceOf("2003"))}</strong></div>
      </div>
      <div className="panel-bottom">
        <Link href="/ledger" className="text-link">View reserve and capture journals <Icon name="arrow" size={16} /></Link>
      </div>
    </Panel>
  );
}

const STEPS = [
  { title: "Funds reserved", text: "14:40 · JRN-0005 moved ¥25,000 into clearing." },
  { title: "Sandbox provider confirmed", text: "Illustrative success. No external request was made." },
  { title: "Funds captured", text: "14:42 · JRN-0006 cleared the reserved payout." },
];

function PayoutTimeline() {
  return (
    <Panel title="Sample workflow" description="15 Nov 2026 · JST">
      <ol className="timeline">
        {STEPS.map((step) => <li key={step.title}><strong>{step.title}</strong><p>{step.text}</p></li>)}
      </ol>
    </Panel>
  );
}

export function Settlements() {
  return (
    <>
      <Heading eyebrow="SETTLEMENT SERVICE" title="Payouts & reconciliation" description="Inspect a completed sandbox payout, from reservation to capture." />
      <StatGrid
        three
        stats={[
          { label: "Sample payout", value: "¥25,000", foot: "One sandbox workflow", icon: "bank" },
          { label: "Workflow state", value: "Captured", foot: "JRN-0005 → JRN-0006", icon: "shield" },
          { label: "Provider mode", value: "Sandbox", foot: "No real bank connection", icon: "code" },
        ]}
      />
      <div className="form-grid">
        <PayoutSummary />
        <PayoutTimeline />
      </div>
      <Notice icon="bank">
        <span>Payout creation requires reserve/capture/release APIs and a sandbox adapter. Ambiguous timeouts remain unresolved until reconciled.</span>
      </Notice>
    </>
  );
}
