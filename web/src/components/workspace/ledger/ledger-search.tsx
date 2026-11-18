"use client";

import { useState } from "react";
import { JournalTable } from "../shared/journal-table";

export function LedgerSearch() {
  const [query, setQuery] = useState("");
  return (
    <>
      <div className="toolbar">
        <label className="search-field">
          <span className="sr-only">Search ledger</span>
          <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search journal, account, type or description…" />
        </label>
        <span className="muted">15 Nov 2026 · JPY</span>
      </div>
      <JournalTable query={query} />
    </>
  );
}
