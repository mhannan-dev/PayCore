"use client";

import { useEffect, useState } from "react";
import * as yup from "yup";
import { AuthApiError, uploadAttachment } from "@/lib/auth-api";
import { useAuthSession, validateSession } from "@/lib/auth-session";
import { fetchAccounts, fetchStatementDetail, fetchStatementImports, importStatement, previewStatement, type LedgerAccount, type StatementDetail, type StatementImport, type StatementPreview } from "@/lib/ledger-api";
import { formatMoney } from "@/lib/money";
import { hasPermission, PERMISSIONS } from "@/lib/rbac";
import { Icon } from "../icons";
import { MatchingReview } from "./matching-review";
import "./reconciliation.css";

const schema = yup.object({
  accountId: yup.string().uuid().required("Choose an account."),
  file: yup.mixed<File>().required("Choose a CSV statement.")
    .test("csv", "Choose a .csv file.", (file) => file.name.toLowerCase().endsWith(".csv"))
    .test("size", "The CSV must be nonempty and at most 2 MiB.", (file) => file.size > 0 && file.size <= 2 * 1024 * 1024),
});
type ImportForm = yup.InferType<typeof schema>;
const headers = ["Date", "Reference", "Currency", "Amount", "Direction", "Description"];

export function Reconciliation() {
  const { session } = useAuthSession();
  return <StatementWorkspace key={session.token} token={session.token} permissions={session.permissions} />;
}

function StatementWorkspace({ token, permissions }: { token: string; permissions: string[] | null }) {
  const [accounts, setAccounts] = useState<LedgerAccount[]>([]);
  const [history, setHistory] = useState<StatementImport[]>([]);
  const [accountId, setAccountId] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [preview, setPreview] = useState<StatementPreview | null>(null);
  const [detail, setDetail] = useState<StatementDetail | null>(null);
  const [offset, setOffset] = useState(0);
  const [historyOffset, setHistoryOffset] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const ledgerRead = hasPermission(permissions, PERMISSIONS.ledgerRead);
  const canImport = [PERMISSIONS.reconciliationImport, PERMISSIONS.attachmentsRead, PERMISSIONS.attachmentsWrite, PERMISSIONS.ledgerRead].every((p) => hasPermission(permissions, p));

  function report(err: unknown) {
    setError(err instanceof Error ? err.message : "Could not complete this request.");
    if (err instanceof AuthApiError && err.code === "invalid_credentials") void validateSession();
  }

  useEffect(() => {
    let active = true;
    if (token && ledgerRead) {
      void fetchStatementImports(token, historyOffset).then((rows) => { if (active) setHistory(rows); }).catch((err: unknown) => { if (active) report(err); });
    }
    return () => { active = false; };
  }, [token, ledgerRead, historyOffset]);

  useEffect(() => {
    let active = true;
    async function load() {
      const found: LedgerAccount[] = [];
      for (let start = 0; ; start += 100) {
        const page = await fetchAccounts(token, { limit: 100, offset: start });
        if (!active) return;
        found.push(...page.items.filter((a) => a.type === "ASSET" || a.type === "LIABILITY"));
        if (start + page.items.length >= page.total || page.items.length === 0) break;
      }
      if (active) setAccounts(found);
    }
    if (token && ledgerRead) void load().catch((err: unknown) => { if (active) report(err); });
    return () => { active = false; };
  }, [token, ledgerRead]);

  function accountLabel(id: string) { const account = accounts.find((a) => a.id === id); return account ? `${account.account_number} · ${account.name}` : id; }

  function clearPreview() { setPreview(null); setDetail(null); setOffset(0); setNotice(""); setError(""); }
  async function upload() {
    setBusy(true); clearPreview();
    try {
      const input: ImportForm = await schema.validate({ accountId, file });
      const attachment = await uploadAttachment(token, input.file);
      setPreview(await previewStatement(token, { attachment_id: attachment.id, account_id: input.accountId }));
    } catch (err) { report(err); } finally { setBusy(false); }
  }
  async function open(id: string, start = 0) {
    setBusy(true); setError("");
    try { setDetail(await fetchStatementDetail(token, id, start)); setOffset(start); } catch (err) { report(err); } finally { setBusy(false); }
  }
  async function save() {
    if (!preview?.can_import || !preview.content_hash) return;
    setBusy(true); setError("");
    try {
      const saved = await importStatement(token, { attachment_id: preview.attachment_id, account_id: preview.account_id, expected_hash: preview.content_hash });
      setNotice(`${saved.row_count} rows imported. Ready for future matching; no ledger entries were posted.`);
      setPreview(null); setDetail(await fetchStatementDetail(token, saved.id)); setOffset(0);
      setHistory(await fetchStatementImports(token, historyOffset));
    } catch (err) { report(err); } finally { setBusy(false); }
  }
  const count = detail?.import.row_count ?? preview?.rows.length ?? 0;
  const currency = detail?.import.currency ?? preview?.currency;
  const totals = detail?.import ?? preview;

  return <div className="statement-workspace">
    <header className="statement-heading"><div><p className="eyebrow">RECONCILIATION · STATEMENT INTAKE</p><h1>Bank statements</h1><p>Upload, validate, and keep statement rows ready for reconciliation.</p></div>
      <a className="button secondary" href="/statement-template.csv" download><Icon name="download" size={16} /> CSV template</a>
    </header>
    {error && <p className="statement-alert error" role="alert">{error}</p>}
    {notice && <p className="statement-alert" role="status">{notice}</p>}
    {!ledgerRead ? <p className="statement-alert">Ledger read permission is also required to view statements.</p> : <>
      {canImport ? <section className="statement-card" aria-labelledby="import-heading">
        <h2 id="import-heading">Import a statement</h2>
        <p>UTF-8 CSV · up to 2 MiB · 2,000 rows. Enter positive amounts in major units (USD 12.50, JPY 1250). DEBIT and CREDIT follow your bank statement.</p>
        <form onSubmit={(event) => { event.preventDefault(); void upload(); }} className="statement-form">
          <label>Ledger account<select value={accountId} disabled={busy} onChange={(event) => { setAccountId(event.target.value); clearPreview(); }} required><option value="">Choose an asset or liability account</option>{accounts.map((a) => <option key={a.id} value={a.id}>{a.account_number} · {a.name} · {a.currency}{a.is_active ? "" : " (inactive)"}</option>)}</select></label>
          <label>Statement file<input type="file" accept=".csv,text/csv" disabled={busy} onChange={(event) => { setFile(event.target.files?.[0] ?? null); clearPreview(); }} required /></label>
          <button type="submit" className="button" disabled={busy || !file || !accountId}><Icon name="file" size={16} /> {busy ? "Processing…" : "Upload & preview"}</button>
        </form>
      </section> : <p className="statement-alert">Your role can review your imported statements. Importing requires reconciliation import and attachment upload/read permissions.</p>}

      {(preview || detail) && <section className="statement-card" aria-labelledby="preview-heading">
        <div className="statement-section-heading"><div><h2 id="preview-heading">{detail ? "Imported statement" : "Validation preview"}</h2><p>{detail?.import.file_name ?? preview?.file_name} · {accountLabel(detail?.import.account_id ?? preview?.account_id ?? "")} · {count} rows{preview ? ` · ${preview.invalid_rows} invalid` : ""}</p></div>
          {preview && !detail && canImport && <button className="button" type="button" disabled={busy || !preview.can_import} onClick={() => void save()}><Icon name="save" size={16} /> Import validated rows</button>}
        </div>
        {totals && currency && <div className="statement-totals"><span>Valid debits <strong>{formatMoney(totals.total_debits, currency)}</strong></span><span>Valid credits <strong>{formatMoney(totals.total_credits, currency)}</strong></span></div>}
        {preview?.duplicate_import_id && !detail && <p className="statement-alert">Already imported for this account. <button type="button" className="button secondary" disabled={busy} onClick={() => { const id = preview.duplicate_import_id; if (id) void open(id); }}><Icon name="book" size={16} /> Open existing import</button></p>}
        <div className="statement-table-wrap"><table><caption className="sr-only">Statement rows and validation results</caption><thead><tr><th>Row</th>{headers.map((h) => <th key={h}>{h}</th>)}<th>Validation</th></tr></thead><tbody>
          {detail ? detail.rows.map((e, i) => <tr key={offset + i}><td>{offset + i + 1}</td><td>{e.booked_date}</td><td>{e.reference}</td><td>{e.currency}</td><td>{formatMoney(e.amount, e.currency)}</td><td>{e.direction}</td><td>{e.description || "—"}</td><td>Imported</td></tr>) : preview?.rows.slice(offset, offset + 50).map((row) => <tr key={row.row_number} className={row.errors.length ? "statement-invalid" : undefined}><td>{row.row_number}</td>{row.values.map((v, i) => <td key={i}>{v || "—"}</td>)}<td>{row.errors.length ? row.errors.join("; ") : "Valid"}</td></tr>)}
        </tbody></table></div>
        <div className="statement-pager"><button type="button" className="button secondary" disabled={busy || offset === 0} onClick={() => { if (detail) void open(detail.import.id, Math.max(0, offset - 50)); else setOffset(Math.max(0, offset - 50)); }}><Icon name="arrow" size={16} /> Previous rows</button><span>{offset + 1}–{Math.min(offset + 50, count)} of {count}</span><button type="button" className="button secondary" disabled={busy || offset + 50 >= count} onClick={() => { if (detail) void open(detail.import.id, offset + 50); else setOffset(offset + 50); }}><Icon name="arrow" size={16} /> Next rows</button></div>
        <p className="statement-footnote">This import stores statement data only. Use Match & review below to reconcile existing ledger postings.</p>
      </section>}

      {detail && <MatchingReview key={detail.import.id} token={token} importId={detail.import.id} canReview={hasPermission(permissions, PERMISSIONS.reconciliationMatch)} />}
      <section className="statement-card"><h2>Your imported statements</h2><p>Imports are private to the uploader. Renaming or reordering the same rows does not create a new import for the same account.</p>
        {history.length === 0 ? <p>No statements imported yet.</p> : <div className="statement-history">{history.map((s) => <button type="button" key={s.id} disabled={busy} onClick={() => void open(s.id)}><Icon name="file" /><span><strong>{s.file_name}</strong><small>{accountLabel(s.account_id)} · {s.currency} · {s.row_count} rows · {new Date(s.created_at).toLocaleString()}</small></span><Icon name="arrow" /></button>)}</div>}
        <div className="statement-pager"><button type="button" className="button secondary" disabled={busy || historyOffset === 0} onClick={() => setHistoryOffset(Math.max(0, historyOffset - 20))}><Icon name="arrow" size={16} /> Previous imports</button><button type="button" className="button secondary" disabled={busy || history.length < 20} onClick={() => setHistoryOffset(historyOffset + 20)}><Icon name="arrow" size={16} /> More imports</button></div>
      </section>
    </>}
  </div>;
}
