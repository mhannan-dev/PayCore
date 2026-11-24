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

const rbac = load("../src/lib/rbac.ts");
const form = load("../src/components/admin/new-user-schema.ts", { yup, "@/lib/rbac": rbac });
const values = { firstName: "Member", lastName: "", email: "member@example.test", roles: ["ACCOUNTANT"] };

test("new users accept only the three current roles, including supported combinations", () => {
  for (const role of ["ADMIN", "ACCOUNTANT", "COMPLIANCE_AUDITOR"]) {
    assert.equal(Object.keys(form.validateNewUser({ ...values, roles: [role] })).length, 0);
  }
  assert.equal(Object.keys(form.validateNewUser({ ...values, roles: ["ACCOUNTANT", "COMPLIANCE_AUDITOR"] })).length, 0);
  for (const roles of [[], ["OPERATOR"], ["LEAD_OPERATOR"], ["STAFF_ENGINEER"], ["UNKNOWN"], ["ADMIN", "UNKNOWN"], ["ADMIN", "ADMIN"]]) {
    assert.equal(typeof form.validateNewUser({ ...values, roles }).roles, "string", JSON.stringify(roles));
  }
});
