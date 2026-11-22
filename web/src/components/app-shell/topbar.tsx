import { useWorkspaceCurrency } from "@/lib/workspace-currency";
import { CURRENCIES } from "@/lib/ledger-api";
import Link from "next/link";
import { UserAvatar } from "../user-avatar";
import { Icon } from "../icons";
import { LogoutButton } from "../logout-button";
import { pageLabel } from "./navigation";

export function Topbar({ pathname, sidebarCollapsed = false, onToggleSidebar }: { pathname: string; sidebarCollapsed?: boolean; onToggleSidebar?: () => void }) {
  const { currency, setCurrency } = useWorkspaceCurrency();
  return (
    <header className="topbar">
      <div className="topbar-start">
        {onToggleSidebar && <button type="button" className="sidebar-toggle" aria-label={sidebarCollapsed ? "Expand sidebar" : "Collapse sidebar"} title={sidebarCollapsed ? "Expand sidebar" : "Collapse sidebar"} aria-expanded={!sidebarCollapsed} aria-controls="workspace-sidebar" onClick={onToggleSidebar}>
          <svg width="19" height="19" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><rect x="3" y="4" width="18" height="16" rx="3" /><path d="M9 4v16" /><path d={sidebarCollapsed ? "m13 9 3 3-3 3" : "m16 9-3 3 3 3"} /></svg>
        </button>}
        <div className="breadcrumb">Workspace <span>/</span> <strong>{pageLabel(pathname)}</strong></div>
      </div>
      <div className="topbar-right">
        <label className="currency-switcher" title="Show ledger data in this currency. Amounts are not converted.">
          <span className="sr-only">Workspace currency</span>
          <select aria-label="Workspace currency" value={currency} onChange={(event) => {
            const selected = CURRENCIES.find((code) => code === event.target.value);
            if (selected) setCurrency(selected);
          }}>
            {CURRENCIES.map((code) => <option key={code} value={code}>{code}</option>)}
          </select>
        </label>
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
      <span className="demo-label">SANDBOX</span> Workspace pages read the live API. Payouts are sandbox ledger operations; notification email and webhook receipts are captured locally.
    </div>
  );
}

export function PageFooter() {
  const { currency } = useWorkspaceCurrency();
  return (
    <footer className="page-footer">
      <span>PayCore / Financial operations</span>
      <span>{currency} · Sample date: 15 Nov 2026</span>
    </footer>
  );
}
