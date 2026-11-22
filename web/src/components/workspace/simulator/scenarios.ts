import type { Currency, TransferInput } from "@/lib/ledger-api";

export const PARALLEL_REQUESTS = 5;
const MAX_MINOR = BigInt("9223372036854775807");

export type ScenarioBuild = {
  currency: Currency;
  amount: string;
  from: string;
  to: string;
  reference: string;
  description: string;
};

export type ApiFailure = { status: number; code: string; message: string };

export type Attempt = {
  index: number;
  status: number;
  code: string;
  journalId: string | null;
  message: string;
};

export type AttemptSummary = {
  commits: number;
  replays: number;
  errors: number;
  journalIds: string[];
  ok: boolean;
};

export function tamperedAmount(minor: string): string {
  const value = BigInt(minor);
  const one = BigInt(1);
  if (value <= one) return "2";
  if (value >= MAX_MINOR - one) return (value - one).toString();
  return (value + one).toString();
}

export function buildTransferInput(build: ScenarioBuild, tampered = false): TransferInput {
  return {
    reference_id: build.reference,
    description: build.description,
    currency: build.currency,
    legs: [
      { account_id: build.from, direction: "DEBIT", amount: tampered ? tamperedAmount(build.amount) : build.amount },
      { account_id: build.to, direction: "CREDIT", amount: tampered ? tamperedAmount(build.amount) : build.amount },
    ],
  };
}

export function classifyFailure(failure: ApiFailure): string {
  if (failure.code === "idempotency_key_reused") return "Payload conflict (422): this key was already used with a different request.";
  if (failure.code === "idempotency_in_flight") return "In flight (409): retry with the same key when the first request finishes.";
  if (failure.status === 403) return "Access denied (403): posting needs transfers:create.";
  if (failure.status === 0) return "Network error: retrying the same key is safe.";
  if (failure.status >= 500) return "API unavailable: retrying the same key is safe.";
  return `Request rejected (${failure.status}): ${failure.message}`;
}

export function summarizeAttempts(attempts: readonly Attempt[]): AttemptSummary {
  const journalIds: string[] = [];
  let successes = 0;
  for (const attempt of attempts) {
    if (attempt.journalId === null) continue;
    successes++;
    if (!journalIds.includes(attempt.journalId)) journalIds.push(attempt.journalId);
  }
  const commits = journalIds.length;
  const replays = successes - commits;
  const errors = attempts.length - successes;
  return {
    commits,
    replays,
    errors,
    journalIds,
    ok: commits === 1 && replays === attempts.length - 1 && errors === 0,
  };
}
