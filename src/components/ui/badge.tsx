import type { ReactNode } from "react";

export type BadgeTone = "green" | "amber" | "neutral";

export function Badge({ children, tone = "green" }: { children: ReactNode; tone?: BadgeTone }) {
  return <span className={`badge ${tone}`}>{children}</span>;
}
