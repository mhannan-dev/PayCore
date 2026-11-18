import { Heading } from "../../ui";
import { IdempotencySandbox } from "./idempotency-sandbox";

export function Simulator() {
  return (
    <>
      <Heading eyebrow="LOCAL DEMONSTRATION" title="One intent. One movement." description="Explore request deduplication before connecting the backend API." />
      <IdempotencySandbox />
    </>
  );
}
