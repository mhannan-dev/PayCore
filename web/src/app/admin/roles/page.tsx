import { RolesAdmin } from "@/components/admin";
import { PageAccess } from "@/components/page-access";
export const metadata = { title: "Roles & permissions" };
export default function Page() { return <PageAccess><RolesAdmin /></PageAccess>; }
