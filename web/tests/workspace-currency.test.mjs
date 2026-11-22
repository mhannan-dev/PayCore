import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import vm from "node:vm";
import ts from "typescript";

const source = readFileSync(new URL("../src/lib/workspace-currency.ts", import.meta.url), "utf8");
const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS } });

function store(initial = null, blocked = false) {
  const events = new EventTarget();
  let raw = initial;
  let readBlocked = blocked;
  let writeBlocked = blocked;
  let snapshot;
  let serverSnapshot;
  let changes = 0;
  let unsubscribe;
  const exports = {};
  vm.runInNewContext(outputText, {
    exports,
    Event,
    window: {
      localStorage: {
        getItem() { if (readBlocked) throw Error("blocked"); return raw; },
        setItem(_key, value) { if (writeBlocked) throw Error("blocked"); raw = value; },
      },
      addEventListener: events.addEventListener.bind(events),
      removeEventListener: events.removeEventListener.bind(events),
      dispatchEvent: events.dispatchEvent.bind(events),
    },
    require(name) {
      if (name === "./ledger-api") return { CURRENCIES: ["JPY", "USD", "EUR"] };
      if (name === "react") return { useSyncExternalStore(subscribe, get, server) {
        snapshot = get;
        serverSnapshot = server;
        unsubscribe = subscribe(() => { changes += 1; });
        return get();
      } };
      throw Error(`Unexpected import ${name}`);
    },
  });
  const hook = exports.useWorkspaceCurrency();
  return {
    exports, hook,
    read: () => snapshot(),
    server: () => serverSnapshot(),
    changes: () => changes,
    stop: () => unsubscribe(),
    failWrites() { writeBlocked = true; readBlocked = false; },
    external(value) { raw = value; events.dispatchEvent(new Event("storage")); },
  };
}

test("saved currency is validated, with a stable server default", () => {
  for (const currency of ["JPY", "USD", "EUR"]) {
    const s = store(currency);
    assert.equal(s.hook.currency, currency);
    assert.equal(s.server(), "JPY");
    s.stop();
  }
  for (const value of [null, "GBP", "usd", "", '{"currency":"EUR"}']) {
    assert.equal(store(value).read(), "JPY");
  }
  assert.equal(store().exports.parseCurrency({ currency: "EUR" }), null);
});

test("switches notify subscribers and follow changes from other tabs", () => {
  const s = store();
  s.hook.setCurrency("USD");
  assert.equal(s.read(), "USD");
  assert.equal(s.changes(), 1);
  s.external("EUR");
  assert.equal(s.read(), "EUR");
  s.external(null);
  assert.equal(s.read(), "JPY");
  s.stop();
  s.hook.setCurrency("USD");
  assert.equal(s.changes(), 3);
});

test("currency switching still works when storage is unavailable or read only", () => {
  const s = store(null, true);
  s.hook.setCurrency("EUR");
  assert.equal(s.read(), "EUR");
  const readOnly = store("USD");
  readOnly.failWrites();
  readOnly.hook.setCurrency("JPY");
  assert.equal(readOnly.read(), "JPY");
});
