import Link from "next/link";
import { Icon } from "./icons";
import "./admin/admin.css";

/** Shown in place of a page the signed-in user's roles do not grant. */
export function AccessDenied({ permission }: { permission: string }) {
  return (
    <section className="access-denied" role="alert">
      <span className="access-denied-icon"><Icon name="lock" size={28} /></span>
      <p className="eyebrow">ACCESS RESTRICTED</p>
      <h1>You don&apos;t have access to this page</h1>
      <p>
        It needs the <code>{permission}</code> permission, which none of your roles grant.
        Ask a PayCore administrator if you need access.
      </p>
      <div className="access-denied-actions">
        <Link href="/" className="button primary">Back to overview</Link>
        <Link href="/profile" className="button secondary">View my roles</Link>
      </div>
    </section>
  );
}
