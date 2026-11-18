import { PageAccess } from "@/components/page-access";
import { UserProfile } from "@/components/user-profile";
export const metadata = { title: "User profile" };
export default function Page() { return <PageAccess><UserProfile /></PageAccess>; }
