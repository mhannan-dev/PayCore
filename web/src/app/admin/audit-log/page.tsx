import { AuditLog } from "@/components/admin";
import { PageAccess } from "@/components/page-access";
export const metadata = { title: "Audit log" };
export default function Page() { return <PageAccess><AuditLog /></PageAccess>; }
