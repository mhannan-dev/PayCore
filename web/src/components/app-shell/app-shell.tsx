"use client";

import { useWorkspaceCurrency } from "@/lib/workspace-currency";
import { usePathname } from "next/navigation";
import { Suspense, useState, type ReactNode } from "react";
import { useAuthSession } from "@/lib/auth-session";
import { Sidebar } from "./sidebar";
import { DemoBanner, PageFooter, Topbar } from "./topbar";
import { useSessionSync } from "./use-session-sync";

// Pages rendered without the workspace shell, even for a signed-in browser.
const PUBLIC_PATHS = new Set(["/login", "/set-password", "/forgot-password", "/reset-password"]);

export function AppShell({ children }: { children: ReactNode }) {
  // Dynamic checkout tokens make the pathname request-specific. Keep the route
  // content available while the shell waits for it during prerendering.
  return <Suspense fallback={children}><ResolvedAppShell>{children}</ResolvedAppShell></Suspense>;
}

function ResolvedAppShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const { currency } = useWorkspaceCurrency();
  const { session } = useAuthSession();
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  useSessionSync(session, pathname);

  // Auth guards live inside the pages so layouts never drop route segments.
  // /set-password is public: an invited user (or anyone signed in on this browser) sees just the form.
  const isPublic = PUBLIC_PATHS.has(pathname) || pathname === "/pay" || pathname.startsWith("/pay/");
  if (isPublic || session.status === "loading" || !session.signedIn) return <>{children}</>;

  return (
    <div className={`app-shell${sidebarCollapsed ? " sidebar-collapsed" : ""}`}>
      <a className="skip-link" href="#main-content">Skip to content</a>
      <Sidebar pathname={pathname} userName={session.name} />
      <div className="main-shell">
        <Topbar pathname={pathname} sidebarCollapsed={sidebarCollapsed} onToggleSidebar={() => setSidebarCollapsed((value) => !value)} />
        <DemoBanner />
        <main key={currency} id="main-content" tabIndex={-1}>{children}</main>
        <PageFooter />
      </div>
    </div>
  );
}
