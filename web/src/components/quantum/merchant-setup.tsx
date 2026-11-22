"use client";

import { useRef, useState, type FormEvent } from "react";
import { ValidationError } from "yup";
import { AuthApiError } from "@/lib/auth-api";
import { createMerchant, type Merchant } from "@/lib/quantum-api";
import { useAuthSession } from "@/lib/auth-session";
import { notify } from "@/lib/notify";
import { Panel } from "../ui";
import { buildMerchantRequest, type MerchantValues } from "./merchant-schema";

type Props = { onCreated: (merchant: Merchant) => void };

export function MerchantSetup({ onCreated }: Props) {
  const { session } = useAuthSession();
  const [values, setValues] = useState<MerchantValues>({ display_name: "", receiver_address: "" });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const submitting = useRef(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (submitting.current) return;
    submitting.current = true;
    setError("");
    setSaving(true);
    try {
      const merchant = await createMerchant(session.token, buildMerchantRequest(values));
      notify.success(`${merchant.display_name} is ready to accept testnet payments.`);
      onCreated(merchant);
    } catch (reason) {
      setError(reason instanceof ValidationError || reason instanceof AuthApiError || reason instanceof Error ? reason.message : "Could not save the merchant.");
    } finally {
      submitting.current = false;
      setSaving(false);
    }
  }

  function field(name: keyof MerchantValues, value: string) {
    setValues((current) => ({ ...current, [name]: value }));
    setError("");
  }

  return (
    <Panel title="Set up your merchant profile" description="One profile per account. Choose carefully: the receiver address cannot be changed later.">
      <form className="form-body" onSubmit={(event) => void submit(event)} aria-busy={saving}>
        <fieldset className="plain-fieldset" disabled={saving}>
          <label>Display name<input value={values.display_name} autoComplete="organization" onChange={(event) => field("display_name", event.target.value)} required /></label>
          <label>Receiver address
            <input className="mono" value={values.receiver_address} placeholder="0x…" spellCheck={false} autoComplete="off" onChange={(event) => field("receiver_address", event.target.value)} required />
            <span className="quantum-field-hint">Lower-case or mixed-case 0x address on the configured test network. No wallet key is needed.</span>
          </label>
          {error && <p className="error-message" role="alert">{error}</p>}
          <div className="settlement-actions">
            <button className="button primary" type="submit" disabled={saving}>{saving ? "Saving…" : "Save merchant"}</button>
          </div>
        </fieldset>
      </form>
    </Panel>
  );
}
