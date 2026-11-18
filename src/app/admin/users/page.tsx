import { UsersAdmin } from "@/components/admin";
import { PageAccess } from "@/components/page-access";
export const metadata = { title: "Users & access" };
export default function Page() { return <PageAccess><UsersAdmin /></PageAccess>; }
