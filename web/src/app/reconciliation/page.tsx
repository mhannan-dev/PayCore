import { PageAccess } from "@/components/page-access";
import { Reconciliation } from "@/components/reconciliation/reconciliation";

export const metadata = { title: "Statement reconciliation" };
export default function Page() { return <PageAccess><Reconciliation /></PageAccess>; }
