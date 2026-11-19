// Ledger Core API client. Types mirror api/internal/model/ledger.go (coding rule 4).
// Amounts and balances are strings of minor units ("50000"); use lib/money.ts.
import { request, type Page } from "./auth-api";

export const CURRENCIES = ["JPY", "USD", "EUR"] as const;
export type Currency = (typeof CURRENCIES)[number];
export const ACCOUNT_TYPES = ["ASSET", "LIABILITY", "EQUITY", "REVENUE", "EXPENSE"] as const;
export type AccountType = (typeof ACCOUNT_TYPES)[number];
export type Direction = "DEBIT" | "CREDIT";

export type LedgerAccount = {
  id: string;
  account_number: string;
  name: string;
  type: AccountType;
  currency: Currency;
  overdraft_allowed: boolean;
  is_active: boolean;
  /** Minor units in the account's normal direction; negative only with overdraft. */
  balance: string;
  created_at: string;
};

export type Posting = {
  id: string;
  journal_id: string;
  account_id: string;
  account_number: string;
  account_name: string;
  direction: Direction;
  amount: string;
  created_at: string;
};

export type AccountPosting = Posting & { reference_id: string; description: string; reversal_for_id: string | null };

export type Journal = {
  id: string;
  reference_id: string;
  description: string;
  currency: Currency;
  posted_at: string;
  /** Set on a reversal: the journal it reverses. */
  reversal_for_id: string | null;
  /** Set on a reversed journal: its reversal. */
  reversed_by_id: string | null;
  created_by_email: string | null;
  total: string;
  postings: Posting[];
};

export type ZeroSumReport = {
  currency: Currency;
  total_debits: string;
  total_credits: string;
  difference: string;
  journals: number;
  postings: number;
  unbalanced_journals: string[];
  balanced: boolean;
  checked_at: string;
};

export type NewAccount = { account_number: string; name: string; type: AccountType; currency: Currency; overdraft_allowed: boolean };
export type TransferLeg = { account_id: string; direction: Direction; amount: string };
export type TransferInput = { reference_id: string; description: string; currency: Currency; legs: TransferLeg[] };

export type AccountQuery = { q?: string; type?: AccountType | ""; currency?: Currency | ""; limit: number; offset: number };
export type JournalQuery = { q?: string; account_id?: string; limit: number; offset: number };

function params(values: Record<string, string | number | undefined>) {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(values)) {
    if (v !== undefined && v !== "") p.set(k, String(v));
  }
  return p.toString();
}

export const fetchAccounts = (token: string, q: AccountQuery) =>
  request<Page<LedgerAccount>>(`/accounts?${params({ q: q.q?.trim(), type: q.type, currency: q.currency, limit: q.limit, offset: q.offset })}`, { token });

export const createAccount = (token: string, input: NewAccount) =>
  request<LedgerAccount>("/accounts", { method: "POST", body: JSON.stringify(input), token });

export const fetchAccountPostings = (token: string, accountId: string, limit: number, offset: number) =>
  request<Page<AccountPosting>>(`/accounts/${encodeURIComponent(accountId)}/postings?${params({ limit, offset })}`, { token });

export const fetchJournals = (token: string, q: JournalQuery) =>
  request<Page<Journal>>(`/transfers?${params({ q: q.q?.trim(), account_id: q.account_id, limit: q.limit, offset: q.offset })}`, { token });

/**
 * Posts a transfer. Reuse the same idempotencyKey when retrying the same
 * transfer (e.g. after a timeout): the API then returns the original result
 * instead of posting twice. Use a new key for a new transfer.
 */
export const postTransfer = (token: string, idempotencyKey: string, input: TransferInput) =>
  request<Journal>("/transfers", { method: "POST", body: JSON.stringify(input), token, headers: { "Idempotency-Key": idempotencyKey }, timeoutMs: 20000 });

/** Posts the full reversal of a journal (idempotent like postTransfer). */
export const reverseJournal = (token: string, idempotencyKey: string, journalId: string, description?: string) =>
  request<Journal>(`/transfers/${encodeURIComponent(journalId)}/reverse`, {
    method: "POST",
    body: JSON.stringify(description ? { description } : {}),
    token,
    headers: { "Idempotency-Key": idempotencyKey },
    timeoutMs: 20000,
  });

export const fetchZeroSum = (token: string, currency: Currency) =>
  request<ZeroSumReport>(`/audit/zero-sum?${params({ currency })}`, { token });
