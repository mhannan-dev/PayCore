"use client";

import { useRef, useState, type FormEvent } from "react";
import { ValidationError } from "yup";
import { AuthApiError } from "@/lib/auth-api";
import { useAuthSession } from "@/lib/auth-session";
import { createPayout, type Currency, type LedgerAccount, type Settlement } from "@/lib/ledger-api";
import { formatMoney } from "@/lib/money";
import { notify } from "@/lib/notify";
import { Notice, Panel } from "../../ui";
import { buildPayoutRequest, eligiblePayoutSource, type PayoutValues } from "./payout-form-schema";

type Props = { currency: Currency; accounts: LedgerAccount[]; onCreated: (payout: Settlement) => void; onClose: () => void };

export function PayoutForm({ currency, accounts, onCreated, onClose }: Props) {
  const { session } = useAuthSession();
  const [values, setValues] = useState<PayoutValues>({ source_account_id: "", amount: "", beneficiary_name: "", beneficiary_bank: "", beneficiary_account: "" });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [retryAvailable, setRetryAvailable] = useState(false);
  const submitting = useRef(false);
  const draft = useRef<{ payload: string; key: string } | null>(null);
  const wallets = accounts.filter((account) => eligiblePayoutSource(account, currency));

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (submitting.current) return;
    setError("");
    try {
      const input = buildPayoutRequest(values, currency);
      if (!wallets.some((account) => account.id === input.source_account_id)) throw new Error("Choose an available source wallet.");
      const payload = JSON.stringify(input);
      const current = draft.current?.payload === payload ? draft.current : { payload, key: crypto.randomUUID() };
      draft.current = current;
      submitting.current = true;
      setSaving(true);
      const payout = await createPayout(session.token, current.key, input);
      notify.success(`${payout.payout_reference}: funds reserved.`);
      onCreated(payout);
    } catch (reason) {
      setRetryAvailable(draft.current !== null);
      setError(reason instanceof ValidationError || reason instanceof AuthApiError || reason instanceof Error ? reason.message : "Could not reserve the payout.");
    } finally {
      submitting.current = false;
      setSaving(false);
    }
  }
  function field(name: keyof PayoutValues, value: string) {
    setValues((current) => ({ ...current, [name]: value }));
    setError("");
  }

  return (
    <Panel title={`New ${currency} payout`} description="Reserve customer wallet funds in payout clearing. Capture completes the sandbox payout; release returns the funds.">
      <form className="form-body" onSubmit={(event) => void submit(event)} aria-busy={saving}>
        <fieldset className="plain-fieldset" disabled={saving}>
          <label>Source wallet
            <select value={values.source_account_id} onChange={(event) => field("source_account_id", event.target.value)} required>
              <option value="">Choose a customer wallet…</option>
              {wallets.map((account) => <option key={account.id} value={account.id}>{account.account_number} · {account.name} · {formatMoney(account.balance, account.currency)}</option>)}
            </select>
          </label>
          {wallets.length === 0 && <Notice icon="wallet">No active {currency} customer wallets are available. A clearing liability account is also required to reserve payouts.</Notice>}
          <div className="field-pair">
            <label>Amount · {currency}<input value={values.amount} inputMode={currency === "JPY" ? "numeric" : "decimal"} placeholder={currency === "JPY" ? "25000" : "250.00"} onChange={(event) => field("amount", event.target.value)} required /></label>
            <label>Beneficiary name<input value={values.beneficiary_name} maxLength={120} onChange={(event) => field("beneficiary_name", event.target.value)} required /></label>
          </div>
          <div className="field-pair">
            <label>Beneficiary bank<input value={values.beneficiary_bank} maxLength={120} onChange={(event) => field("beneficiary_bank", event.target.value)} required /></label>
            <label>Beneficiary account<input value={values.beneficiary_account} maxLength={120} onChange={(event) => field("beneficiary_account", event.target.value)} required /></label>
          </div>
          <Notice icon="bank">Sandbox only: this posts real ledger entries but does not send money to an external bank.</Notice>
          {error && <p className="error-message" role="alert">{error}</p>}
          {error && retryAvailable && <p className="muted">Retrying the same payout form uses the same request key, so it cannot reserve funds twice.</p>}
          <div className="settlement-actions">
            <button className="button secondary" type="button" onClick={onClose}>Cancel</button>
            <button className="button primary" type="submit" disabled={wallets.length === 0}>{saving ? "Reserving…" : "Reserve payout"}</button>
          </div>
        </fieldset>
      </form>
    </Panel>
  );
}
