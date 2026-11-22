import { PageAccess } from "@/components/page-access";
import { QuantumConsole } from "@/components/quantum/quantum-console";

export const metadata = { title: "Quantum payments" };

export default function Page() {
  return <PageAccess><QuantumConsole /></PageAccess>;
}
