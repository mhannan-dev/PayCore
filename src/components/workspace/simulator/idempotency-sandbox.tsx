"use client";

import { Icon } from "../../icons";

import { useRef, useState } from "react";
import { validAmount } from "@/lib/demo-data";
import { Badge, EmptyHint, Panel } from "../../ui";

function SimulationResults({ results, commits }: { results: string[]; commits: number }) {
  return (
    <Panel title="Request results" action={<Badge tone="neutral">{commits} simulated commits</Badge>}>
      {results.length ? (
        <div className="simulation-results" role="status">
          {results.map((result, i) => <p key={i}><code>{result}</code></p>)}
        </div>
      ) : (
        <EmptyHint icon="code" title="Make retries predictable." text="Run a scenario to see matching replays or payload conflicts." />
      )}
    </Panel>
  );
}

export function IdempotencySandbox() {
  const [amount, setAmount] = useState("50000");
  const [key, setKey] = useState("");
  const [results, setResults] = useState<string[]>([]);
  const [commits, setCommits] = useState(0);
  const cache = useRef(new Map<string, string>());

  function run(tamper: boolean) {
    if (!validAmount(amount)) return setResults(["Enter a positive whole-yen amount within int64 limits."]);
    const activeKey = key || crypto.randomUUID();
    setKey(activeKey);
    const prior = cache.current.get(activeKey);
    if (tamper && !prior) return setResults(["Run the duplicate scenario first, then test a changed payload."]);
    const payload = tamper ? `${amount}:changed` : amount;
    if (prior && prior !== payload) return setResults(["422 · Changed payload for the same key. No simulated commit."]);
    if (!prior) {
      cache.current.set(activeKey, payload);
      setCommits((n) => n + 1);
    }
    setResults(
      Array.from({ length: 5 }, (_, i) => `Request ${i + 1} · 200 · ${!prior && i === 0 ? "One simulated commit" : "Stored response replay"} · same result ID`),
    );
  }

  function reset() {
    cache.current.clear();
    setKey("");
    setResults([]);
    setCommits(0);
  }

  return (
    <div className="form-grid">
      <Panel title="Idempotency sandbox" description="Local in-memory illustration, not a backend concurrency test.">
        <div className="form-body">
          <label>Sample amount · JPY<input value={amount} inputMode="numeric" onChange={(e) => setAmount(e.target.value)} /></label>
          <label>Active key<code className="key-display">{key || "Generated on first run"}</code></label>
          <div className="button-stack">
            <button className="button primary" onClick={() => run(false)}><Icon name="play" size={16} />Simulate 5 duplicate requests</button>
            <button className="button secondary" onClick={() => run(true)}><Icon name="edit" size={16} />Test changed payload</button>
            <button className="text-link" onClick={reset}><Icon name="refresh" size={14} />Reset simulation</button>
          </div>
          <p className="footnote">Duplicates are processed locally in sequence. Real network and database concurrency require backend integration tests.</p>
        </div>
      </Panel>
      <SimulationResults results={results} commits={commits} />
    </div>
  );
}
