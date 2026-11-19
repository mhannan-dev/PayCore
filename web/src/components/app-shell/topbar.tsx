import Link from "next/link";
import { UserAvatar } from "../user-avatar";
import { Icon } from "../icons";
import { LogoutButton } from "../logout-button";
import { pageLabel } from "./navigation";

export function Topbar({ pathname, sidebarCollapsed = false, onToggleSidebar }: { pathname: string; sidebarCollapsed?: boolean; onToggleSidebar?: () => void }) {
  return (
    <header className="topbar">
      <div className="topbar-start">
        {onToggleSidebar && <button type="button" className="sidebar-toggle" aria-label={sidebarCollapsed ? "Expand sidebar" : "Collapse sidebar"} title={sidebarCollapsed ? "Expand sidebar" : "Collapse sidebar"} aria-expanded={!sidebarCollapsed} aria-controls="workspace-sidebar" onClick={onToggleSidebar}>
          <svg width="19" height="19" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><rect x="3" y="4" width="18" height="16" rx="3" /><path d="M9 4v16" /><path d={sidebarCollapsed ? "m13 9 3 3-3 3" : "m16 9-3 3 3 3"} /></svg>
        </button>}
        <div className="breadcrumb">Workspace <span>/</span> <strong>{pageLabel(pathname)}</strong></div>
      </div>
      <div className="topbar-right">
        <span className="environment-chip"><span className="status-dot" />Sandbox preview</span>
        <Link href="/notifications" aria-label="Open notifications" className="icon-button"><Icon name="bell" /></Link>
        <Link href="/profile" aria-label="Open user profile" className="operator-avatar"><UserAvatar /></Link>
        <LogoutButton className="logout-button" />
      </div>
    </header>
  );
}

export function DemoBanner() {
  return (
    <div className="demo-banner">
      <span className="demo-label">PARTLY LIVE</span> Accounts, Transfer studio, Ledger explorer and Invariant audit use the live ledger. Overview, Reports, Notifications, Settlements and Simulator still show sample data.
    </div>
  );
}

export function PageFooter() {
  return (
    <footer className="page-footer">
      <span>PayCore / Financial operations</span>
      <span>JPY · Sample date: 15 Nov 2026</span>
    </footer>
  );
}
