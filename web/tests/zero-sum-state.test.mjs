import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import vm from "node:vm";
import ts from "typescript";

const source = readFileSync(new URL("../src/components/workspace/audit/zero-sum-state.ts", import.meta.url), "utf8");
const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 } });
const zeroSum = {};
vm.runInNewContext(outputText, { exports: zeroSum });

const checkedAt = "2026-11-21T12:00:00.000Z";
const now = Date.parse(checkedAt);
const report = (overrides = {}) => ({
  currency: "JPY", total_debits: "0", total_credits: "0", difference: "0",
  journals: 1, postings: 2, unbalanced_journals: [], balanced: true, checked_at: checkedAt, ...overrides,
});
const options = (overrides = {}) => ({ loading: false, error: "", forbidden: false, now, ...overrides });

test("a recent balanced report is live", () => {
  assert.equal(zeroSum.zeroSumState(report(), options()), "balanced");
  assert.equal(zeroSum.zeroSumLabel("balanced"), "Balanced (Δ = 0)");
  assert.equal(zeroSum.zeroSumTone("balanced"), "green");
});

test("a report older than 60 seconds is stale, not balanced", () => {
  assert.equal(zeroSum.zeroSumState(report(), options({ now: now + 60000 })), "balanced");
  assert.equal(zeroSum.zeroSumState(report(), options({ now: now + 60001 })), "stale");
  assert.equal(zeroSum.zeroSumTone("stale"), "amber");
});

test("failed, forbidden and malformed checks never show a green result", () => {
  assert.equal(zeroSum.zeroSumState(report(), options({ error: "API down" })), "unavailable");
  assert.equal(zeroSum.zeroSumState(report(), options({ forbidden: true })), "forbidden");
  assert.equal(zeroSum.zeroSumState(report({ checked_at: "not-a-date" }), options()), "unavailable");
  assert.equal(zeroSum.zeroSumState(null, options()), "unavailable");
  assert.equal(zeroSum.zeroSumState(null, options({ loading: true })), "loading");
});

test("unbalanced reports stay visible even while refreshing", () => {
  const bad = report({ balanced: false, difference: "5", unbalanced_journals: ["journal-1"] });
  assert.equal(zeroSum.zeroSumState(bad, options({ loading: true })), "unbalanced");
  assert.equal(zeroSum.zeroSumLabel("unbalanced"), "Not balanced");
});
