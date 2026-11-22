import { PageAccess } from "@/components/page-access";
import { Outbox } from "@/components/workspace";
export const metadata = { title: "Event outbox" };
export default function Page() { return <PageAccess><Outbox /></PageAccess>; }
