"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { validateSession, type Session } from "@/lib/auth-session";

/** Re-checks the token with the API on load and tab focus; sends signed-in users away from /login. */
export function useSessionSync(session: Session, pathname: string) {
  const router = useRouter();

  useEffect(() => {
    if (!session.token) return;
    void validateSession();
    const onFocus = () => { void validateSession(); };
    window.addEventListener("focus", onFocus);
    return () => window.removeEventListener("focus", onFocus);
  }, [session.token]);

  useEffect(() => {
    if (pathname === "/login" && session.signedIn) router.replace("/");
  }, [pathname, session.signedIn, router]);
}
