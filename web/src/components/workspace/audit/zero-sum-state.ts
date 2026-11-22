import type { ZeroSumReport } from "@/lib/ledger-api";

/** Matches the outbox health threshold: older than one minute is not "live". */
export const ZERO_SUM_STALE_MS = 60000;

export type ZeroSumState = "loading" | "balanced" | "stale" | "unavailable" | "unbalanced" | "forbidden";
export type ZeroSumTone = "green" | "amber" | "neutral";

export function zeroSumState(
  report: ZeroSumReport | null,
  options: { loading: boolean; error: string; forbidden: boolean; now: number },
): ZeroSumState {
  if (options.forbidden) return "forbidden";
  if (options.error) return "unavailable";
  if (!report) return options.loading ? "loading" : "unavailable";
  const checked = Date.parse(report.checked_at);
  if (!Number.isFinite(checked)) return "unavailable";
  if (!report.balanced) return "unbalanced";
  if (options.now - checked > ZERO_SUM_STALE_MS) return "stale";
  if (options.loading) return "loading";
  return "balanced";
}

export function zeroSumLabel(state: ZeroSumState): string {
  switch (state) {
    case "balanced": return "Balanced (Δ = 0)";
    case "stale": return "Stale check";
    case "unavailable": return "Audit unavailable";
    case "unbalanced": return "Not balanced";
    case "forbidden": return "Access denied";
    case "loading": return "Refreshing";
  }
}

export function zeroSumTone(state: ZeroSumState): ZeroSumTone {
  if (state === "balanced") return "green";
  if (state === "loading" || state === "forbidden") return "neutral";
  return "amber";
}
