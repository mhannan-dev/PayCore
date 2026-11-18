"use client";

import "./pagination.css";

/** Page numbers to show: always first and last, the current page ±1, and "…" gaps. */
function pageItems(current: number, count: number): (number | "gap")[] {
  if (count <= 7) return Array.from({ length: count }, (_, i) => i);
  const pages = new Set([0, count - 1, current - 1, current, current + 1].filter((p) => p >= 0 && p < count));
  // Avoid a lone "…" hiding a single page next to the ends.
  if (current <= 3) [1, 2, 3, 4].forEach((p) => pages.add(p));
  if (current >= count - 4) [count - 5, count - 4, count - 3, count - 2].forEach((p) => pages.add(p));
  const sorted = [...pages].sort((a, b) => a - b);
  const items: (number | "gap")[] = [];
  let previous: number | undefined;
  for (const p of sorted) {
    if (previous !== undefined && p - previous > 1) items.push("gap");
    items.push(p);
    previous = p;
  }
  return items;
}

type Props = {
  /** Zero-based page index. */
  page: number;
  pageSize: number;
  total: number;
  onPageChange: (page: number) => void;
  pageSizes?: number[];
  onPageSizeChange?: (size: number) => void;
  disabled?: boolean;
  label?: string;
};

export function Pagination({ page, pageSize, total, onPageChange, pageSizes, onPageSizeChange, disabled = false, label = "Pagination" }: Props) {
  const count = Math.max(1, Math.ceil(total / pageSize));
  const from = total === 0 ? 0 : page * pageSize + 1;
  const to = Math.min(total, (page + 1) * pageSize);
  return (
    <nav className="pagination" aria-label={label}>
      <p className="pagination-summary" aria-live="polite">
        {total === 0 ? "No results" : <>Showing <strong>{from}–{to}</strong> of <strong>{total}</strong></>}
      </p>
      <div className="pagination-controls">
        {pageSizes && onPageSizeChange && (
          <label className="pagination-size">
            <span>Rows</span>
            <select value={pageSize} disabled={disabled} onChange={(e) => onPageSizeChange(Number(e.target.value))}>
              {pageSizes.map((size) => <option key={size} value={size}>{size}</option>)}
            </select>
          </label>
        )}
        <button type="button" className="pagination-btn" disabled={disabled || page === 0} onClick={() => onPageChange(page - 1)} aria-label="Previous page">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true"><path d="m15 6-6 6 6 6" /></svg>
        </button>
        {pageItems(page, count).map((item, i) =>
          item === "gap" ? (
            <span key={`gap-${i}`} className="pagination-gap" aria-hidden="true">…</span>
          ) : (
            <button
              key={item}
              type="button"
              className={`pagination-btn${item === page ? " is-current" : ""}`}
              disabled={disabled}
              aria-current={item === page ? "page" : undefined}
              aria-label={`Page ${item + 1}`}
              onClick={() => item !== page && onPageChange(item)}
            >
              {item + 1}
            </button>
          ),
        )}
        <button type="button" className="pagination-btn" disabled={disabled || page >= count - 1} onClick={() => onPageChange(page + 1)} aria-label="Next page">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true"><path d="m9 6 6 6-6 6" /></svg>
        </button>
      </div>
    </nav>
  );
}
