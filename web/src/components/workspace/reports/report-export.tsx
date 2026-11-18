"use client";

import { journals } from "@/lib/demo-data";
import { notify } from "@/lib/notify";
import { Icon } from "../../icons";
import { Heading } from "../../ui";

function downloadPostingsCsv() {
  const header = ["journal_id", "sample_date_jst", "time_jst", "account_id", "direction", "currency", "amount_minor_units"];
  const rows = journals.flatMap((j) => j.postings.map((p) => [j.id, "2026-11-15", j.time, p.accountId, p.direction, "JPY", p.amount]));
  const csv = [header, ...rows].map((row) => row.join(",")).join("\r\n");
  const url = URL.createObjectURL(new Blob([csv], { type: "text/csv;charset=utf-8" }));
  const link = document.createElement("a");
  link.href = url;
  link.download = "paycore-sample-postings.csv";
  link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
  return rows.length;
}

/** Page heading with the CSV export button; the result is announced as a toast. */
export function ReportExport() {
  const exportCsv = () => {
    const count = downloadPostingsCsv();
    notify.success(`Sample CSV generated with ${count} postings in integer minor units.`);
  };
  return (
    <Heading
      eyebrow="REPORTING & AUDIT"
      title="Reports & statements"
      description="A readable picture of the sample ledger, with a portable posting export."
      action={
        <button className="button primary" onClick={exportCsv}>
          <Icon name="download" size={17} /> Export sample CSV
        </button>
      }
    />
  );
}
