"use client";

import type { Currency, Journal } from "@/lib/ledger-api";
import { formatMoney } from "@/lib/money";

export function ActivityChart({ currency, journals = [] }: { currency: Currency; journals?: Journal[] }) {
  // Aggregate recent journal volumes into up to 6 chronological buckets
  const recent = [...journals].slice(0, 6).reverse();

  if (recent.length === 0) {
    return (
      <div className="activity-chart" role="img" aria-label="No recent transactions to display.">
        <div className="chart-y">
          <span>—</span>
          <span>—</span>
          <span>0</span>
        </div>
        <div className="chart-bars" style={{ alignItems: "center", justifyContent: "center" }}>
          <span className="text-xs text-muted">No ledger transactions recorded yet</span>
        </div>
      </div>
    );
  }

  // Calculate highest amount for relative bar heights
  const amounts = recent.map((j) => {
    try {
      return BigInt(j.total);
    } catch {
      return BigInt(0);
    }
  });

  let max = BigInt(1);
  for (const amt of amounts) {
    if (amt > max) max = amt;
  }

  return (
    <div
      className="activity-chart"
      role="img"
      aria-label={`Recent ledger volume across ${recent.length} transaction entries.`}
    >
      <div className="chart-y">
        <span>{formatMoney(max, currency)}</span>
        <span>{formatMoney(max / BigInt(2), currency)}</span>
        <span>0</span>
      </div>
      <div className="chart-bars">
        {recent.map((j, i) => {
          const amt = amounts[i] ?? BigInt(0);
          const pct = Math.max(8, Number((amt * BigInt(100)) / max));
          const time = new Date(j.posted_at).toLocaleTimeString(undefined, {
            hour: "2-digit",
            minute: "2-digit",
            hour12: false,
          });

          return (
            <div className="chart-column" key={j.id}>
              <div className="bar-space">
                <div className="chart-bar" style={{ height: `${pct}%` }}>
                  <span>{formatMoney(j.total, j.currency)}</span>
                </div>
              </div>
              <span>{time}</span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
