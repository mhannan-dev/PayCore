"use client";

import { useWorkspaceCurrency } from "@/lib/workspace-currency";
import { useState } from "react";
import { object, ref, string, ValidationError, type InferType } from "yup";
import { AuthApiError } from "@/lib/auth-api";
import { useAuthSession } from "@/lib/auth-session";
import { fetchAccounts, fetchOutboxMetrics, postTransfer, type Currency, type Journal } from "@/lib/ledger-api";
import { formatMoney, parseMoneyInput } from "@/lib/money";
import { hasPermission, PERMISSIONS } from "@/lib/rbac";
import { usePermission } from "../../can";
import { Icon } from "../../icons";
import { Badge, EmptyHint, Notice, Panel } from "../../ui";
import { ApiStatus } from "../shared/api-status";
import { useApi } from "../shared/use-api";
import {
  buildTransferInput,
  classifyFailure,
  PARALLEL_REQUESTS,
  summarizeAttempts,
  type ApiFailure,
  type Attempt,
  type AttemptSummary,
  type ScenarioBuild,
} from "./scenarios";

const scenarioSchema = (currency: Currency) => object({
  amount: string()
    .required("Enter an amount.")
    .test("money", "Enter a positive amount within the currency precision and int64 limits.", (value) => typeof value === "string" && !("error" in parseMoneyInput(value, currency))),
  from: string().required("Choose the account to debit."),
  to: string().required("Choose the account to credit.").notOneOf([ref("from")], "Choose two different accounts."),
});
type ScenarioValues = InferType<ReturnType<typeof scenarioSchema>>;

type StoredScenario = { key: string; build: ScenarioBuild };

function failureFrom(reason: unknown): ApiFailure {
  if (reason instanceof AuthApiError) return { status: reason.status, code: reason.code, message: reason.message };
  return { status: 0, code: "network_error", message: "Could not reach the PayCore API." };
}

function journalAttempt(index: number, journal: Journal): Attempt {
  return {
    index,
    status: 201,
    code: "journal_posted",
    journalId: journal.id,
    message: `${journal.reference_id} · ${formatMoney(journal.total, journal.currency)}`,
  };
}

function SimulationResults({
  attempts,
  summary,
  mode,
  delta,
  busy,
}: {
  attempts: Attempt[];
  summary: AttemptSummary | null;
  mode: "duplicate" | "tamper" | null;
  delta: number | null;
  busy: boolean;
}) {
  const first = attempts[0];
  const tamperOk = first?.status === 422 && first.code === "idempotency_key_reused";
  const ok = attempts.length === 0 ? false : mode === "tamper" ? tamperOk : summary?.ok === true;
  const tone = attempts.length === 0 || busy ? "neutral" : ok ? "green" : "amber";
  const label = busy ? "Running…" : attempts.length === 0 ? "No scenario yet" : ok ? "Expected result" : "Unexpected result";

  return (
    <Panel title="Request results" action={<Badge tone={tone}>{label}</Badge>}>
      {summary && (
        <p className="muted">
          {summary.commits} commit · {summary.replays} stored-response replays · {summary.errors} errors · journal <code>{summary.journalIds[0]}</code>
        </p>
      )}
      {delta !== null && (
        <p className="footnote">Outbox published_count changed by {delta}. This aggregate can also move when other transfers post.</p>
      )}
      {attempts.length ? (
        <div className="simulation-results" role="status">
          {attempts.map((attempt) => (
            <p key={attempt.index}>
              <code>{String(attempt.index).padStart(2, "0")}</code> · <code>{attempt.status === 0 ? "NET" : attempt.status}</code> ·{" "}
              {attempt.journalId === null ? classifyFailure(attempt) : <>Journal <code>{attempt.journalId}</code> · {attempt.message}</>}
            </p>
          ))}
        </div>
      ) : (
        <EmptyHint icon="code" title="Prove deduplication end to end." text="Run five parallel posts with one key, then retry that key with a changed amount." />
      )}
    </Panel>
  );
}

/** Exercises the real ledger API: same key, real concurrent posts, real outbox event. */
export function IdempotencySandbox() {
  const { currency } = useWorkspaceCurrency();
  const { session } = useAuthSession();
  const can = usePermission();
  const allowed = can(PERMISSIONS.transfersCreate);
  const canMetrics = hasPermission(session.permissions, PERMISSIONS.systemRead);
  const [amount, setAmount] = useState("50000");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [stored, setStored] = useState<StoredScenario | null>(null);
  const [attempts, setAttempts] = useState<Attempt[]>([]);
  const [summary, setSummary] = useState<AttemptSummary | null>(null);
  const [mode, setMode] = useState<"duplicate" | "tamper" | null>(null);
  const [delta, setDelta] = useState<number | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const accounts = useApi(`simulator-accounts-${currency}`, (token) => fetchAccounts(token, { currency, limit: 200, offset: 0 }));
  const active = (accounts.data?.items ?? []).filter((account) => account.is_active);
  const fromId = from || active.find((account) => account.account_number === "2001")?.id || active[0]?.id || "";
  const toId = to || active.find((account) => account.account_number === "2002")?.id || active.find((account) => account.id !== fromId)?.id || "";

  async function publishedCount(): Promise<number | null> {
    if (!canMetrics) return null;
    try {
      return (await fetchOutboxMetrics(session.token)).published_count;
    } catch {
      return null;
    }
  }

  async function runDuplicate() {
    if (busy || !allowed) return;
    setError("");
    let values: ScenarioValues;
    try {
      values = scenarioSchema(currency).validateSync({ amount, from: fromId, to: toId });
    } catch (cause: unknown) {
      setError(cause instanceof ValidationError ? cause.message : "Check the scenario and try again.");
      return;
    }
    const parsed = parseMoneyInput(values.amount, currency);
    if ("error" in parsed) return setError(parsed.error);

    const build: ScenarioBuild = {
      currency,
      amount: parsed.minor,
      from: values.from,
      to: values.to,
      reference: `SIM-${new Date().toISOString().slice(0, 10).replace(/-/g, "")}-${crypto.randomUUID().slice(0, 6).toUpperCase()}`,
      description: "Idempotency replay demo",
    };
    const key = crypto.randomUUID();
    const input = buildTransferInput(build);
    setBusy(true);
    setStored(null);
    setSummary(null);
    setMode("duplicate");
    setAttempts([]);
    setDelta(null);
    try {
      const before = await publishedCount();
      const results = await Promise.allSettled(Array.from({ length: PARALLEL_REQUESTS }, () => postTransfer(session.token, key, input)));
      const nextAttempts = results.map((result, index) =>
        result.status === "fulfilled"
          ? journalAttempt(index + 1, result.value)
          : { index: index + 1, ...failureFrom(result.reason), journalId: null },
      );
      const nextSummary = summarizeAttempts(nextAttempts);
      const after = await publishedCount();
      setAttempts(nextAttempts);
      setSummary(nextSummary);
      setDelta(before !== null && after !== null ? after - before : null);
      if (nextSummary.commits > 0) setStored({ key, build });
      accounts.reload();
    } finally {
      setBusy(false);
    }
  }

  async function runTampered() {
    if (busy || !allowed || !stored) return;
    setError("");
    setBusy(true);
    setSummary(null);
    setMode("tamper");
    setAttempts([]);
    setDelta(null);
    try {
      const before = await publishedCount();
      const input = buildTransferInput(stored.build, true);
      let attempt: Attempt;
      try {
        attempt = journalAttempt(1, await postTransfer(session.token, stored.key, input));
      } catch (cause: unknown) {
        const failure = failureFrom(cause);
        attempt = { index: 1, ...failure, journalId: null };
      }
      const after = await publishedCount();
      setAttempts([attempt]);
      setDelta(before !== null && after !== null ? after - before : null);
    } finally {
      setBusy(false);
    }
  }

  function clearScenario() {
    setStored(null);
    setAttempts([]);
    setSummary(null);
    setMode(null);
    setDelta(null);
    setError("");
  }

  function reset() {
    clearScenario();
    setAmount("50000");
  }

  return (
    <div className="form-grid">
      <Panel title="Live idempotency sandbox" description="Five parallel posts with one key prove that one intent commits once, even under concurrency.">
        {!allowed && <Notice icon="lock">Posting needs the <code>transfers:create</code> permission. Your role can open the simulator but cannot run live scenarios.</Notice>}
        <ApiStatus loading={accounts.loading && !accounts.data} error={accounts.error} forbidden={accounts.forbidden} onRetry={accounts.reload} />
        <div className="form-body">
          <label>
            Amount · {currency}
            <input value={amount} inputMode={currency === "JPY" ? "numeric" : "decimal"} onChange={(e) => { setAmount(e.target.value); clearScenario(); }} />
          </label>
          <div className="field-pair">
            <label>
              Debit account
              <select value={fromId} onChange={(e) => { setFrom(e.target.value); clearScenario(); }}>
                <option value="">Choose an account…</option>
                {active.map((account) => <option key={account.id} value={account.id}>{account.account_number} · {account.name}</option>)}
              </select>
            </label>
            <label>
              Credit account
              <select value={toId} onChange={(e) => { setTo(e.target.value); clearScenario(); }}>
                <option value="">Choose an account…</option>
                {active.map((account) => <option key={account.id} value={account.id}>{account.account_number} · {account.name}</option>)}
              </select>
            </label>
          </div>
          <label>
            Active key
            <code className="key-display">{stored?.key ?? "Generated for each duplicate run"}</code>
          </label>
          <div className="button-stack">
            <button className="button primary" disabled={busy || !allowed || !fromId || !toId} onClick={() => void runDuplicate()}>
              <Icon name="play" size={16} /> {busy ? "Running…" : "Run 5 duplicate requests"}
            </button>
            <button className="button secondary" disabled={busy || !allowed || !stored} onClick={() => void runTampered()}>
              <Icon name="edit" size={16} /> Test changed payload with the same key
            </button>
            <button className="text-link" onClick={reset}><Icon name="refresh" size={14} />Reset scenario</button>
          </div>
          {error && <p role="alert" className="error-message">{error}</p>}
          <p className="footnote">This posts real journals to the seeded accounts. The tamper test must return <code>422 idempotency_key_reused</code> and post nothing.</p>
        </div>
      </Panel>
      <SimulationResults attempts={attempts} summary={summary} mode={mode} delta={delta} busy={busy} />
    </div>
  );
}
