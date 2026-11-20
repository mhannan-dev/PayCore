"use client";

import { useEffect, useState } from "react";
import * as yup from "yup";
import { AuthApiError } from "@/lib/auth-api";
import { validateSession } from "@/lib/auth-session";
import { configureMatching, fetchMatchingReport, reviewStatementMatch, STATEMENT_DIRECTION_MODES, type MatchingReport, type MatchingRow, type MatchAction, type MatchStatus } from "@/lib/ledger-api";
import { formatMoney } from "@/lib/money";
import { Icon } from "../icons";

const modeSchema = yup.object({ direction_mode: yup.mixed<(typeof STATEMENT_DIRECTION_MODES)[number]>().oneOf(STATEMENT_DIRECTION_MODES).required() });
const noteSchema = yup.object({ reason: yup.string().trim().min(3, "Enter at least 3 characters.").max(500).required("Enter a review reason.") });
type NoteForm = yup.InferType<typeof noteSchema>;
const labels: Record<MatchStatus, string> = { PENDING: "Choose a direction mapping", UNMATCHED: "No exact candidate", SUGGESTED: "Suggested match", AMBIGUOUS: "Multiple candidates", MATCHED: "Confirmed", REJECTED: "Candidates rejected" };

export function MatchingReview({ token, importId, canReview }: { token: string; importId: string; canReview: boolean }) {
  const [report, setReport] = useState<MatchingReport | null>(null);
  const [mode, setMode] = useState("");
  const [offset, setOffset] = useState(0);
  const [revision, setRevision] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  function failed(err: unknown) {
    setError(err instanceof Error ? err.message : "Could not load statement matching.");
    if (err instanceof AuthApiError && err.code === "invalid_credentials") void validateSession();
  }
  useEffect(() => {
    let active = true;
    void fetchMatchingReport(token, importId, offset).then((data) => { if (active) { setReport(data); setMode(data.direction_mode ?? ""); } }).catch((err: unknown) => { if (active) failed(err); });
    return () => { active = false; };
  }, [token, importId, offset, revision]);

  async function configure() {
    if (!report) return;
    setBusy(true); setError(""); setNotice("");
    try {
      const input = await modeSchema.validate({ direction_mode: mode });
      await configureMatching(token, importId, { ...input, expected_version: report.version });
      setReport(await fetchMatchingReport(token, importId, offset));
      setNotice("Direction mapping saved. Exact candidates refreshed.");
    } catch (err) { failed(err); } finally { setBusy(false); }
  }
  async function review(row: number, action: MatchAction, postingId: string | null, reason: string) {
    if (!report) return;
    setBusy(true); setError(""); setNotice("");
    try {
      await reviewStatementMatch(token, importId, row, { action, posting_id: postingId, reason, expected_version: report.version });
      setReport(await fetchMatchingReport(token, importId, offset));
      setNotice(action === "CONFIRM" ? "Match confirmed. Ledger balances are unchanged." : "Review saved.");
    } catch (err) { failed(err); } finally { setBusy(false); }
  }
  function page(start: number) { setReport(null); setOffset(start); setNotice(""); setError(""); }

  return <section className="statement-card statement-matching" aria-labelledby="matching-heading">
    <div className="statement-section-heading"><div><h2 id="matching-heading">Match & review</h2><p>Exact account, currency, amount, reference, UTC date, and mapped direction. Confirmations link existing postings without changing balances.</p></div>
      <button className="button secondary" type="button" disabled={busy} onClick={() => { setError(""); setReport(null); setRevision((v) => v + 1); }}><Icon name="refresh" size={16} /> Refresh matches</button>
    </div>
    {error && <p className="statement-alert error" role="alert">{error}</p>}
    {notice && <p className="statement-alert" role="status">{notice}</p>}
    {report ? <>
      {canReview ? <form className="matching-config" onSubmit={(event) => { event.preventDefault(); void configure(); }}>
        <label>Bank → ledger direction<select value={mode} disabled={busy} required onChange={(event) => setMode(event.target.value)}><option value="">Choose your statement convention</option><option value="OPPOSITE">Opposite — bank CREDIT → ledger DEBIT</option><option value="SAME">Same — bank CREDIT → ledger CREDIT</option></select></label>
        <button className="button" type="submit" disabled={busy || !mode || mode === report.direction_mode}><Icon name="save" size={16} /> Apply mapping</button>
        <p>Confirm your bank&apos;s convention before applying. Changing mapping requires resetting confirmed matches and clears previous candidate rejections.</p>
      </form> : <p>Direction mapping: <strong>{report.direction_mode ?? "Not configured by uploader"}</strong>. Your permissions allow viewing only.</p>}
      <p className="statement-footnote">Statuses below cover this page. Unmatched rows need investigation; record a note for follow-up. Candidates already reconciled elsewhere are unavailable.</p>
      <div className="matching-rows">{report.rows.map((row) => <MatchRow key={`${report.version}:${row.row_number}`} row={row} busy={busy} canReview={canReview} onReview={review} />)}</div>
      <div className="statement-pager"><button className="button secondary" type="button" disabled={busy || offset === 0} onClick={() => page(Math.max(0, offset - 50))}><Icon name="arrow" size={16} /> Previous rows</button><span>{offset + 1}–{Math.min(offset + report.rows.length, report.import.row_count)} of {report.import.row_count}</span><button className="button secondary" type="button" disabled={busy || offset + 50 >= report.import.row_count} onClick={() => page(offset + 50)}><Icon name="arrow" size={16} /> Next rows</button></div>
    </> : <p role="status">Loading matching review…</p>}
  </section>;
}

function MatchRow({ row, busy, canReview, onReview }: { row: MatchingRow; busy: boolean; canReview: boolean; onReview: (row: number, action: MatchAction, postingId: string | null, reason: string) => Promise<void> }) {
  const [reason, setReason] = useState("");
  const [error, setError] = useState("");
  async function act(action: MatchAction, postingId: string | null) {
    setError("");
    try {
      const input: NoteForm = action === "CONFIRM" ? { reason: "" } : await noteSchema.validate({ reason });
      await onReview(row.row_number, action, postingId, input.reason);
    } catch (err) { setError(err instanceof Error ? err.message : "Enter a review reason."); }
  }
  return <article className="matching-row">
    <div className="statement-section-heading"><div><strong>Row {row.row_number} · {row.entry.reference}</strong><p>{row.entry.booked_date} · {formatMoney(row.entry.amount, row.entry.currency)} · Bank {row.entry.direction}</p></div><span className={`matching-status matching-status-${row.status.toLowerCase()}`}>{labels[row.status]}</span></div>
    {row.entry.description && <p>{row.entry.description}</p>}
    {row.matched_posting_id && <p className="matching-id">Posting: {row.matched_posting_id}</p>}
    {row.note && <blockquote><strong>Review note:</strong> {row.note}</blockquote>}
    {row.candidates.map((c) => <div key={c.posting_id} className="matching-candidate"><div><strong>{c.reference} · {c.direction} · {formatMoney(c.amount, row.entry.currency)}</strong><small>{new Date(c.posted_at).toISOString()} · Posting {c.posting_id}</small></div>{canReview && <div className="matching-actions"><button className="button" type="button" disabled={busy} onClick={() => void act("CONFIRM", c.posting_id)}><Icon name="shield" size={16} /> Confirm match</button><button className="button secondary" type="button" disabled={busy} onClick={() => void act("REJECT", c.posting_id)}><Icon name="close" size={16} /> Reject candidate</button></div>}</div>)}
    {row.candidate_count > row.candidates.length && <p>{row.candidate_count} exact candidates; showing the first {row.candidates.length}. Rejecting candidates reveals the next ones.</p>}
    {canReview && <div className="matching-note"><label htmlFor={`matching-note-${row.row_number}`}>Review reason / follow-up note</label><textarea id={`matching-note-${row.row_number}`} maxLength={500} value={reason} disabled={busy} placeholder="Required for rejecting, resetting, or saving a note" onChange={(event) => setReason(event.target.value)} /><div className="matching-actions"><button className="button secondary" type="button" disabled={busy} onClick={() => void act("NOTE", null)}><Icon name="save" size={16} /> Save note</button>{(row.matched_posting_id !== null || row.rejected_count > 0) && <button className="button secondary" type="button" disabled={busy} onClick={() => void act("RESET", null)}><Icon name="refresh" size={16} /> Reset row review</button>}</div>{error && <p className="matching-error" role="alert">{error}</p>}</div>}
  </article>;
}
