"use client";

import { useWorkspaceCurrency } from "@/lib/workspace-currency";
import { useState, type FormEvent } from "react";
import { AuthApiError } from "@/lib/auth-api";
import { useAuthSession } from "@/lib/auth-session";
import { CURRENCIES, fetchAccounts, postTransfer, type Journal, type LedgerAccount, type TransferInput } from "@/lib/ledger-api";
import { formatMoney, parseMoneyInput } from "@/lib/money";
import { notify } from "@/lib/notify";
import { PERMISSIONS } from "@/lib/rbac";
import { usePermission } from "../../can";
import { Icon } from "../../icons";
import { Notice, Panel } from "../../ui";
import { ApiStatus } from "../shared/api-status";
import { useApi } from "../shared/use-api";
import { TransferResult } from "./request-preview";

const newReference = () => `TXN-${new Date().toISOString().slice(0, 10).replace(/-/g, "")}-${crypto.randomUUID().slice(0, 8).toUpperCase()}`;

function AccountSelect({ label, value, accounts, onChange }: { label: string; value: string; accounts: LedgerAccount[]; onChange: (v: string) => void }) {
  return (
    <label>
      {label}
      <select value={value} onChange={(e) => onChange(e.target.value)} required>
        <option value="">Choose an account…</option>
        {accounts.map((a) => (
          <option key={a.id} value={a.id}>{a.account_number} · {a.name} ({formatMoney(a.balance, a.currency)})</option>
        ))}
      </select>
    </label>
  );
}

/** A draft keeps its Idempotency-Key until it is posted or edited, so retrying the
 *  same draft after a timeout can never post it twice. */
type Draft = { payload: string; key: string };

export function TransferStudio() {
  const { session } = useAuthSession();
  const can = usePermission();
  const allowed = can(PERMISSIONS.transfersCreate);
  const { currency, setCurrency } = useWorkspaceCurrency();
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [amount, setAmount] = useState("");
  const [description, setDescription] = useState("Peer-to-peer wallet transfer");
  const [reference, setReference] = useState(newReference);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [error, setError] = useState("");
  const [sending, setSending] = useState(false);
  const [result, setResult] = useState<Journal | null>(null);
  const [version, setVersion] = useState(0);

  const accounts = useApi(JSON.stringify([currency, version]), (token) => fetchAccounts(token, { currency, limit: 200, offset: 0 }));
  const active = (accounts.data?.items ?? []).filter((a) => a.is_active);
  const fromAccount = active.find((a) => a.id === from);
  const parsed = parseMoneyInput(amount, currency);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setError("");
    if (!from || !to) return setError("Choose the account to debit and the account to credit.");
    if (from === to) return setError("Choose two different accounts.");
    if ("error" in parsed) return setError(parsed.error);
    const input: TransferInput = {
      reference_id: reference.trim(),
      description: description.trim(),
      currency,
      legs: [
        { account_id: from, direction: "DEBIT", amount: parsed.minor },
        { account_id: to, direction: "CREDIT", amount: parsed.minor },
      ],
    };
    const payload = JSON.stringify(input);
    const current = draft?.payload === payload ? draft : { payload, key: crypto.randomUUID() };
    setDraft(current);
    setSending(true);
    try {
      const journal = await postTransfer(session.token, current.key, input);
      setResult(journal);
      notify.success(`Transfer ${journal.reference_id} posted: ${formatMoney(journal.total, journal.currency)}.`);
      // Next transfer: fresh reference and key; refresh balances.
      setDraft(null);
      setReference(newReference());
      setAmount("");
      setVersion((v) => v + 1);
    } catch (err) {
      setError(err instanceof AuthApiError ? err.message : "Could not post the transfer. Try again: retrying is safe.");
    } finally {
      setSending(false);
    }
  }

  return (
    <div className="form-grid">
      <Panel title="New transfer" description="Moves money between two ledger accounts as one balanced journal.">
        {!allowed && <Notice icon="lock">You can view transfers but not post them: posting needs the <code>transfers:create</code> permission.</Notice>}
        <ApiStatus loading={accounts.loading && !accounts.data} error={accounts.error} forbidden={accounts.forbidden} onRetry={accounts.reload} />
        <form className="form-body" onSubmit={(e) => void submit(e)}>
          <fieldset disabled={!allowed || sending} className="plain-fieldset">
            <label>
              Currency
              <select value={currency} onChange={(e) => { setCurrency(CURRENCIES.find((c) => c === e.target.value) ?? "JPY"); setFrom(""); setTo(""); }}>
                {CURRENCIES.map((c) => <option key={c} value={c}>{c}</option>)}
              </select>
            </label>
            <div className="field-pair">
              <AccountSelect label="Debit (from)" value={from} accounts={active} onChange={setFrom} />
              <AccountSelect label="Credit (to)" value={to} accounts={active} onChange={setTo} />
            </div>
            <label>
              Amount · {currency}
              <input inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} placeholder={currency === "JPY" ? "50000" : "125.00"} required />
              <small>{fromAccount ? `Balance of ${fromAccount.account_number}: ${formatMoney(fromAccount.balance, currency)}.` : "Choose the debit account to see its balance."}</small>
            </label>
            <div className="field-pair">
              <label>
                Reference
                <input value={reference} onChange={(e) => setReference(e.target.value)} maxLength={128} required />
              </label>
              <label>
                Description
                <input value={description} onChange={(e) => setDescription(e.target.value)} maxLength={500} required />
              </label>
            </div>
            <Notice icon="shield">
              {"minor" in parsed ? `Debit ${formatMoney(parsed.minor, currency)} = Credit ${formatMoney(parsed.minor, currency)}` : "Enter an amount to see both sides."}
            </Notice>
            {draft && <p className="muted">Idempotency-Key <code>{draft.key}</code>: retrying this exact transfer is safe.</p>}
            {error && <p role="alert" className="error-message">{error}</p>}
            <button type="submit" className="button primary">
              {sending ? <><span className="spinner" /> Posting…</> : <>Post transfer <Icon name="arrow" size={17} /></>}
            </button>
          </fieldset>
        </form>
      </Panel>
      <TransferResult journal={result} />
    </div>
  );
}
