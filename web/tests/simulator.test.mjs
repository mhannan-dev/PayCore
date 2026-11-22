import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import vm from "node:vm";
import ts from "typescript";

const source = readFileSync(new URL("../src/components/workspace/simulator/scenarios.ts", import.meta.url), "utf8");
const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 } });
const scenarios = {};
vm.runInNewContext(outputText, { exports: scenarios });

const build = { currency: "JPY", amount: "50000", from: "account-a", to: "account-b", reference: "SIM-1", description: "Idempotency replay demo" };

function attempt(index, journalId, status = 201, code = "journal_posted") {
  return { index, status, code, journalId, message: "SIM-1" };
}

const plain = (value) => JSON.parse(JSON.stringify(value));

test("buildTransferInput creates one balanced JPY transfer", () => {
  const input = scenarios.buildTransferInput(build);
  assert.equal(input.currency, "JPY");
  assert.equal(input.reference_id, "SIM-1");
  assert.deepEqual(plain(input.legs), [
    { account_id: "account-a", direction: "DEBIT", amount: "50000" },
    { account_id: "account-b", direction: "CREDIT", amount: "50000" },
  ]);
});

test("simulator posts the selected currency without changing minor units", () => {
  for (const currency of ["USD", "EUR"]) {
    const input = scenarios.buildTransferInput({ ...build, currency, amount: "1250" });
    assert.equal(input.currency, currency);
    assert.equal(input.legs[0].amount, "1250");
    assert.equal(input.legs[1].amount, "1250");
  }
});

test("tampered payload changes only the amount and stays within int64", () => {
  const original = scenarios.buildTransferInput(build);
  const tampered = scenarios.buildTransferInput(build, true);
  assert.equal(tampered.legs[0].amount, "50001");
  assert.equal(tampered.legs[1].amount, "50001");
  assert.equal(tampered.reference_id, original.reference_id);
  assert.equal(tampered.description, original.description);
  assert.equal(tampered.legs[0].account_id, original.legs[0].account_id);
  assert.equal(scenarios.tamperedAmount("1"), "2");
  assert.equal(scenarios.tamperedAmount("9223372036854775807"), "9223372036854775806");
});

test("failures are classified for the operator", () => {
  assert.match(scenarios.classifyFailure({ status: 422, code: "idempotency_key_reused", message: "reused" }), /Payload conflict/);
  assert.match(scenarios.classifyFailure({ status: 409, code: "idempotency_in_flight", message: "in flight" }), /In flight/);
  assert.match(scenarios.classifyFailure({ status: 403, code: "permission_denied", message: "denied" }), /transfers:create/);
  assert.match(scenarios.classifyFailure({ status: 0, code: "network_error", message: "offline" }), /Network error/);
  assert.match(scenarios.classifyFailure({ status: 503, code: "ledger_busy", message: "busy" }), /retrying the same key is safe/);
});

test("five replays with one journal ID summarize as one commit", () => {
  const summary = scenarios.summarizeAttempts(Array.from({ length: 5 }, (_, i) => attempt(i + 1, "journal-1")));
  assert.equal(summary.commits, 1);
  assert.equal(summary.replays, 4);
  assert.equal(summary.errors, 0);
  assert.deepEqual(plain(summary.journalIds), ["journal-1"]);
  assert.equal(summary.ok, true);
});

test("multiple journal IDs or failures are not a clean replay", () => {
  const summary = scenarios.summarizeAttempts([attempt(1, "journal-1"), attempt(2, "journal-2"), attempt(3, null, 422, "idempotency_key_reused")]);
  assert.equal(summary.commits, 2);
  assert.equal(summary.replays, 0);
  assert.equal(summary.errors, 1);
  assert.equal(summary.ok, false);
});
