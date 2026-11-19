import { Heading, Panel } from "../../ui";
import { LedgerSearch } from "./ledger-search";

export function Ledger() {
  return (
    <>
      <Heading eyebrow="THE FINANCIAL RECORD" title="Ledger explorer" description="Every journal ever posted. Expand an entry to see both sides; mistakes are corrected by reversal, never by editing." />
      <Panel title="Journal entries">
        <LedgerSearch />
      </Panel>
    </>
  );
}
