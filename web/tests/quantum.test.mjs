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
  vm.runInNewContext(outputText, { exports, URLSearchParams, require(name) { if (!(name in dependencies)) throw Error(`Unexpected import ${name}`); return dependencies[name]; } });
  return exports;
}

const amounts = load("../src/lib/quantum-amount.ts");
const labels = load("../src/lib/quantum-labels.ts");
const api = load("../src/lib/quantum-api.ts", { "./auth-api": { request: async () => ({}) } });
const invoiceForms = load("../src/components/quantum/invoice-schema.ts", { yup, "@/lib/quantum-amount": amounts });
const merchantForms = load("../src/components/quantum/merchant-schema.ts", { yup });

test("decimal input becomes exact integer base units without float math", () => {
  assert.equal(amounts.parseTokenAmount("12.5", 6).toString(), "12500000");
  assert.equal(amounts.parseTokenAmount(".5", 6).toString(), "500000");
  assert.equal(amounts.parseTokenAmount("1.", 6).toString(), "1000000");
  assert.equal(amounts.parseTokenAmount("00012.500000", 6).toString(), "12500000");
  assert.equal(amounts.parseTokenAmount("12", 0).toString(), "12");
  assert.equal(amounts.parseTokenAmount("9223372036854775807", 0).toString(), "9223372036854775807");
});

test("invalid, over-precise and overflowing amounts are rejected", () => {
  for (const input of ["0", "0.000000", "-1", "1.0000001", "1e3", "", "  ", "abc", "12,5", "9223372036854775808"]) {
    assert.throws(() => amounts.parseTokenAmount(input, 6), undefined, input);
  }
  assert.throws(() => amounts.parseTokenAmount("1", -1));
  assert.throws(() => amounts.parseTokenAmount("1", 19));
  assert.throws(() => amounts.parseTokenAmount("1", 1.5));
});

test("base-unit formatting and conversion stay string-based", () => {
  assert.equal(amounts.formatTokenAmount("12500000", 6), "12.5");
  assert.equal(amounts.formatTokenAmount("1", 6), "0.000001");
  assert.equal(amounts.formatTokenAmount("1000000", 6), "1");
  assert.equal(amounts.formatTokenAmount("0", 6), "0");
  assert.equal(amounts.formatTokenAmount("12", 0), "12");
  assert.equal(amounts.formatTokenAmount(BigInt("12500000"), 6), "12.5");
  for (const invalid of ["abc", "", "-1", "9223372036854775808"]) assert.equal(amounts.formatTokenAmount(invalid, 6), "—");
  assert.equal(amounts.baseUnitsToBigInt("12500000").toString(), "12500000");
  for (const invalid of ["abc", "0", "-1", "9223372036854775808"]) assert.throws(() => amounts.baseUnitsToBigInt(invalid));
});

test("every contract enum has a plain-language label", () => {
  assert.deepEqual([...api.QUANTUM_INVOICE_STATUSES], ["PENDING", "PAID", "EXPIRED", "CANCELLED"]);
  assert.deepEqual([...api.QUANTUM_ATTEMPT_STATUSES], ["CREATED", "BROADCAST", "VERIFYING", "CONFIRMED", "FAILED", "REORGED", "REVIEW_REQUIRED"]);
  assert.equal(api.QUANTUM_FAILURE_CODES.length, 12);
  for (const status of api.QUANTUM_INVOICE_STATUSES) {
    assert.equal(typeof labels.invoiceStatusLabel(status), "string");
    assert.match(labels.invoiceStatusLabel(status), /\S/);
    assert.ok(["green", "amber", "neutral"].includes(labels.invoiceStatusTone(status)));
  }
  for (const status of api.QUANTUM_ATTEMPT_STATUSES) {
    assert.equal(typeof labels.attemptStatusLabel(status), "string");
    assert.match(labels.attemptStatusLabel(status), /\S/);
    assert.ok(["green", "amber", "neutral"].includes(labels.attemptStatusTone(status)));
  }
  const messages = api.QUANTUM_FAILURE_CODES.map((code) => labels.failureCodeMessage(code));
  for (const message of messages) assert.match(message, /\S/);
  assert.equal(new Set(messages).size, messages.length);
  assert.match(labels.failureCodeMessage("underpaid"), /merchant/i);
  assert.match(labels.failureCodeMessage("wrong_chain"), /network/i);
});

test("explorer links escape the transaction hash", () => {
  assert.equal(api.explorerTxUrl("https://explorer.test/", "0xabc"), "https://explorer.test/tx/0xabc");
});

test("merchant setup normalizes addresses and mirrors the API character limit", () => {
  const receiver = `0x${"aB".repeat(20)}`;
  const merchant = merchantForms.buildMerchantRequest({ display_name: "  Test shop  ", receiver_address: ` ${receiver} ` });
  assert.equal(merchant.display_name, "Test shop");
  assert.equal(merchant.receiver_address, receiver.toLowerCase());
  // Go counts Unicode code points rather than UTF-16 code units.
  assert.equal(merchantForms.buildMerchantRequest({ display_name: "💳".repeat(120), receiver_address: receiver }).display_name, "💳".repeat(120));
  for (const display_name of ["", "  ", "a".repeat(121), "💳".repeat(121)]) {
    assert.throws(() => merchantForms.buildMerchantRequest({ display_name, receiver_address: receiver }));
  }
  for (const receiver_address of ["", "0x123", `0x${"g".repeat(40)}`, `0x${"0".repeat(40)}`]) {
    assert.throws(() => merchantForms.buildMerchantRequest({ display_name: "Shop", receiver_address }));
  }
});

test("invoice forms preserve precision and enforce amount, expiry and description limits", () => {
  const values = { amount: "12.5", description: "  Test invoice  ", expires_in_minutes: 30 };
  const input = invoiceForms.buildInvoiceRequest(values, 6);
  assert.equal(input.amount_base_units, "12500000");
  assert.equal(input.description, "Test invoice");
  assert.equal(input.expires_in_minutes, 30);
  for (const expires_in_minutes of [5, 1440]) {
    assert.equal(invoiceForms.buildInvoiceRequest({ ...values, expires_in_minutes }, 6).expires_in_minutes, expires_in_minutes);
  }
  assert.equal(invoiceForms.buildInvoiceRequest({ ...values, description: "💳".repeat(500) }, 6).description, "💳".repeat(500));
  assert.equal(invoiceForms.buildInvoiceRequest({ ...values, amount: "9223372036854.775807" }, 6).amount_base_units, "9223372036854775807");
  for (const patch of [
    { amount: "0" }, { amount: "-1" }, { amount: "1.0000001" }, { amount: "9223372036854.775808" },
    { description: "  " }, { description: "a".repeat(501) }, { description: "💳".repeat(501) },
    { expires_in_minutes: 4 }, { expires_in_minutes: 1441 }, { expires_in_minutes: 5.5 }, { expires_in_minutes: NaN },
  ]) {
    assert.throws(() => invoiceForms.buildInvoiceRequest({ ...values, ...patch }, 6));
  }
});

test("invoice requests retain the supplied retry key and encode filters and IDs", async () => {
  const calls = [];
  const client = load("../src/lib/quantum-api.ts", { "./auth-api": { request: async (path, options) => { calls.push({ path, options }); return {}; } } });
  const input = { amount_base_units: "12500000", description: "Invoice", expires_in_minutes: 30 };
  await client.createInvoice("session-token", "same-intent-key", input);
  await client.createInvoice("session-token", "same-intent-key", input);
  assert.equal(calls.length, 2);
  for (const { path, options } of calls) {
    assert.equal(path, "/quantum/invoices");
    assert.equal(options.method, "POST");
    assert.equal(options.token, "session-token");
    assert.equal(options.headers["Idempotency-Key"], "same-intent-key");
    assert.deepEqual(JSON.parse(options.body), input);
  }
  await client.fetchInvoices("session-token", { status: "PAID", limit: 25, offset: 50 });
  assert.equal(calls.at(-1).path, "/quantum/invoices?status=PAID&limit=25&offset=50");
  await client.fetchInvoice("session-token", "untrusted/id");
  assert.equal(calls.at(-1).path, "/quantum/invoices/untrusted%2Fid");
  await client.cancelInvoice("session-token", "untrusted/id");
  assert.equal(calls.at(-1).path, "/quantum/invoices/untrusted%2Fid/cancel");
  assert.equal(calls.at(-1).options.method, "POST");
});
