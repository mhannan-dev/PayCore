import { Badge, Heading, Panel } from "../../ui";
import { LedgerSearch } from "./ledger-search";

export function Ledger() {
  return (
    <>
      <Heading eyebrow="THE FINANCIAL RECORD" title="Ledger explorer" description="Follow every debit and credit. Expand an entry to inspect both sides." />
      <Panel title="Journal entries" action={<Badge>6 / 6 balanced</Badge>}>
        <LedgerSearch />
      </Panel>
    </>
  );
}
