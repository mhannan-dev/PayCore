import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import vm from "node:vm";
import ts from "typescript";
import * as viem from "viem";

function load(path, dependencies = {}) {
  const source = readFileSync(new URL(path, import.meta.url), "utf8");
  const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 } });
  const exports = {};
  vm.runInNewContext(outputText, { exports, require(name) { if (!(name in dependencies)) throw Error(`Unexpected import ${name}`); return dependencies[name]; } });
  return exports;
}
const amounts = load("../src/lib/quantum-amount.ts");
const { sendCheckoutPayment } = load("../src/lib/quantum-payment.ts", { viem, "./quantum-amount": amounts });
const contract = "0xdddddddddddddddddddddddddddddddddddddddd";
const orderId = "0x797307cb894f69cc35dd9635045e83fb9989baaf0c6a900c0a0caf03324a0634";
const checkout = { token_address: "0xcccccccccccccccccccccccccccccccccccccccc", receiver_address: "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", amount_base_units: "1250000", payment_contract_address: contract, order_id: orderId };
const approvalHash = `0x${"1".repeat(64)}`;
const paymentHash = `0x${"2".repeat(64)}`;

function fixture({ allowance = 0n, approvalFails = false, rejected = false } = {}) {
  const calls = [];
  let approved = false;
  const actions = {
    allowance: async () => { calls.push("allowance"); return approved ? 1250000n : allowance; },
    approve: async (payment) => { calls.push("approve"); assert.equal(payment.contract, contract); assert.equal(payment.amount, 1250000n); if (rejected) throw Error("rejected"); return approvalHash; },
    waitForApproval: async (hash) => { calls.push("waitApproval"); assert.equal(hash, approvalHash); if (approvalFails) throw Error("approval reverted"); approved = true; },
    register: async () => { calls.push("register"); return "attempt-1"; },
    pay: async (payment) => { calls.push("pay"); assert.equal(payment.orderId, orderId); assert.equal(payment.merchant, checkout.receiver_address); assert.equal(payment.amount, 1250000n); return paymentHash; },
    transfer: async () => { calls.push("transfer"); return paymentHash; },
  };
  return { actions, calls };
}

test("contract checkout waits for exact approval before registering and paying", async () => {
  const { actions, calls } = fixture();
  const result = await sendCheckoutPayment(checkout, actions, () => {});
  assert.deepEqual(calls, ["allowance", "approve", "waitApproval", "allowance", "register", "pay"]);
  assert.equal(result.hash, paymentHash);
  assert.equal(result.attemptId, "attempt-1");
});

test("existing allowance skips approval and legacy invoices still transfer directly", async () => {
  const ready = fixture({ allowance: 1250000n });
  await sendCheckoutPayment(checkout, ready.actions, () => {});
  assert.deepEqual(ready.calls, ["allowance", "register", "pay"]);
  const legacy = fixture();
  await sendCheckoutPayment({ ...checkout, payment_contract_address: null }, legacy.actions, () => {});
  assert.deepEqual(legacy.calls, ["register", "transfer"]);
});

test("rejected or reverted approval never registers or broadcasts payment", async () => {
  for (const options of [{ approvalFails: true }, { rejected: true }]) {
    const { actions, calls } = fixture(options);
    await assert.rejects(sendCheckoutPayment(checkout, actions, () => {}));
    assert.ok(!calls.includes("register"));
    assert.ok(!calls.includes("pay"));
  }
});

test("insufficient confirmed allowance and malformed contract details fail closed", async () => {
  const insufficient = fixture();
  insufficient.actions.allowance = async () => 0n;
  await assert.rejects(sendCheckoutPayment(checkout, insufficient.actions, () => {}), /insufficient/);
  assert.ok(!insufficient.calls.includes("pay"));
  for (const change of [{ order_id: "0x1234" }, { payment_contract_address: "bad" }, { token_address: "bad" }]) {
    const { actions, calls } = fixture();
    await assert.rejects(sendCheckoutPayment({ ...checkout, ...change }, actions, () => {}), /invalid/);
    assert.deepEqual(calls, []);
  }
});

test("attempt registration failure never broadcasts payment", async () => {
  const { actions, calls } = fixture({ allowance: 1250000n });
  actions.register = async () => { throw Error("invoice expired"); };
  await assert.rejects(sendCheckoutPayment(checkout, actions, () => {}), /expired/);
  assert.ok(!calls.includes("pay"));
});
