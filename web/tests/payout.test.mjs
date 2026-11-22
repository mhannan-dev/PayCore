import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import vm from "node:vm";
import ts from "typescript";
import * as yup from "yup";

function load(path, dependencies = {}) {
  const source = readFileSync(new URL(path, import.meta.url), "utf8");
  const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 } });
  const exports = {};
  vm.runInNewContext(outputText, { exports, require(name) { if (!(name in dependencies)) throw Error(`Unexpected import ${name}`); return dependencies[name]; } });
  return exports;
}
const money = load("../src/lib/money.ts");
const payout = load("../src/components/workspace/settlements/payout-form-schema.ts", { yup, "@/lib/money": money });
const values = { source_account_id: "22222222-2222-4222-8222-222222222222", amount: "125.50", beneficiary_name: " Hana ", beneficiary_bank: " Bank ", beneficiary_account: " 00123456 " };

test("payout forms preserve minor units, beneficiary account zeros, and currency precision", () => {
  for (const currency of ["USD", "EUR"]) {
    const input = payout.buildPayoutRequest(values, currency);
    assert.equal(input.amount, "12550");
    assert.equal(input.currency, currency);
    assert.equal(input.beneficiary_name, "Hana");
    assert.equal(input.beneficiary_bank, "Bank");
    assert.equal(input.beneficiary_account, "00123456");
  }
  assert.equal(payout.buildPayoutRequest({ ...values, amount: "125" }, "JPY").amount, "125");
  assert.equal(payout.buildPayoutRequest({ ...values, amount: "92233720368547758.07" }, "USD").amount, "9223372036854775807");
});

test("payout form rejects zero, negatives, extra decimals, overflow, and incomplete beneficiaries", () => {
  for (const amount of ["0", "-1", "1.001", "1e3", "92233720368547758.08"]) assert.throws(() => payout.buildPayoutRequest({ ...values, amount }, "USD"));
  assert.throws(() => payout.buildPayoutRequest(values, "JPY"));
  for (const field of ["source_account_id", "beneficiary_name", "beneficiary_bank", "beneficiary_account"]) assert.throws(() => payout.buildPayoutRequest({ ...values, [field]: "" }, "USD"));
  assert.throws(() => payout.buildPayoutRequest({ ...values, beneficiary_account: "x".repeat(121) }, "USD"));
});

test("only active customer liability wallets in the selected currency fund payouts", () => {
  const wallet = { id: values.source_account_id, account_number: "2002", name: "Customer wallet", type: "LIABILITY", currency: "USD", is_active: true };
  assert.equal(payout.eligiblePayoutSource(wallet, "USD"), true);
  for (const patch of [{ currency: "JPY" }, { is_active: false }, { type: "ASSET" }, { account_number: "2003" }, { name: "USD Clearing" }]) assert.equal(payout.eligiblePayoutSource({ ...wallet, ...patch }, "USD"), false);
});
