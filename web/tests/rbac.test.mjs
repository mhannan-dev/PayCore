import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import vm from "node:vm";
import ts from "typescript";

// Load the pure permission module without adding a browser or test framework.
const source = readFileSync(new URL("../src/lib/rbac.ts", import.meta.url), "utf8");
const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS } });
const rbac = {};
vm.runInNewContext(outputText, { exports: rbac });

test("unknown grants deny protected controls", () => {
  assert.equal(rbac.hasPermission(null, "users:write"), false);
  assert.equal(rbac.hasPermission([], "users:write"), false);
  assert.equal(rbac.hasPermission(["USERS:WRITE"], "users:write"), false);
  assert.equal(rbac.hasPermission(["users:write"], "users:write"), true);
  assert.equal(rbac.hasPermission([""], ""), false);
});

test("nested pages inherit permissions without matching sibling prefixes", () => {
  assert.equal(rbac.canVisit([], "/admin/users/123"), false);
  assert.equal(rbac.canVisit(null, "/accounts/123/postings"), false);
  assert.equal(rbac.canVisit(["ledger:read"], "/accounts/123/postings"), true);
  assert.equal(rbac.permissionForRoute("/accounts-elsewhere"), undefined);
  assert.equal(rbac.canVisit(null, "/profile"), true);
});

test("reconciliation is permission protected", () => {
  assert.equal(rbac.canVisit(["ledger:read"], "/reconciliation"), false);
  assert.equal(rbac.canVisit(["reconciliation:read"], "/reconciliation"), true);
  assert.equal(rbac.canVisit(null, "/reconciliation/imports/123"), false);
  assert.equal(rbac.hasPermission(["reconciliation:read"], rbac.PERMISSIONS.reconciliationMatch), false);
  assert.equal(rbac.hasPermission(["reconciliation:match"], rbac.PERMISSIONS.reconciliationMatch), true);
});
