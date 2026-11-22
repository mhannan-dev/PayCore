import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import vm from "node:vm";
import ts from "typescript";
const source = readFileSync(new URL("../src/components/workspace/notifications/delivery-state.ts", import.meta.url), "utf8");
const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS } });
const state = {};
vm.runInNewContext(outputText, { exports: state });
const pending = { status: "PENDING", dispatch_status: "PENDING", next_attempt_at: null, attempts: 0, can_retry: false };
test("notification labels distinguish sandbox delivery from outbox publication", () => {
  assert.equal(state.deliveryLabel(pending), "Pending");
  assert.equal(state.deliveryLabel({ ...pending, dispatch_status: "PUBLISHED" }), "Pending");
  assert.equal(state.deliveryLabel({ ...pending, dispatch_status: "SENDING" }), "Processing");
  assert.equal(state.deliveryLabel({ ...pending, status: "DELIVERED", dispatch_status: "FAILED" }), "Delivered to sandbox");
  assert.equal(state.deliveryLabel({ ...pending, status: "FAILED", attempts: 1, next_attempt_at: "2026-11-21T15:00:00Z" }), "Retry scheduled");
});
test("manual retries require a failed delivery and terminal outbox failure", () => {
  const failed = { ...pending, status: "FAILED", dispatch_status: "FAILED", attempts: 5, can_retry: true };
  assert.equal(state.canRetryDelivery(failed), true);
  for (const patch of [{ status: "DELIVERED" }, { dispatch_status: "SENDING" }, { dispatch_status: "PENDING" }, { can_retry: false }]) assert.equal(state.canRetryDelivery({ ...failed, ...patch }), false);
});
