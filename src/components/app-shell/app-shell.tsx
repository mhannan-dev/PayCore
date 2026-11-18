"use client";

import { usePathname } from "next/navigation";
import { useState, type ReactNode } from "react";
import { useAuthSession } from "@/lib/auth-session";
import { Sidebar } from "./sidebar";
import { DemoBanner, PageFooter, Topbar } from "./topbar";
import { useSessionSync } from "./use-session-sync";

// Pages rendered without the workspace shell, even for a signed-in browser.
const PUBLIC_PATHS = new Set(["/login", "/set-password", "/forgot-password", "/reset-password"]);

export function AppShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const { session } = useAuthSession();
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  useSessionSync(session, pathname);

  // Auth guards live inside the pages so layouts never drop route segments.
  // /set-password is public: an invited user (or anyone signed in on this browser) sees just the form.
  if (PUBLIC_PATHS.has(pathname) || session.status === "loading" || !session.signedIn) return <>{children}</>;

  return (
    <div className={`app-shell${sidebarCollapsed ? " sidebar-collapsed" : ""}`}>
      <a className="skip-link" href="#main-content">Skip to content</a>
      <Sidebar pathname={pathname} userName={session.name} />
      <div className="main-shell">
        <Topbar pathname={pathname} sidebarCollapsed={sidebarCollapsed} onToggleSidebar={() => setSidebarCollapsed((value) => !value)} />
        <DemoBanner />
        <main id="main-content" tabIndex={-1}>{children}</main>
        <PageFooter />
      </div>
    </div>
  );
}
