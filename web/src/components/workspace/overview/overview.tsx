import Link from "next/link";
import { money, total } from "@/lib/demo-data";
import { Icon } from "../../icons";
import { Badge, Heading, Panel, StatGrid } from "../../ui";
import { JournalTable } from "../shared/journal-table";
import { ActivityChart } from "./activity-chart";
import { BalanceCard } from "./balance-card";
import { QuickLinks } from "./quick-links";

export function Overview() {
  const debits = money(total("DEBIT"));
  return (
    <>
      <Heading
        eyebrow="YOUR FINANCIAL WORKSPACE"
        title="A clear view of every movement."
        description="Monitor your ledger, explore transactions, and keep the books balanced."
        action={<Link className="button primary" href="/transfers"><Icon name="plus" size={17} /> New transfer</Link>}
      />
      <StatGrid
        stats={[
          { label: "Posted debit volume", value: debits, foot: "Across 6 sample journal entries", icon: "arrows" },
          { label: "Ledger accounts", value: "05", foot: "Assets, liabilities and revenue", icon: "wallet" },
          { label: "Journal entries", value: "06", foot: "12 debit / credit postings", icon: "book" },
          { label: "Ledger difference", value: "¥0", foot: "Sample ledger · All journals balanced", icon: "shield" },
        ]}
      />
      <div className="overview-grid">
        <Panel title="Daily ledger activity" description="Posted debit volume · 15 Nov 2026 · JST" action={<Badge tone="neutral">JPY</Badge>}>
          <div className="chart-summary"><strong>{debits}</strong><span>Sample activity</span></div>
          <ActivityChart />
          <div className="chart-legend"><span><i />Posted debits</span><span>Fixed demonstration data</span></div>
        </Panel>
        <BalanceCard />
      </div>
      <Panel
        title="Recent journal entries"
        description="A transparent record of every financial movement."
        action={<Link href="/ledger" className="text-link">View all entries <Icon name="arrow" size={16} /></Link>}
      >
        <JournalTable compact />
      </Panel>
      <QuickLinks />
    </>
  );
}
