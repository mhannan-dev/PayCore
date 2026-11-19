import { Heading } from "../../ui";
import { TransferStudio } from "./transfer-studio";

export function Transfers() {
  return (
    <>
      <Heading eyebrow="LEDGER CORE" title="Transfer studio" description="Post a balanced transfer between two accounts. Funds are checked under lock; retries never post twice." />
      <TransferStudio />
    </>
  );
}
