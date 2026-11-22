"use client";

import { useEffect, useState } from "react";
import { AuthApiError } from "@/lib/auth-api";
import { useAuthSession, validateSession } from "@/lib/auth-session";

type Result<T> = { key: string; data?: T; error?: string; status?: number };

/**
 * Loads data with the session token whenever `key` changes (pass a key that
 * encodes the query, e.g. JSON of the filters). The previous data stays visible
 * while the next request runs. `load` may be a new closure every render; only a
 * new `key` (or reload()) sends a request, using the `load` of that render.
 * A positive `pollMs` refreshes on that interval without changing the key.
 */
export function useApi<T>(key: string, load: (token: string) => Promise<T>, pollMs = 0) {
  const { session } = useAuthSession();
  const token = session.token;
  const [version, setVersion] = useState(0);
  const [result, setResult] = useState<Result<T> | null>(null);
  const requestKey = `${key}#${version}`;

  useEffect(() => {
    if (!token) return;
    let cancelled = false;
    load(token).then(
      (data) => { if (!cancelled) setResult({ key: requestKey, data }); },
      (error: unknown) => {
        if (cancelled) return;
        if (error instanceof AuthApiError && error.status === 401) void validateSession();
        const message = error instanceof AuthApiError ? error.message : "Could not load data. Try again.";
        const status = error instanceof AuthApiError ? error.status : 0;
        setResult((previous) => ({ key: requestKey, data: previous?.data, error: message, status }));
      },
    );
    return () => { cancelled = true; };
    // Requests are keyed by requestKey on purpose: it encodes everything `load` uses,
    // and `load` is a new closure every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token, requestKey]);

  useEffect(() => {
    if (!token || pollMs <= 0) return;
    const interval = window.setInterval(() => setVersion((v) => v + 1), pollMs);
    return () => window.clearInterval(interval);
  }, [token, pollMs]);

  const fresh = result?.key === requestKey;
  return {
    data: result?.data ?? null,
    loading: !fresh,
    error: fresh ? result?.error ?? "" : "",
    forbidden: fresh && result?.status === 403,
    reload: () => setVersion((v) => v + 1),
  };
}
