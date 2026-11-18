"use client";

import { useEffect, useState } from "react";
import { AuthApiError, fetchUsers, type ManagedUser, type Page } from "@/lib/auth-api";
import { useAuthSession, validateSession } from "@/lib/auth-session";

export type UsersPageQuery = { q: string; role: string; page: number; pageSize: number };

type Result = { key: string; data?: Page<ManagedUser>; error?: string };

/**
 * Loads one page of users for the query. The previous page stays visible while
 * the next one loads; `loading` says a request is in flight.
 */
export function useUsersPage(query: UsersPageQuery) {
  const { session } = useAuthSession();
  const token = session.token;
  const [version, setVersion] = useState(0);
  const [result, setResult] = useState<Result | null>(null);
  // Edits applied on top of the loaded page (role changes, re-sent invites).
  const [patches, setPatches] = useState<Record<string, ManagedUser>>({});
  const { q, role, page, pageSize } = query;
  const key = JSON.stringify([q, role, page, pageSize, version]);

  useEffect(() => {
    if (!token) return;
    let cancelled = false;
    fetchUsers(token, { q, role: role === "all" ? "" : role, limit: pageSize, offset: page * pageSize }).then(
      (data) => { if (!cancelled) { setPatches({}); setResult({ key, data }); } },
      (error: unknown) => {
        if (cancelled) return;
        if (error instanceof AuthApiError && error.status === 401) void validateSession();
        const message = error instanceof AuthApiError ? error.message : "Could not load users. Try again.";
        setResult((previous) => ({ key, data: previous?.data, error: message }));
      },
    );
    return () => { cancelled = true; };
  }, [token, key, q, role, page, pageSize]);

  const fresh = result?.key === key;
  // The previous page stays on screen while the next loads, and after a failed load.
  const data = result?.data ?? null;

  return {
    users: data ? data.items.map((u) => patches[u.id] ?? u) : [],
    total: data?.total ?? 0,
    loaded: data !== null,
    loading: !fresh,
    error: fresh ? result?.error ?? "" : "",
    reload: () => setVersion((v) => v + 1),
    patch: (user: ManagedUser) => setPatches((current) => ({ ...current, [user.id]: user })),
  };
}
