export type NavItem = { href: string; label: string; icon: string; section?: "admin" };

export const NAVIGATION: NavItem[] = [
  { href: "/", label: "Overview", icon: "grid" },
  { href: "/accounts", label: "Accounts", icon: "wallet" },
  { href: "/transfers", label: "Transfer studio", icon: "arrows" },
  { href: "/reconciliation", label: "Reconciliation", icon: "file" },
  { href: "/ledger", label: "Ledger explorer", icon: "book" },
  { href: "/audit", label: "Invariant audit", icon: "shield" },
  { href: "/reports", label: "Reports", icon: "chart" },
  { href: "/notifications", label: "Notifications", icon: "bell" },
  { href: "/settlements", label: "Settlements", icon: "bank" },
  { href: "/quantum", label: "Quantum Payments", icon: "wallet" },
  { href: "/simulator", label: "Request simulator", icon: "code" },
  { href: "/security", label: "Account security", icon: "shield" },
  { href: "/admin/users", label: "Users & access", icon: "users", section: "admin" },
  { href: "/admin/roles", label: "Roles & permissions", icon: "key", section: "admin" },
  { href: "/admin/audit-log", label: "Audit log", icon: "file", section: "admin" },
  { href: "/system/outbox", label: "Event outbox", icon: "refresh", section: "admin" },
];

const EXTRA_PAGE_LABELS: Record<string, string> = {
  "/profile": "User profile",
};

export const pageLabel = (pathname: string) =>
  EXTRA_PAGE_LABELS[pathname] ?? NAVIGATION.find((item) => item.href === pathname)?.label ?? "Page";
