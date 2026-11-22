import { formatMoney } from "@/lib/money";
import type { ZeroSumReport } from "@/lib/ledger-api";
import { Badge } from "../../ui";
import { Icon } from "../../icons";
import { zeroSumLabel, zeroSumTone, type ZeroSumState } from "./zero-sum-state";

/** Freshness strip for the authoritative zero-sum report; never shows a false green. */
export function ZeroSumStatus({ report, state, onRetry }: { report: ZeroSumReport | null; state: ZeroSumState; onRetry: () => void }) {
  const checked = report ? Date.parse(report.checked_at) : Number.NaN;
  const checkedLabel = Number.isFinite(checked) ? new Date(checked).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit", second: "2-digit" }) : "—";
  const detail = report
    ? `${report.journals} journals · ${report.postings} postings · Δ ${formatMoney(report.difference, report.currency)} · checked ${checkedLabel}`
    : "Waiting for the first consistent ledger snapshot.";

  return (
    <div className="chart-legend" role="status">
      <Badge tone={zeroSumTone(state)}>{zeroSumLabel(state)}</Badge>
      <span className="text-sm text-muted">{detail}</span>
      {state === "unavailable" && (
        <button type="button" className="text-link" onClick={onRetry}><Icon name="refresh" size={14} />Retry audit</button>
      )}
    </div>
  );
}
