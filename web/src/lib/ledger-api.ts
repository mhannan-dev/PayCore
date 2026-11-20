// Ledger Core API client. Types mirror api/internal/models/ledger.go (coding rule 4).
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

// Mirrors api/internal/models/statement.go. CSV input amounts are major units; these API
// amounts and totals are always integer minor-unit strings.
export type StatementEntry = { booked_date: string; reference: string; currency: Currency; amount: string; direction: Direction; description: string };
export type StatementRow = { row_number: number; values: string[]; entry: StatementEntry | null; errors: string[] };
export type StatementPreview = {
  attachment_id: string; account_id: string; file_name: string; currency: Currency;
  rows: StatementRow[]; invalid_rows: number; total_debits: string; total_credits: string;
  content_hash: string | null; duplicate_import_id: string | null; can_import: boolean;
};
export type StatementImport = {
  id: string; attachment_id: string | null; account_id: string; file_name: string; currency: Currency;
  content_hash: string; row_count: number; total_debits: string; total_credits: string; created_at: string;
};
export type StatementDetail = { import: StatementImport; rows: StatementEntry[] };
export type StatementRequest = { attachment_id: string; account_id: string; expected_hash?: string };
export const previewStatement = (token: string, input: StatementRequest) =>
  request<StatementPreview>("/reconciliation/preview", { token, method: "POST", body: JSON.stringify(input) });
export const importStatement = (token: string, input: StatementRequest) =>
  request<StatementImport>("/reconciliation/imports", { token, method: "POST", body: JSON.stringify(input) });
export const fetchStatementImports = (token: string, offset = 0) =>
  request<StatementImport[]>(`/reconciliation/imports?limit=20&offset=${offset}`, { token });
export const fetchStatementDetail = (token: string, id: string, offset = 0) =>
  request<StatementDetail>(`/reconciliation/imports/${encodeURIComponent(id)}?limit=50&offset=${offset}`, { token });

export const STATEMENT_DIRECTION_MODES = ["SAME", "OPPOSITE"] as const;
export type StatementDirectionMode = (typeof STATEMENT_DIRECTION_MODES)[number];
export type MatchStatus = "PENDING" | "UNMATCHED" | "SUGGESTED" | "AMBIGUOUS" | "MATCHED" | "REJECTED";
export type MatchAction = "CONFIRM" | "REJECT" | "RESET" | "NOTE";
export type StatementCandidate = { posting_id: string; journal_id: string; reference: string; amount: string; direction: Direction; posted_at: string };
export type MatchingRow = {
  row_number: number; entry: StatementEntry; status: MatchStatus; matched_posting_id: string | null;
  note: string | null; rejected_count: number; candidate_count: number; candidates: StatementCandidate[];
};
export type MatchingReport = { import: StatementImport; direction_mode: StatementDirectionMode | null; version: number; rows: MatchingRow[]; limit: number; offset: number };
export type MatchingConfigRequest = { direction_mode: StatementDirectionMode; expected_version: number };
export type MatchingReviewRequest = { action: MatchAction; posting_id: string | null; reason: string; expected_version: number };
export const fetchMatchingReport = (token: string, id: string, offset = 0) =>
  request<MatchingReport>(`/reconciliation/imports/${encodeURIComponent(id)}/matches?limit=50&offset=${offset}`, { token });
export const configureMatching = (token: string, id: string, input: MatchingConfigRequest) =>
  request<void>(`/reconciliation/imports/${encodeURIComponent(id)}/matching`, { token, method: "POST", body: JSON.stringify(input) });
export const reviewStatementMatch = (token: string, id: string, row: number, input: MatchingReviewRequest) =>
  request<void>(`/reconciliation/imports/${encodeURIComponent(id)}/rows/${row}/match`, { token, method: "POST", body: JSON.stringify(input) });
