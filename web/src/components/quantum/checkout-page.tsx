"use client";

import { useCallback, useEffect, useState } from "react";
import { fetchCheckout, fetchQuantumConfig, type Checkout, type CheckoutStatus, type QuantumConfig } from "@/lib/quantum-api";
import { formatTokenAmount } from "@/lib/quantum-amount";
import { invoiceStatusLabel, invoiceStatusTone } from "@/lib/quantum-labels";
import { Badge } from "../ui";
import { TestnetBanner } from "./quantum-banner";
import { CheckoutFlow } from "./wallet/checkout-flow";
import { QuantumWalletProvider } from "./wallet/provider";
import "./quantum.css";

type Props = { token: string };

export function CheckoutPage({ token }: Props) {
  const [config, setConfig] = useState<QuantumConfig | null>(null);
  const [checkout, setCheckout] = useState<Checkout | null>(null);
  const [checkoutStatus, setCheckoutStatus] = useState<CheckoutStatus | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const handleStatusChange = useCallback((next: CheckoutStatus) => setCheckoutStatus(next), []);

  useEffect(() => {
    let cancelled = false;
    Promise.all([fetchQuantumConfig(), fetchCheckout(token)]).then(
      ([nextConfig, nextCheckout]) => {
        if (cancelled) return;
        setConfig(nextConfig);
        setCheckout(nextCheckout);
        setLoading(false);
      },
      (reason: unknown) => {
        if (cancelled) return;
        setError(reason instanceof Error ? reason.message : "Could not load this checkout.");
        setLoading(false);
      },
    );
    return () => { cancelled = true; };
  }, [token]);

  return (
    <main className="quantum-checkout" id="main-content">
      <TestnetBanner />
      <div className="quantum-checkout-card">
        <p className="quantum-eyebrow">PAYCORE QUANTUM · TESTNET INVOICE</p>
        <h1>{checkout?.merchant_name ?? "Payment checkout"}</h1>
        {loading && <p role="status">Loading invoice…</p>}
        {error && <p className="error-message" role="alert">{error}</p>}
        {checkout && <>
          <p className="quantum-checkout-description">{checkout.description}</p>
          <p className="quantum-checkout-amount">{formatTokenAmount(checkout.amount_base_units, checkout.token_decimals)} <small>{checkout.token_symbol}</small></p>
          <div className="quantum-checkout-meta">
            <span><strong>Status</strong><Badge tone={invoiceStatusTone(checkoutStatus?.invoice_status ?? checkout.status)}>{invoiceStatusLabel(checkoutStatus?.invoice_status ?? checkout.status)}</Badge></span>
            <span><strong>Network</strong> Testnet {checkout.chain_id}</span>
            <span><strong>Expires</strong> {new Date(checkout.expires_at).toLocaleString()}</span>
            <span className="quantum-break"><strong>Receiver</strong> <span className="mono">{checkout.receiver_address}</span></span>
            {checkout.payment_contract_address && <span className="quantum-break"><strong>Payment contract</strong> <a className="text-link mono" href={`${checkout.explorer_url.replace(/\/+$/, "")}/address/${encodeURIComponent(checkout.payment_contract_address)}`} target="_blank" rel="noreferrer">{checkout.payment_contract_address}</a></span>}
          </div>
          {config?.enabled ? <QuantumWalletProvider key={checkout.chain_id} config={{ ...config, chain_id: checkout.chain_id }}>
            <CheckoutFlow token={token} checkout={checkout} minConfirmations={config.min_confirmations} onStatusChange={handleStatusChange} />
          </QuantumWalletProvider> : <p className="error-message" role="alert">Quantum payments are unavailable on this server. Contact the merchant.</p>}
        </>}
      </div>
      <p className="quantum-checkout-footnote">Noncustodial testnet payment. PayCore never holds wallet keys or real funds.</p>
    </main>
  );
}
