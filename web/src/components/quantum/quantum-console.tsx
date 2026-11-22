"use client";

import { useEffect, useState } from "react";
import { AuthApiError, type Page } from "@/lib/auth-api";
import { useAuthSession } from "@/lib/auth-session";
import { fetchInvoices, fetchMerchant, fetchQuantumConfig, QUANTUM_INVOICE_STATUSES, type Invoice, type InvoiceStatus, type Merchant } from "@/lib/quantum-api";
import { formatTokenAmount } from "@/lib/quantum-amount";
import { invoiceStatusLabel, invoiceStatusTone } from "@/lib/quantum-labels";
import { PERMISSIONS } from "@/lib/rbac";
import { usePermission } from "../can";
import { Icon } from "../icons";
import { Badge, Empty, Heading, Notice, Panel, Pagination } from "../ui";
import { ApiStatus } from "../workspace/shared/api-status";
import { useApi } from "../workspace/shared/use-api";
import { InvoiceDetails } from "./invoice-details";
import { InvoiceForm } from "./invoice-form";
import { MerchantSetup } from "./merchant-setup";
import { TestnetBanner } from "./quantum-banner";
import "./quantum.css";

const PAGE_SIZE = 25;

type MerchantState = "loading" | "ready" | "missing" | "error";
type MerchantResult = { key: string; merchant: Merchant | null; error: string; missing: boolean };

export function QuantumConsole() {
  const { session } = useAuthSession();
  const can = usePermission();
  const canWrite = can(PERMISSIONS.quantumWrite);
  const config = useApi("quantum-config", () => fetchQuantumConfig(), 60000);
  const [merchantResult, setMerchantResult] = useState<MerchantResult | null>(null);
  const [merchantVersion, setMerchantVersion] = useState(0);
  const [page, setPage] = useState(0);
  const [status, setStatus] = useState<InvoiceStatus | "">("");
  const [creating, setCreating] = useState(false);
  const [selected, setSelected] = useState<string | null>(null);
  const [version, setVersion] = useState(0);
  const merchantKey = config.data?.enabled === true && session.token ? `${session.token}:${merchantVersion}` : "";

  useEffect(() => {
    if (!merchantKey) return;
    let cancelled = false;
    fetchMerchant(session.token).then(
      (found) => { if (!cancelled) setMerchantResult({ key: merchantKey, merchant: found, error: "", missing: false }); },
      (reason: unknown) => {
        if (cancelled) return;
        if (reason instanceof AuthApiError && reason.status === 404 && reason.code === "merchant_not_found") {
          setMerchantResult({ key: merchantKey, merchant: null, error: "", missing: true });
          return;
        }
        setMerchantResult({ key: merchantKey, merchant: null, error: reason instanceof Error ? reason.message : "Could not load your merchant profile.", missing: false });
      },
    );
    return () => { cancelled = true; };
  }, [merchantKey, session.token]);

  const fresh = merchantResult?.key === merchantKey;
  const merchant = fresh ? merchantResult?.merchant ?? null : null;
  const merchantError = fresh && !merchantResult?.missing ? merchantResult?.error ?? "" : "";
  const merchantState: MerchantState = !fresh ? "loading"
    : merchantResult?.merchant ? "ready"
      : merchantResult?.missing ? "missing"
        : merchantResult?.error ? "error" : "loading";

  const list = useApi(JSON.stringify(["quantum-invoices", merchant?.id ?? "", status, page, version]), async (token): Promise<Page<Invoice>> => {
    if (!merchant) return { items: [], total: 0, limit: PAGE_SIZE, offset: page * PAGE_SIZE };
    return fetchInvoices(token, { status, limit: PAGE_SIZE, offset: page * PAGE_SIZE });
  }, 10000);

  function refresh() {
    setVersion((value) => value + 1);
  }

  if (config.error && !config.data) {
    return <div className="quantum-page"><TestnetBanner /><Heading eyebrow="QUANTUM PAYMENTS" title="Invoice console" description="Testnet tUSD8 invoices for your merchant profile." />
      <ApiStatus loading={false} error={config.error} onRetry={config.reload} /></div>;
  }
  if (!config.data) {
    return <div className="quantum-page"><TestnetBanner /><Heading eyebrow="QUANTUM PAYMENTS" title="Invoice console" description="Testnet tUSD8 invoices for your merchant profile." />
      <ApiStatus loading error="" onRetry={config.reload} /></div>;
  }
  if (!config.data.enabled) {
    return <div className="quantum-page"><TestnetBanner /><Heading eyebrow="QUANTUM PAYMENTS" title="Invoice console" description="Testnet tUSD8 invoices for your merchant profile." />
      <Notice icon="lock">Testnet payments are temporarily unavailable. Try again later.</Notice></div>;
  }

  const tokenDecimals = config.data.token_decimals;
  const tokenSymbol = config.data.token_symbol;
  const explorerUrl = config.data.explorer_url;
  const rows = list.data?.items ?? [];
  const total = list.data?.total ?? 0;

  return (
    <div className="quantum-page">
      <TestnetBanner />
      <Heading eyebrow="QUANTUM PAYMENTS" title="Invoice console" description={`Create tUSD8 invoices on testnet chain ${config.data.chain_id}. Payments never touch the accounting ledger.`}
        action={merchant && canWrite ? <button type="button" className="button primary" disabled={creating} onClick={() => setCreating(true)}><Icon name="plus" size={16} /> New invoice</button> : undefined} />
      {merchantState === "loading" && <ApiStatus loading error="" onRetry={() => setMerchantVersion((value) => value + 1)} />}
      {merchantState === "error" && <ApiStatus loading={false} error={merchantError} onRetry={() => setMerchantVersion((value) => value + 1)} />}
      {merchantState === "missing" && (canWrite
        ? <MerchantSetup onCreated={(created) => setMerchantResult({ key: merchantKey, merchant: created, error: "", missing: false })} />
        : <Notice icon="lock">Your merchant profile has not been set up. Ask an administrator for permission to manage Quantum payments.</Notice>)}
      {merchantState === "ready" && merchant && <>
        <Panel title="Merchant" description="Payments go to the receiver below. This address cannot be changed.">
          <div className="summary-list">
            <div><span>Display name</span><strong>{merchant.display_name}</strong></div>
            <div><span>Receiver</span><strong className="mono">{merchant.receiver_address}</strong></div>
            <div><span>Token</span><strong>{tokenSymbol} · {config.data.chain_id}</strong></div>
          </div>
        </Panel>
        {creating && canWrite && <InvoiceForm decimals={tokenDecimals} tokenSymbol={tokenSymbol} onClose={() => setCreating(false)}
          onCreated={(invoice: Invoice) => { setCreating(false); setPage(0); setSelected(invoice.id); refresh(); }} />}
        <Panel title="Invoices" description="Newest first. Open an invoice to share its checkout link or cancel it."
          action={<label className="quantum-filter">Status
            <select value={status} onChange={(event) => { setStatus(QUANTUM_INVOICE_STATUSES.find((value) => value === event.target.value) ?? ""); setPage(0); }}>
              <option value="">All</option>
              {QUANTUM_INVOICE_STATUSES.map((value) => <option key={value} value={value}>{invoiceStatusLabel(value)}</option>)}
            </select>
          </label>}>
          <ApiStatus loading={list.loading && !list.data} error={list.error} forbidden={list.forbidden} onRetry={list.reload} />
          {list.data && <div className={`table-scroll${list.loading ? " is-refreshing" : ""}`}><table>
            <caption className="sr-only">Quantum invoices</caption>
            <thead><tr><th>INVOICE</th><th>AMOUNT</th><th>STATUS</th><th>CREATED</th><th>EXPIRES</th><th>DETAILS</th></tr></thead>
            <tbody>{rows.map((invoice) => <tr key={invoice.id}>
              <td><strong className="cell-primary mono quantum-break">{invoice.id}</strong><span className="cell-secondary">{invoice.description}</span></td>
              <td className="money">{formatTokenAmount(invoice.amount_base_units, tokenDecimals)} <small>{tokenSymbol}</small></td>
              <td><Badge tone={invoiceStatusTone(invoice.status)}>{invoiceStatusLabel(invoice.status)}</Badge></td>
              <td>{new Date(invoice.created_at).toLocaleString()}</td>
              <td>{new Date(invoice.expires_at).toLocaleString()}</td>
              <td><button type="button" className="text-link" aria-expanded={selected === invoice.id} onClick={() => setSelected(selected === invoice.id ? null : invoice.id)}>{selected === invoice.id ? "Close" : "View invoice"}</button></td>
            </tr>)}</tbody>
          </table>{rows.length === 0 && <Empty>No invoices for this filter yet.</Empty>}</div>}
          {total > PAGE_SIZE && <Pagination label="Invoice pages" page={page} pageSize={PAGE_SIZE} total={total} onPageChange={setPage} disabled={list.loading} />}
        </Panel>
        {selected && <InvoiceDetails key={`${selected}-${version}`} id={selected} version={version} decimals={tokenDecimals} tokenSymbol={tokenSymbol}
          explorerUrl={explorerUrl} onClose={() => setSelected(null)} onUpdated={refresh} />}
      </>}
    </div>
  );
}
