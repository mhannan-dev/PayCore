"use client";

import { useEffect, useState } from "react";
import { AuthApiError } from "@/lib/auth-api";
import { useAuthSession, validateSession } from "@/lib/auth-session";

export type AdminState<T> =
  | { status: "loading" }
  | { status: "error"; message: string; forbidden: boolean }
  | { status: "ready"; data: T };

/**
 * Loads admin data with the session token. `loader` must be stable (define it at
 * module level). Returns the state plus `reload` and an in-place `update`.
 */
export function useAdminData<T>(loader: (token: string) => Promise<T>) {
  const { session } = useAuthSession();
  const token = session.token;
  const [state, setState] = useState<AdminState<T>>({ status: "loading" });
  const [version, setVersion] = useState(0);

  useEffect(() => {
    if (!token) return;
    let cancelled = false;
    loader(token).then(
      (data) => { if (!cancelled) setState({ status: "ready", data }); },
      (error: unknown) => {
        if (cancelled) return;
        // 401: the session is gone; re-validating clears it and shows the login screen.
        if (error instanceof AuthApiError && error.status === 401) void validateSession();
        setState({
          status: "error",
          message: error instanceof AuthApiError ? error.message : "Could not load access data. Try again.",
          forbidden: error instanceof AuthApiError && error.status === 403,
        });
      },
    );
    return () => { cancelled = true; };
  }, [loader, token, version]);

  return {
    state,
    reload: () => { setState({ status: "loading" }); setVersion((v) => v + 1); },
    update: (change: (data: T) => T) => setState((s) => (s.status === "ready" ? { status: "ready", data: change(s.data) } : s)),
  };
}
