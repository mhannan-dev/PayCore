import { PageAccess } from "@/components/page-access";
import { Audit } from "@/components/workspace";
export const metadata = { title: "Invariant audit" };
export default function Page() { return <PageAccess><Audit /></PageAccess>; }
