"use client";

import { useEffect, useRef, useState } from "react";
import { type Hash } from "viem";
import { useConnect, useConnection, useDisconnect, usePublicClient, useSwitchChain, useWriteContract } from "wagmi";
import { AuthApiError } from "@/lib/auth-api";
import { createCheckoutAttempt, explorerTxUrl, fetchCheckoutStatus, submitCheckoutTx, type Checkout, type CheckoutStatus } from "@/lib/quantum-api";
import { formatTokenAmount } from "@/lib/quantum-amount";
import { QUANTUM_PAYMENTS_ABI, QUANTUM_TOKEN_ABI, sendCheckoutPayment } from "@/lib/quantum-payment";
import { attemptStatusLabel, failureCodeMessage } from "@/lib/quantum-labels";
import { Notice } from "../../ui";

type Props = { token: string; checkout: Checkout; minConfirmations: number; onStatusChange: (status: CheckoutStatus) => void };

function invoiceClosed(checkout: Checkout, status: CheckoutStatus | null): boolean {
  return checkout.status !== "PENDING"
    || status?.invoice_status === "PAID"
    || status?.invoice_status === "EXPIRED"
    || status?.invoice_status === "CANCELLED";
}

function stopPolling(status: CheckoutStatus | null): boolean {
  const attemptStatus = status?.attempt?.status;
  return attemptStatus === "FAILED" || attemptStatus === "REVIEW_REQUIRED";
}

function describeError(reason: unknown): string {
  if (reason instanceof AuthApiError) return reason.message;
  if (reason instanceof Error) {
    const text = reason.message.toLowerCase();
    if (text.includes("rejected") || text.includes("denied") || text.includes("4001")) return "The wallet request was rejected. Nothing was sent.";
    return reason.message;
  }
  return "Could not complete the payment. Try again.";
}

export function CheckoutFlow({ token, checkout, minConfirmations, onStatusChange }: Props) {
  const connection = useConnection();
  const { connectors, connectAsync, isPending: connecting } = useConnect();
  const { disconnect } = useDisconnect();
  const { switchChainAsync, isPending: switching } = useSwitchChain();
  const { writeContractAsync } = useWriteContract();
  const publicClient = usePublicClient({ chainId: checkout.chain_id });
  const sending = useRef(false);
  const [broadcast, setBroadcast] = useState<{ attemptId: string; hash: Hash } | null>(null);
  const [status, setStatus] = useState<CheckoutStatus | null>(null);
  const [statusError, setStatusError] = useState("");
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [started, setStarted] = useState(false);
  const closed = invoiceClosed(checkout, status);
  const latest = status?.attempt ?? null;
  const paused = stopPolling(status);
  const shouldPoll = !closed && !paused && (started || Boolean(latest?.tx_hash));

  useEffect(() => {
    let cancelled = false;
    fetchCheckoutStatus(token).then(
      (next) => { if (!cancelled) { setStatus(next); onStatusChange(next); } },
      (reason: unknown) => { if (!cancelled && reason instanceof Error) setStatusError(reason.message); },
    );
    return () => { cancelled = true; };
  }, [token, onStatusChange]);

  useEffect(() => {
    if (!shouldPoll) return;
    let cancelled = false;
    const refresh = () => {
      fetchCheckoutStatus(token).then(
        (next) => { if (!cancelled) { setStatus(next); onStatusChange(next); setStatusError(""); } },
        (reason: unknown) => { if (!cancelled && reason instanceof Error) setStatusError(reason.message); },
      );
    };
    refresh();
    const interval = window.setInterval(refresh, 3000);
    return () => { cancelled = true; window.clearInterval(interval); };
  }, [token, shouldPoll, onStatusChange]);

  const address = connection.status === "connected" ? connection.address : undefined;
  const connectedChain = connection.status === "connected" ? connection.chainId : undefined;
  const wrongNetwork = connection.status === "connected" && connectedChain !== checkout.chain_id;
  const inFlight = latest?.status === "BROADCAST" || latest?.status === "VERIFYING" || latest?.status === "REORGED";
  const cannotPay = closed || latest?.status === "REVIEW_REQUIRED" || inFlight || (started && !paused) || wrongNetwork || !address || busy || broadcast !== null;
  const paid = status?.invoice_status === "PAID";
  const paidHash = status?.paid_tx_hash ?? null;

  async function connect() {
    const [connector] = connectors;
    if (!connector) {
      setError("No browser wallet was found. Install an EVM wallet on this device and reload.");
      return;
    }
    setError("");
    try {
      await connectAsync({ connector });
    } catch (reason) {
      setError(describeError(reason));
    }
  }

  async function switchNetwork() {
    setError("");
    try {
      await switchChainAsync({ chainId: checkout.chain_id });
    } catch (reason) {
      setError(describeError(reason));
    }
  }

  async function pay() {
    if (sending.current || !address || cannotPay) return;
    if (!publicClient) {
      setError("Your wallet is not ready. Reconnect and try again.");
      return;
    }
    sending.current = true;
    setBusy(true);
    setError("");
    setMessage("");
    try {
      const payment = await sendCheckoutPayment(checkout, {
        allowance: async ({ token: tokenAddress, contract }) => publicClient.readContract({ address: tokenAddress, abi: QUANTUM_TOKEN_ABI, functionName: "allowance", args: [address, contract] }),
        approve: async ({ token: tokenAddress, contract, amount }) => writeContractAsync({ address: tokenAddress, abi: QUANTUM_TOKEN_ABI, functionName: "approve", args: [contract, amount], account: address, chainId: checkout.chain_id, type: "legacy" }),
        waitForApproval: async (hash) => {
          const receipt = await publicClient.waitForTransactionReceipt({ hash, confirmations: 1, timeout: 120000 });
          if (receipt.status !== "success") throw new Error("Token approval failed. No payment was sent.");
        },
        register: async () => {
          if (await publicClient.getChainId() !== checkout.chain_id) throw new Error("Switch back to the invoice test network before paying.");
          return (await createCheckoutAttempt(token, { payer_address: address.toLowerCase() })).id;
        },
        pay: async ({ contract, orderId, merchant, amount }) => writeContractAsync({ address: contract, abi: QUANTUM_PAYMENTS_ABI, functionName: "pay", args: [orderId, merchant, amount], account: address, chainId: checkout.chain_id, type: "legacy" }),
        transfer: async ({ token: tokenAddress, merchant, amount }) => writeContractAsync({ address: tokenAddress, abi: QUANTUM_TOKEN_ABI, functionName: "transfer", args: [merchant, amount], account: address, chainId: checkout.chain_id, type: "legacy" }),
      }, setMessage);
      setBroadcast(payment);
      setMessage("Payment broadcast. Waiting for the API to verify it…");
      await submitCheckoutTx(token, payment.attemptId, { tx_hash: payment.hash.toLowerCase() });
      setBroadcast(null);
      setStarted(true);
      setMessage("Transaction submitted. Waiting for testnet confirmations…");
    } catch (reason) {
      if (reason instanceof AuthApiError && reason.code === "tx_already_submitted") {
        setBroadcast(null);
        setStarted(true);
        setMessage("This transaction was already registered. Waiting for verification…");
        return;
      }
      setError(describeError(reason));
    } finally {
      sending.current = false;
      setBusy(false);
    }
  }

  async function retrySubmission() {
    if (!broadcast || sending.current) return;
    sending.current = true;
    setBusy(true);
    setError("");
    try {
      await submitCheckoutTx(token, broadcast.attemptId, { tx_hash: broadcast.hash.toLowerCase() });
      setBroadcast(null);
      setStarted(true);
      setMessage("Transaction submitted. Waiting for testnet confirmations…");
    } catch (reason) {
      if (reason instanceof AuthApiError && reason.code === "tx_already_submitted") {
        setBroadcast(null);
        setStarted(true);
      } else setError(describeError(reason));
    } finally {
      sending.current = false;
      setBusy(false);
    }
  }

  const invoiceStatus = status?.invoice_status ?? checkout.status;
  const statusText = paid
    ? "Payment confirmed. This invoice is paid."
    : invoiceStatus === "EXPIRED" ? "This invoice expired before payment."
      : invoiceStatus === "CANCELLED" ? "The merchant cancelled this invoice."
        : latest?.failure_code ? failureCodeMessage(latest.failure_code)
          : latest?.status === "FAILED" ? "The transaction failed on testnet. You can register a new attempt."
            : latest?.status === "REORGED" ? "The payment was reorganised on testnet. Wait for status to settle, then try again."
              : message || "Connect your testnet wallet, then send the exact amount shown.";

  return (
    <section className="quantum-checkout-actions" aria-busy={busy}>
      <div className="quantum-wallet">
        {connection.status === "connected" ? <>
          <span className="quantum-wallet-address mono">{address}</span>
          <span className="quantum-network">{connectedChain === checkout.chain_id ? `Testnet ${checkout.chain_id}` : `Network ${String(connectedChain)}`}</span>
          <button type="button" className="text-link" onClick={() => disconnect()}>Disconnect</button>
        </> : <button type="button" className="button secondary" disabled={connecting} onClick={() => void connect()}>
          {connecting ? "Connecting…" : "Connect wallet"}
        </button>}
      </div>
      {wrongNetwork && <Notice icon="shield">Your wallet is on the wrong network. Switch to testnet chain {checkout.chain_id} before paying.</Notice>}
      {wrongNetwork && <button type="button" className="button secondary" disabled={switching} onClick={() => void switchNetwork()}>{switching ? "Switching…" : "Switch network"}</button>}
      <p className="quantum-status" role="status" aria-live="polite">{statusText}</p>
      {statusError && <p className="muted" role="status">{statusError}</p>}
      {error && <p className="error-message" role="alert">{error}</p>}
      {broadcast && !paid && <Notice icon="shield">Payment was broadcast. Keep this page open and retry verification submission; do not pay again. <a className="text-link" href={explorerTxUrl(checkout.explorer_url, broadcast.hash)} target="_blank" rel="noreferrer">View transaction</a><button type="button" className="button secondary" disabled={busy} onClick={() => void retrySubmission()}>Retry verification submission</button></Notice>}
      {!paid && !closed && latest?.status !== "REVIEW_REQUIRED" && <>
        <button type="button" className="button primary quantum-pay" disabled={cannotPay} onClick={() => void pay()}>
          {busy ? "Sending…" : `Pay ${formatTokenAmount(checkout.amount_base_units, checkout.token_decimals)} ${checkout.token_symbol}`}
        </button>
        <p className="quantum-field-hint">{checkout.payment_contract_address === null ? "Confirm the transfer in your wallet." : "Approve the invoice amount if requested, then confirm payment in your wallet. Token approval alone does not pay the invoice."} Success is shown only after the API verifies payment.</p>
      </>}
      {latest && <div className="quantum-attempt-status">
        <strong>{attemptStatusLabel(latest.status)}</strong>
        {latest.tx_hash && <span>Confirmations: {latest.confirmations} / {minConfirmations}</span>}
      </div>}
      {latest?.tx_hash && <p className="quantum-field-hint">
        <a className="text-link mono" href={explorerTxUrl(checkout.explorer_url, latest.tx_hash)} target="_blank" rel="noreferrer">View transaction on the testnet explorer</a>
        {" · a hash alone does not mean the invoice is paid"}
      </p>}
      {paid && paidHash && <p><a className="text-link mono" href={explorerTxUrl(checkout.explorer_url, paidHash)} target="_blank" rel="noreferrer">{paidHash}</a></p>}
    </section>
  );
}
