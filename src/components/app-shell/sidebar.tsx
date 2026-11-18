"use client";

import Link from "next/link";
import { useState } from "react";
import { UserAvatar } from "../user-avatar";
import { Icon } from "../icons";
import { useAuthSession } from "@/lib/auth-session";
import { canVisit } from "@/lib/rbac";
import { NAVIGATION, type NavItem } from "./navigation";
import "./sidebar.css";

function NavItemLink({ item, pathname, onNavigate }: { item: NavItem; pathname: string; onNavigate: () => void }) {
  const { href, label, icon } = item;
  const active = pathname === href;
  return (
    <Link href={href} className={`nav-link ${active ? "active" : ""}`} aria-label={label} title={label} aria-current={active ? "page" : undefined} onClick={onNavigate}>
      <Icon name={icon} />
      <span className="nav-label">{label}</span>
      {active && <span className="active-dot" />}
    </Link>
  );
}

function NavLinks({ pathname, onNavigate }: { pathname: string; onNavigate: () => void }) {
  const { session } = useAuthSession();
  // Only list pages the user's roles grant; PageAccess still guards direct visits.
  const visible = NAVIGATION.filter((item) => canVisit(session.permissions, item.href));
  const workspace = visible.filter((item) => !item.section);
  const admin = visible.filter((item) => item.section === "admin");
  return (
    <nav id="primary-navigation" aria-label="Main navigation">
      <p className="nav-heading">WORKSPACE</p>
      {workspace.map((item) => <NavItemLink key={item.href} item={item} pathname={pathname} onNavigate={onNavigate} />)}
      {admin.length > 0 && <p className="nav-heading nav-heading-section">ADMINISTRATION</p>}
      {admin.map((item) => <NavItemLink key={item.href} item={item} pathname={pathname} onNavigate={onNavigate} />)}
    </nav>
  );
}

function SidebarFooter({ name }: { name: string }) {
  return (
    <div className="sidebar-footer">
      <Link href="/profile" className="operator" aria-label={`View profile: ${name}`} title={name}>
        <span className="operator-avatar"><UserAvatar /></span>
        <span className="operator-details">{name}<small>View user profile</small></span>
      </Link>
    </div>
  );
}

export function Sidebar({ pathname, userName }: { pathname: string; userName: string }) {
  const [menuOpen, setMenuOpen] = useState(false);
  const close = () => setMenuOpen(false);
  return (
    <aside id="workspace-sidebar" className={`sidebar ${menuOpen ? "menu-open" : ""}`}>
      <Link href="/" className="brand" aria-label="PayCore overview" title="PayCore" onClick={close}>
        <span className="brand-mark">p<span>·</span></span><span className="brand-name">PayCore</span><span className="brand-tag">CONSOLE</span>
      </Link>
      <button type="button" className="workspace-switch" title="PayCore workspace" aria-label="Toggle workspace navigation" onClick={() => setMenuOpen(!menuOpen)} aria-expanded={menuOpen} aria-controls="primary-navigation">
        <span className="workspace-avatar">P</span>
        <span className="workspace-details">PayCore workspace<small>Development environment</small></span>
        <span className="menu-symbol">☰</span>
      </button>
      <NavLinks pathname={pathname} onNavigate={close} />
      <SidebarFooter name={userName} />
    </aside>
  );
}
