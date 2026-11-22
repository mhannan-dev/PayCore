"use client";

import type { Currency } from "@/lib/ledger-api";
import Link from "next/link";
import { formatMoney } from "@/lib/money";
import { Icon } from "../../icons";

export function BalanceCard({
  currency,
  totalDebits,
  totalCredits,
  difference,
  balanced = true,
  loading = false,
}: {
  currency: Currency;
  totalDebits: string;
  totalCredits: string;
  difference: string;
  balanced?: boolean;
  loading?: boolean;
}) {
  const debitsFormatted = loading ? "..." : formatMoney(totalDebits, currency);
  const creditsFormatted = loading ? "..." : formatMoney(totalCredits, currency);
  const diffFormatted = loading ? "..." : formatMoney(difference, currency);

  return (
    <section className="balance-panel">
      <div className="balance-icon">
        <Icon name="shield" size={28} />
      </div>
      <p className="eyebrow">DOUBLE-ENTRY INTEGRITY</p>
      <h2>
        Every entry.
        <br />
        In balance.
      </h2>
      <p>
        {balanced
          ? "The live ledger strictly balances across all journals. Zero unexplained difference."
          : `Discrepancy detected: ${diffFormatted} difference between debits and credits.`}
      </p>
      <div className="balance-equation">
        <span>
          Debits<strong>{debitsFormatted}</strong>
        </span>
        <span>=</span>
        <span>
          Credits<strong>{creditsFormatted}</strong>
        </span>
      </div>
      <Link href="/audit" className="balance-link">
        Explore invariant audit <Icon name="arrow" size={18} />
      </Link>
    </section>
  );
}
