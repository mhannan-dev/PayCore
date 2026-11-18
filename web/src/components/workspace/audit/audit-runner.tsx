"use client";

import { useState, type ReactNode } from "react";
import { Icon } from "../../icons";
import { Heading, Notice } from "../../ui";

/** Heading with the "Run sample audit" button; `stats` is server-rendered and slotted in. */
export function AuditRunner({ allBalanced, journalCount, stats }: { allBalanced: boolean; journalCount: number; stats: ReactNode }) {
  const [result, setResult] = useState<string | null>(null);
  const run = () =>
    setResult(
      allBalanced
        ? `Sample audit complete: ${journalCount} journals have positive postings and matching debit/credit totals.`
        : "Sample audit failed. Review journal constraints.",
    );
  return (
    <>
      <Heading
        eyebrow="FINANCIAL INTEGRITY"
        title="Invariant audit"
        description="Verify the sample ledger arithmetic. This is not a live database audit."
        action={<button className="button primary" onClick={run}><Icon name="shield" size={17} /> Run sample audit</button>}
      />
      {stats}
      {result && <Notice icon="shield" className="audit-notice" role="status">{result}</Notice>}
    </>
  );
}
