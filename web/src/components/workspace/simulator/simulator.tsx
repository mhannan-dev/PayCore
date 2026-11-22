import { Heading } from "../../ui";
import { IdempotencySandbox } from "./idempotency-sandbox";

export function Simulator() {
  return (
    <>
      <Heading eyebrow="LIVE LEDGER DEMO" title="One intent. One movement." description="Fire parallel transfers with one Idempotency-Key, then tamper with the payload and watch the real ledger reject it." />
      <IdempotencySandbox />
    </>
  );
}
