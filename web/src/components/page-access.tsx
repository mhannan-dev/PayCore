"use client";

import { usePathname } from "next/navigation";
import type { ReactNode } from "react";
import { useAuthSession } from "@/lib/auth-session";
import { hasPermission, permissionForRoute } from "@/lib/rbac";
import { AccessDenied } from "./access-denied";
import { LoginPage } from "./login-page";

// The route segment itself must render before deciding which client UI to show.
// API endpoints remain responsible for enforcing authentication/authorization.
// `permission` defaults to the route's entry in ROUTE_PERMISSIONS.
export function PageAccess({ children, permission }: { children: ReactNode; permission?: string }) {
  const { session } = useAuthSession();
  const pathname = usePathname();
  // The GlobalPreloader covers the screen (and announces loading) while the session resolves.
  if (session.status === "loading") return <div className="auth-loading" aria-hidden="true" />;
  if (!session.signedIn) return <LoginPage />;
  const required = permission ?? permissionForRoute(pathname);
  // hasPermission allows an undefined requirement, so a denial always names one.
  if (required && !hasPermission(session.permissions, required)) return <AccessDenied permission={required} />;
  return <>{children}</>;
}
