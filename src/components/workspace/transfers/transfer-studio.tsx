"use client";

import { useState, type FormEvent } from "react";
import { accounts, balanceOf, money, validAmount } from "@/lib/demo-data";
import { Icon } from "../../icons";
import { Notice, Panel } from "../../ui";
import { RequestPreview } from "./request-preview";

const WALLETS = accounts.filter((a) => ["2001", "2002"].includes(a.id));
const available = (id: string) => balanceOf(id);

function WalletSelect({ label, value, onChange }: { label: string; value: string; onChange: (v: string) => void }) {
  return (
    <label>
      {label}
      <select value={value} onChange={(e) => onChange(e.target.value)}>
        {WALLETS.map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}
      </select>
    </label>
  );
}

function IdempotencyKey({ value, onReset }: { value: string; onReset: () => void }) {
  return (
    <div className="field-group">
      <span>Idempotency-Key header</span>
      <div className="key-field">
        <code>{value || "Generated on preview"}</code>
        <button className="text-link" type="button" onClick={onReset}><Icon name="key" size={14} />New key</button>
      </div>
    </div>
  );
}

export function TransferStudio() {
  const [from, setFrom] = useState("2001");
  const [to, setTo] = useState("2002");
  const [amount, setAmount] = useState("50000");
  const [description, setDescription] = useState("Peer-to-peer wallet transfer");
  const [preview, setPreview] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [requestKey, setRequestKey] = useState("");
  const [keyPayload, setKeyPayload] = useState("");

  function submit(e: FormEvent) {
    e.preventDefault();
    setError("");
    setPreview(null);
    if (!validAmount(amount) || from === to) return setError("Choose different accounts and a positive whole-yen amount within int64 limits.");
    if (BigInt(amount) > available(from)) return setError("Amount exceeds the sample wallet balance.");
    const payload = JSON.stringify({
      currency: "JPY",
      description,
      legs: [{ account_id: from, direction: "DEBIT", amount }, { account_id: to, direction: "CREDIT", amount }],
    });
    if (requestKey && keyPayload !== payload) return setError("Request changed. Choose New key before previewing a different transfer.");
    setRequestKey(requestKey || crypto.randomUUID());
    setKeyPayload(payload);
    setPreview(JSON.stringify(JSON.parse(payload), null, 2));
  }

  const resetKey = () => { setRequestKey(""); setKeyPayload(""); setPreview(null); };

  return (
    <div className="form-grid">
      <Panel title="Build a transfer" description="Preview only. No request is sent and no balance changes.">
        <form className="form-body" onSubmit={submit}>
          <div className="field-pair">
            <WalletSelect label="From wallet" value={from} onChange={setFrom} />
            <WalletSelect label="To wallet" value={to} onChange={setTo} />
          </div>
          <label>
            Amount · JPY
            <input inputMode="numeric" value={amount} onChange={(e) => setAmount(e.target.value)} required />
            <small>Whole yen. Available: {money(available(from))}.</small>
          </label>
          <label>
            Description
            <input value={description} onChange={(e) => setDescription(e.target.value)} maxLength={200} required />
          </label>
          <Notice icon="shield">{validAmount(amount) ? `Debit ${money(amount)} = Credit ${money(amount)}` : "Enter a positive whole-yen amount."}</Notice>
          <IdempotencyKey value={requestKey} onReset={resetKey} />
          {error && <p role="alert" className="error-message">{error}</p>}
          <button type="submit" className="button primary">Preview request <Icon name="arrow" size={17} /></button>
        </form>
      </Panel>
      <RequestPreview preview={preview} />
    </div>
  );
}
