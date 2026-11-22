import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { test } from "node:test";
import vm from "node:vm";
import ts from "typescript";

function load(path) {
  const source = readFileSync(new URL(path, import.meta.url), "utf8");
  const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 } });
  const exports = {};
  vm.runInNewContext(outputText, { exports, process: { env: {} } });
  return exports;
}

const docs = load("../src/lib/docs.ts");
const canonical = new URL("../../docs/user_guide.md", import.meta.url);
const generated = load("../src/content/user-guide.ts");
const guide = generated.USER_GUIDE_MARKDOWN;

test("web copy of the user guide matches docs/user_guide.md", { skip: !existsSync(canonical) }, () => {
  assert.equal(guide, readFileSync(canonical, "utf8"), "run `node scripts/sync-docs.mjs` in web/");
});

test("heading slugs match GitHub anchors used in the guide", () => {
  assert.equal(docs.headingSlug("1. Start and sign in"), "1-start-and-sign-in");
  assert.equal(docs.headingSlug("Staff engineer"), "staff-engineer");
  // Bangla: vowel signs and hasanta (combining marks) must survive, as on GitHub.
  assert.equal(docs.headingSlug("কে ব্যবহার করবেন (Who)"), "কে-ব্যবহার-করবেন-who");
  assert.equal(docs.headingSlug("১. চালু করা ও sign in"), "১-চালু-করা-ও-sign-in");
  assert.equal(docs.headingSlug("৪. PayCore Quantum: testnet-এ tUSD8 পেমেন্ট"), "৪-paycore-quantum-testnet-এ-tusd8-পেমেন্ট");
});

test("every in-page anchor in the guide points at a heading", () => {
  const markdown = guide;
  const headings = new Set([...markdown.matchAll(/^#{2,4} (.+)$/gm)].map(([, text = ""]) => docs.headingSlug(text)));
  const anchors = [...markdown.matchAll(/\]\((#[^)]+)\)/g)].map(([, href = ""]) => href.slice(1));
  assert.ok(anchors.length > 0);
  for (const anchor of anchors) assert.ok(headings.has(anchor), `missing heading for #${anchor}`);
});

test("doc links resolve safely", () => {
  const base = "https://github.com/mhannan-dev/PayCore/blob/PayCore-Quantum/";
  assert.deepEqual({ ...docs.resolveDocLink("#rules-that-protect-you") }, { href: "#rules-that-protect-you", external: false });
  assert.deepEqual({ ...docs.resolveDocLink("ledger.md") }, { href: `${base}docs/ledger.md`, external: true });
  assert.deepEqual({ ...docs.resolveDocLink("../README.md#getting-started") }, { href: `${base}README.md#getting-started`, external: true });
  assert.deepEqual({ ...docs.resolveDocLink("quantum/API.md") }, { href: `${base}docs/quantum/API.md`, external: true });
  assert.deepEqual({ ...docs.resolveDocLink("https://faucet.qday.info") }, { href: "https://faucet.qday.info", external: true });
  assert.deepEqual({ ...docs.resolveDocLink("javascript:alert(1)") }, { href: "#", external: false });
});
