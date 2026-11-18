"use client";

import type { ReactNode } from "react";
import { useAuthSession } from "@/lib/auth-session";
import { hasPermission, type Permission } from "@/lib/rbac";

/** Renders children only when the signed-in user has `permission` (otherwise `fallback`). */
export function Can({ permission, children, fallback = null }: { permission: Permission; children: ReactNode; fallback?: ReactNode }) {
  const { session } = useAuthSession();
  return <>{hasPermission(session.permissions, permission) ? children : fallback}</>;
}

/** Hook form of <Can>: returns a checker bound to the current session. */
export function usePermission() {
  const { session } = useAuthSession();
  return (permission: Permission) => hasPermission(session.permissions, permission);
}
