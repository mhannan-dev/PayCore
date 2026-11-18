import Link from "next/link";
import { money, total } from "@/lib/demo-data";
import { Icon } from "../../icons";

export function BalanceCard() {
  return (
    <section className="balance-panel">
      <div className="balance-icon"><Icon name="shield" size={28} /></div>
      <p className="eyebrow">DOUBLE-ENTRY INTEGRITY</p>
      <h2>Every entry.<br />In balance.</h2>
      <p>The sample ledger balances across all six journals. No unexplained difference.</p>
      <div className="balance-equation">
        <span>Debits<strong>{money(total("DEBIT"))}</strong></span>
        <span>=</span>
        <span>Credits<strong>{money(total("CREDIT"))}</strong></span>
      </div>
      <Link href="/audit" className="balance-link">Explore invariant audit <Icon name="arrow" size={18} /></Link>
    </section>
  );
}
