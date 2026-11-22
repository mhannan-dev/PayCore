"use client";

import { useEffect, useId, useRef, useState, type FormEvent } from "react";
import { ValidationError } from "yup";
import { AuthApiError } from "@/lib/auth-api";
import { useAuthSession } from "@/lib/auth-session";
import { createInvoice, type Invoice } from "@/lib/quantum-api";
import { tokenAmountHint } from "@/lib/quantum-amount";
import { notify } from "@/lib/notify";
import { Icon } from "../icons";
import { Notice } from "../ui";
import { buildInvoiceRequest, type InvoiceValues } from "./invoice-schema";

type Props = { decimals: number; tokenSymbol: string; onCreated: (invoice: Invoice) => void; onClose: () => void };

export function InvoiceForm({ decimals, tokenSymbol, onCreated, onClose }: Props) {
  const { session } = useAuthSession();
  const id = useId();
  const dialog = useRef<HTMLDialogElement>(null);
  const amountInput = useRef<HTMLInputElement>(null);
  const [values, setValues] = useState<InvoiceValues>({ amount: "", description: "", expires_in_minutes: 30 });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const submitting = useRef(false);
  const draft = useRef<{ payload: string; key: string } | null>(null);

  useEffect(() => {
    const element = dialog.current;
    if (element && !element.open) {
      element.showModal();
      amountInput.current?.focus();
    }
    return () => { if (element?.open) element.close(); };
  }, []);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (submitting.current) return;
    setError("");
    try {
      const input = buildInvoiceRequest(values, decimals);
      const payload = JSON.stringify(input);
      const current = draft.current?.payload === payload ? draft.current : { payload, key: crypto.randomUUID() };
      draft.current = current;
      submitting.current = true;
      setSaving(true);
      const invoice = await createInvoice(session.token, current.key, input);
      notify.success(`${invoice.id}: invoice ready to share.`);
      onCreated(invoice);
    } catch (reason) {
      setError(reason instanceof ValidationError || reason instanceof AuthApiError || reason instanceof Error ? reason.message : "Could not create the invoice.");
    } finally {
      submitting.current = false;
      setSaving(false);
    }
  }

  function field<K extends keyof InvoiceValues>(name: K, value: InvoiceValues[K]) {
    setValues((current) => ({ ...current, [name]: value }));
    setError("");
  }

  return (
    <dialog ref={dialog} className="quantum-invoice-dialog" aria-labelledby={`${id}-title`} aria-describedby={`${id}-description`} aria-busy={saving}
      onCancel={(event) => { event.preventDefault(); if (!submitting.current) onClose(); }}>
      <header className="quantum-dialog-heading">
        <div><h2 id={`${id}-title`}>New payment invoice</h2><p id={`${id}-description`}>Create a checkout link for an exact {tokenSymbol} payment.</p></div>
        <button type="button" className="button secondary" aria-label="Close invoice form" disabled={saving} onClick={onClose}><Icon name="close" size={16} /></button>
      </header>
      <form className="form-body" onSubmit={(event) => void submit(event)} aria-busy={saving}>
        <fieldset className="plain-fieldset" disabled={saving}>
          <div className="field-pair">
            <label>Amount · {tokenSymbol}
              <input ref={amountInput} value={values.amount} inputMode="decimal" placeholder="12.5" onChange={(event) => field("amount", event.target.value)} required />
              <span className="quantum-field-hint">{tokenAmountHint(decimals)}</span>
            </label>
            <label>Expires in (minutes)
              <input type="number" min={5} max={1440} step={1} value={values.expires_in_minutes} onChange={(event) => field("expires_in_minutes", event.target.valueAsNumber)} required />
            </label>
          </div>
          <label>Description<input value={values.description} placeholder="What is this payment for?" onChange={(event) => field("description", event.target.value)} required /></label>
          <Notice icon="shield">If a request times out, retry the same form to recover your invoice without creating a duplicate.</Notice>
          {error && <p className="error-message" role="alert">{error}</p>}
          <div className="settlement-actions">
            <button className="button secondary" type="button" onClick={onClose}>Cancel</button>
            <button className="button primary" type="submit" disabled={saving}>{saving ? "Creating…" : "Create invoice"}</button>
          </div>
        </fieldset>
      </form>
    </dialog>
  );
}
