"use client";

import { useWorkspaceCurrency } from "@/lib/workspace-currency";
import { useEffect, useId, useRef, useState, type FormEvent } from "react";
import { boolean, mixed, object, string, ValidationError } from "yup";
import { AuthApiError } from "@/lib/auth-api";
import { useAuthSession } from "@/lib/auth-session";
import {
  ACCOUNT_TYPES, CURRENCIES, SYSTEM_ROLES, SYSTEM_ROLE_LABEL, SYSTEM_ROLE_TYPE, createAccount,
  type AccountType, type Currency, type SystemRole,
} from "@/lib/ledger-api";
import { notify } from "@/lib/notify";
import { Icon } from "../../icons";
import "../../admin/admin.css";

// Mirrors model.CreateAccountRequest.Normalize in the API.
const schema = object({
  account_number: string().trim().required("Enter an account number.").matches(/^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/, "Use 1–64 letters, digits, '.', '_' or '-'."),
  name: string().trim().required("Enter a name.").max(255, "At most 255 characters."),
  type: mixed<AccountType>().oneOf([...ACCOUNT_TYPES]).required(),
  currency: mixed<Currency>().oneOf([...CURRENCIES]).required(),
  overdraft_allowed: boolean().required(),
  system_role: mixed<SystemRole>().oneOf([...SYSTEM_ROLES]).nullable().defined()
    .test("role-type", "This role needs a different account type.", (role, ctx) => {
      const parent: unknown = ctx.parent;
      const accountType = typeof parent === "object" && parent !== null && "type" in parent ? parent.type : undefined;
      return role == null || SYSTEM_ROLE_TYPE[role] === accountType;
    }),
});

const ROLE_HELP: Record<SystemRole, string> = {
  PAYOUT_CLEARING: "Holds reserved payout funds until they are captured or released.",
  OPERATING_BANK: "Receives captured payouts.",
};

const TYPE_HELP: Record<AccountType, string> = {
  ASSET: "What the business holds (bank, cash). Debits increase it.",
  LIABILITY: "What the business owes (customer wallets, clearing). Credits increase it.",
  EQUITY: "Owners' capital. Credits increase it.",
  REVENUE: "Income such as fees. Credits increase it.",
  EXPENSE: "Costs. Debits increase it.",
};

export function NewAccountDialog({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const { session } = useAuthSession();
  const dialog = useRef<HTMLDialogElement>(null);
  const id = useId();
  const [accountNumber, setAccountNumber] = useState("");
  const [name, setName] = useState("");
  const [type, setType] = useState<AccountType>("LIABILITY");
  const { currency: workspaceCurrency } = useWorkspaceCurrency();
  const [currency, setCurrency] = useState<Currency>(workspaceCurrency);
  const [overdraft, setOverdraft] = useState(false);
  const [systemRole, setSystemRole] = useState<SystemRole | null>(null);
  const rolesForType = SYSTEM_ROLES.filter((r) => SYSTEM_ROLE_TYPE[r] === type);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    const el = dialog.current;
    if (el && !el.open) el.showModal();
  }, []);

  async function submit(event: FormEvent) {
    event.preventDefault();
    const values = { account_number: accountNumber, name, type, currency, overdraft_allowed: overdraft, system_role: systemRole };
    try {
      schema.validateSync(values);
    } catch (e) {
      setError(e instanceof ValidationError ? e.message : "Check the form.");
      return;
    }
    setSaving(true);
    setError("");
    try {
      const account = await createAccount(session.token, { ...values, account_number: accountNumber.trim(), name: name.trim() });
      notify.success(`Account ${account.account_number} · ${account.name} was created.`);
      onCreated();
    } catch (e) {
      setError(e instanceof AuthApiError ? e.message : "Could not create the account. Try again.");
      setSaving(false);
    }
  }

  return (
    <dialog ref={dialog} className="adm-dialog adm-dialog-wide" aria-labelledby={`${id}-title`} aria-busy={saving}
      onCancel={(e) => { e.preventDefault(); if (!saving) onClose(); }}>
      <form noValidate onSubmit={(e) => void submit(e)} className="adm-dialog-form">
        <header className="adm-dialog-head">
          <div>
            <p className="eyebrow">CHART OF ACCOUNTS</p>
            <h2 id={`${id}-title`}>New account</h2>
            <p className="adm-dialog-sub">Balances start at zero and change only through posted transfers.</p>
          </div>
          <button type="button" className="adm-icon-btn" aria-label="Close" disabled={saving} onClick={onClose}><Icon name="close" size={18} /></button>
        </header>
        <fieldset className="adm-form-body" disabled={saving}>
          <div className="adm-form-grid">
            <div className="adm-field">
              <label htmlFor={`${id}-num`}>Account number <em aria-hidden="true">*</em></label>
              <input id={`${id}-num`} value={accountNumber} maxLength={64} onChange={(e) => setAccountNumber(e.target.value)} placeholder="e.g. 2004" autoFocus />
            </div>
            <div className="adm-field">
              <label htmlFor={`${id}-name`}>Name <em aria-hidden="true">*</em></label>
              <input id={`${id}-name`} value={name} maxLength={255} onChange={(e) => setName(e.target.value)} placeholder="e.g. Kenji · Wallet" />
            </div>
          </div>
          <div className="adm-form-grid">
            <div className="adm-field">
              <label htmlFor={`${id}-type`}>Type</label>
              <select id={`${id}-type`} value={type} onChange={(e) => {
                const next = ACCOUNT_TYPES.find((t) => t === e.target.value) ?? "LIABILITY";
                setType(next);
                // A role is tied to one account type; drop it when the type no longer fits.
                if (systemRole !== null && SYSTEM_ROLE_TYPE[systemRole] !== next) setSystemRole(null);
              }}>
                {ACCOUNT_TYPES.map((t) => <option key={t} value={t}>{t}</option>)}
              </select>
              <p className="adm-field-hint">{TYPE_HELP[type]}</p>
            </div>
            <div className="adm-field">
              <label htmlFor={`${id}-cur`}>Currency</label>
              <select id={`${id}-cur`} value={currency} onChange={(e) => setCurrency(CURRENCIES.find((c) => c === e.target.value) ?? "JPY")}>
                {CURRENCIES.map((c) => <option key={c} value={c}>{c}</option>)}
              </select>
              <p className="adm-field-hint">Cannot be changed later; transfers use one currency.</p>
            </div>
          </div>
          {rolesForType.length > 0 && (
            <div className="adm-field">
              <label htmlFor={`${id}-role`}>System role</label>
              <select id={`${id}-role`} value={systemRole ?? ""} onChange={(e) => setSystemRole(rolesForType.find((r) => r === e.target.value) ?? null)}>
                <option value="">None (ordinary account)</option>
                {rolesForType.map((r) => <option key={r} value={r}>{SYSTEM_ROLE_LABEL[r]}</option>)}
              </select>
              <p className="adm-field-hint">{systemRole ? ROLE_HELP[systemRole] : "Payouts use the accounts holding these roles; one per currency."}</p>
            </div>
          )}
          <label className="adm-check">
            <input type="checkbox" checked={overdraft} onChange={(e) => setOverdraft(e.target.checked)} />
            <span>Allow overdraft <small>— the balance may go below zero (e.g. a funding or clearing account). Leave off for customer wallets.</small></span>
          </label>
        </fieldset>
        {error && <p className="error-message" role="alert">{error}</p>}
        <footer className="adm-dialog-foot">
          <button type="button" className="button secondary" disabled={saving} onClick={onClose}>Cancel</button>
          <button type="submit" className="button primary" disabled={saving}>
            {saving ? <span className="progress-spinner adm-btn-spinner" aria-hidden="true" /> : <Icon name="plus" size={15} />}
            {saving ? "Creating…" : "Create account"}
          </button>
        </footer>
      </form>
    </dialog>
  );
}
