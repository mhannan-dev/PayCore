import { Heading } from "../../ui";
import { TransferStudio } from "./transfer-studio";

export function Transfers() {
  return (
    <>
      <Heading eyebrow="LEDGER CORE" title="Transfer studio" description="Build a balanced transfer and inspect its request before API integration." />
      <TransferStudio />
    </>
  );
}
