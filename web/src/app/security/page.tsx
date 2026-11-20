import { PageAccess } from "@/components/page-access";
import { AccountSecurity } from "@/components/security/account-security";

export const metadata = { title: "Account security" };
export default function Page() { return <PageAccess><AccountSecurity /></PageAccess>; }
