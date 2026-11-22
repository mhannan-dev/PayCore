"use client";

import { useRef, useState } from "react";
import { AuthApiError } from "@/lib/auth-api";
import { useAuthSession } from "@/lib/auth-session";
import { cancelInvoice, explorerTxUrl, fetchInvoice } from "@/lib/quantum-api";
import { formatTokenAmount } from "@/lib/quantum-amount";
import { attemptStatusLabel, attemptStatusTone, failureCodeMessage, invoiceStatusLabel, invoiceStatusTone } from "@/lib/quantum-labels";
import { notify } from "@/lib/notify";
import { PERMISSIONS } from "@/lib/rbac";
import { ConfirmationModal } from "../confirmation-modal";
import { usePermission } from "../can";
import { Badge, Notice, Panel } from "../ui";
import { ApiStatus } from "../workspace/shared/api-status";
import { useApi } from "../workspace/shared/use-api";

type Props = {
  id: string;
  version: number;
  decimals: number;
  tokenSymbol: string;
  explorerUrl: string;
  onClose: () => void;
  onUpdated: () => void;
};

export function InvoiceDetails({ id, version, decimals, tokenSymbol, explorerUrl, onClose, onUpdated }: Props) {
  const { session } = useAuthSession();
  const can = usePermission();
  const detail = useApi(`quantum-invoice-${id}-${version}`, (token) => fetchInvoice(token, id), 10000);
  const [confirming, setConfirming] = useState(false);
  const [cancelling, setCancelling] = useState(false);
  const [cancelError, setCancelError] = useState("");
  const [copyError, setCopyError] = useState("");
  const cancellingRequest = useRef(false);
  const invoice = detail.data;

  async function copyLink() {
    if (!invoice) return;
    setCopyError("");
    try {
      await navigator.clipboard.writeText(invoice.checkout_url);
      notify.success("Checkout link copied.");
    } catch {
      setCopyError("Could not copy the link. Select and copy the URL below instead.");
    }
  }

  async function confirmCancel() {
    if (cancellingRequest.current) return;
    cancellingRequest.current = true;
    setCancelError("");
    setCancelling(true);
    try {
      await cancelInvoice(session.token, id);
      notify.success("Invoice cancelled.");
      setConfirming(false);
      onUpdated();
    } catch (reason) {
      const message = reason instanceof AuthApiError ? reason.message : "Could not cancel the invoice. Try again.";
      setConfirming(false);
      setCancelError(message);
      notify.error(message);
      detail.reload();
    } finally {
      cancellingRequest.current = false;
      setCancelling(false);
    }
  }

  return (
    <Panel title="Invoice details" description={invoice?.id ?? "Loading invoice…"} action={<button type="button" className="text-link" onClick={onClose}>Close details</button>}>
      <ApiStatus loading={detail.loading && !invoice} error={detail.error} forbidden={detail.forbidden} onRetry={detail.reload} />
      {cancelError && <p className="error-message" role="alert">{cancelError}</p>}
      {invoice && !detail.error && <>
        <div className="summary-list">
          <div><span>Status</span><Badge tone={invoiceStatusTone(invoice.status)}>{invoiceStatusLabel(invoice.status)}</Badge></div>
          <div><span>Amount</span><strong className="money">{formatTokenAmount(invoice.amount_base_units, decimals)} {tokenSymbol}</strong></div>
          <div><span>Description</span><strong>{invoice.description}</strong></div>
          <div><span>Receiver</span><strong className="mono">{invoice.receiver_address}</strong></div>
          <div><span>Checkout link</span><strong className="mono quantum-break">{invoice.checkout_url}</strong></div>
          <div><span>Expires</span><strong>{new Date(invoice.expires_at).toLocaleString()}</strong></div>
          {invoice.paid_at && <div><span>Paid at</span><strong>{new Date(invoice.paid_at).toLocaleString()}</strong></div>}
          {invoice.paid_tx_hash && <div><span>Paid transaction</span><a className="text-link mono" href={explorerTxUrl(explorerUrl, invoice.paid_tx_hash)} target="_blank" rel="noreferrer">{invoice.paid_tx_hash}</a></div>}
        </div>
        <div className="settlement-actions">
          <button className="button secondary" type="button" onClick={() => void copyLink()}>Copy checkout link</button>
          {invoice.status === "PENDING" && can(PERMISSIONS.quantumWrite) && <button className="button danger" type="button" disabled={cancelling || detail.loading} onClick={() => { setCancelError(""); setConfirming(true); }}>Cancel invoice</button>}
        </div>
        {copyError && <p className="error-message" role="alert">{copyError}</p>}
        <section className="quantum-attempts">
          <h3>Payment attempts</h3>
          <div className="table-scroll"><table>
            <caption className="sr-only">Payment attempts for this invoice</caption>
            <thead><tr><th>Payer</th><th>STATUS</th><th>TRANSACTION</th><th>CONFIRMATIONS</th><th>UPDATED</th></tr></thead>
            <tbody>{invoice.attempts.map((attempt) => <tr key={attempt.id}>
              <td className="mono quantum-break">{attempt.payer_address}</td>
              <td><Badge tone={attemptStatusTone(attempt.status)}>{attemptStatusLabel(attempt.status)}</Badge>
                {attempt.failure_code && <span className="cell-secondary">{failureCodeMessage(attempt.failure_code)}</span>}
              </td>
              <td>{attempt.tx_hash ? <a className="text-link mono quantum-break" href={explorerTxUrl(explorerUrl, attempt.tx_hash)} target="_blank" rel="noreferrer">{attempt.tx_hash}</a> : <span className="muted">Not submitted yet</span>}</td>
              <td>{attempt.confirmations}</td>
              <td>{new Date(attempt.updated_at).toLocaleString()}</td>
            </tr>)}</tbody>
          </table>{invoice.attempts.length === 0 && <Notice icon="wallet">No payment attempts yet.</Notice>}</div>
        </section>
      </>}
      {confirming && can(PERMISSIONS.quantumWrite) && <ConfirmationModal open={confirming} title="Cancel this invoice?" description="The checkout link will stop accepting payments. This cannot be undone."
        confirmLabel="Cancel invoice" pendingLabel="Cancelling…" pending={cancelling} onCancel={() => setConfirming(false)} onConfirm={() => void confirmCancel()} />}
    </Panel>
  );
}
