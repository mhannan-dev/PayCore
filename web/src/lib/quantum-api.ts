import { request, type Page } from "./auth-api";

export const QUANTUM_INVOICE_STATUSES = ["PENDING", "PAID", "EXPIRED", "CANCELLED"] as const;
export type InvoiceStatus = (typeof QUANTUM_INVOICE_STATUSES)[number];

export const QUANTUM_ATTEMPT_STATUSES = ["CREATED", "BROADCAST", "VERIFYING", "CONFIRMED", "FAILED", "REORGED", "REVIEW_REQUIRED"] as const;
export type AttemptStatus = (typeof QUANTUM_ATTEMPT_STATUSES)[number];

export const QUANTUM_FAILURE_CODES = [
  "wrong_chain",
  "wrong_token",
  "wrong_recipient",
  "wrong_payer",
  "underpaid",
  "overpaid",
  "late_payment",
  "tx_failed",
  "tx_not_found",
  "duplicate_transfer",
] as const;
export type FailureCode = (typeof QUANTUM_FAILURE_CODES)[number];

export type QuantumConfig = {
  enabled: boolean;
  chain_id: number;
  token_address: string;
  token_symbol: string;
  token_decimals: number;
  explorer_url: string;
  min_confirmations: number;
};

export type Merchant = { id: string; display_name: string; receiver_address: string; created_at: string };

export type Attempt = {
  id: string;
  payer_address: string;
  tx_hash: string | null;
  status: AttemptStatus;
  failure_code: FailureCode | null;
  confirmations: number;
  created_at: string;
  updated_at: string;
};

export type Invoice = {
  id: string;
  description: string;
  amount_base_units: string;
  chain_id: number;
  token_address: string;
  receiver_address: string;
  status: InvoiceStatus;
  checkout_url: string;
  expires_at: string;
  paid_at: string | null;
  paid_tx_hash: string | null;
  attempts: Attempt[];
  created_at: string;
};

export type Checkout = {
  merchant_name: string;
  description: string;
  amount_base_units: string;
  chain_id: number;
  token_address: string;
  token_symbol: string;
  token_decimals: number;
  receiver_address: string;
  status: InvoiceStatus;
  expires_at: string;
  explorer_url: string;
};

export type CheckoutStatus = { invoice_status: InvoiceStatus; attempt: Attempt | null; paid_tx_hash: string | null };

export type CreateMerchantInput = { display_name: string; receiver_address: string };
export type CreateInvoiceInput = { amount_base_units: string; description: string; expires_in_minutes: number };
export type CreateAttemptInput = { payer_address: string };
export type SubmitTxInput = { tx_hash: string };
export type InvoiceQuery = { status?: InvoiceStatus | ""; limit: number; offset: number };

function params(values: Record<string, string | number | undefined>) {
  const result = new URLSearchParams();
  for (const [key, value] of Object.entries(values)) if (value !== undefined && value !== "") result.set(key, String(value));
  return result.toString();
}

export const fetchQuantumConfig = () => request<QuantumConfig>("/quantum/config");

export function explorerTxUrl(explorerUrl: string, txHash: string): string {
  return `${explorerUrl.replace(/\/+$/, "")}/tx/${encodeURIComponent(txHash)}`;
}

export const createMerchant = (token: string, input: CreateMerchantInput) =>
  request<Merchant>("/quantum/merchants", { method: "POST", token, body: JSON.stringify(input), timeoutMs: 20000 });

export const fetchMerchant = (token: string) => request<Merchant>("/quantum/merchants/me", { token });

export const createInvoice = (token: string, idempotencyKey: string, input: CreateInvoiceInput) =>
  request<Invoice>("/quantum/invoices", { method: "POST", token, headers: { "Idempotency-Key": idempotencyKey }, body: JSON.stringify(input), timeoutMs: 20000 });

export const fetchInvoices = (token: string, query: InvoiceQuery) =>
  request<Page<Invoice>>(`/quantum/invoices?${params({ status: query.status, limit: query.limit, offset: query.offset })}`, { token });

export const fetchInvoice = (token: string, id: string) =>
  request<Invoice>(`/quantum/invoices/${encodeURIComponent(id)}`, { token });

export const cancelInvoice = (token: string, id: string) =>
  request<Invoice>(`/quantum/invoices/${encodeURIComponent(id)}/cancel`, { method: "POST", token, timeoutMs: 20000 });

export const fetchCheckout = (checkoutToken: string, signal?: AbortSignal) =>
  request<Checkout>(`/quantum/checkout/${encodeURIComponent(checkoutToken)}`, { signal });

export const createCheckoutAttempt = (checkoutToken: string, input: CreateAttemptInput, signal?: AbortSignal) =>
  request<Attempt>(`/quantum/checkout/${encodeURIComponent(checkoutToken)}/attempts`, { method: "POST", body: JSON.stringify(input), signal, timeoutMs: 20000 });

export const submitCheckoutTx = (checkoutToken: string, attemptId: string, input: SubmitTxInput, signal?: AbortSignal) =>
  request<Attempt>(`/quantum/checkout/${encodeURIComponent(checkoutToken)}/attempts/${encodeURIComponent(attemptId)}/tx`, { method: "POST", body: JSON.stringify(input), signal, timeoutMs: 20000 });

export const fetchCheckoutStatus = (checkoutToken: string, signal?: AbortSignal) =>
  request<CheckoutStatus>(`/quantum/checkout/${encodeURIComponent(checkoutToken)}/status`, { signal });
