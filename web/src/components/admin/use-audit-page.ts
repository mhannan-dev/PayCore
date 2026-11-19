"use client";

import { useEffect, useState } from "react";
import { AuthApiError, fetchAuditEvents, type AuditAction, type AuditEvent, type Page } from "@/lib/auth-api";
import { useAuthSession, validateSession } from "@/lib/auth-session";

export type AuditPageQuery = { action: AuditAction | ""; q: string; page: number; pageSize: number };

type Result = { key: string; data?: Page<AuditEvent>; error?: string };

/** Loads one page of the audit log; the previous page stays visible while the next loads. */
export function useAuditPage(query: AuditPageQuery) {
  const { session } = useAuthSession();
  const token = session.token;
  const [version, setVersion] = useState(0);
  const [result, setResult] = useState<Result | null>(null);
  const { action, q, page, pageSize } = query;
  const key = JSON.stringify([action, q, page, pageSize, version]);

  useEffect(() => {
    if (!token) return;
    let cancelled = false;
    fetchAuditEvents(token, { action, q, limit: pageSize, offset: page * pageSize }).then(
      (data) => { if (!cancelled) setResult({ key, data }); },
      (error: unknown) => {
        if (cancelled) return;
        if (error instanceof AuthApiError && error.status === 401) void validateSession();
        const message = error instanceof AuthApiError ? error.message : "Could not load the audit log. Try again.";
        setResult((previous) => ({ key, data: previous?.data, error: message }));
      },
    );
    return () => { cancelled = true; };
  }, [token, key, action, q, page, pageSize]);

  const fresh = result?.key === key;
  const data = result?.data ?? null;
  return {
    events: data?.items ?? [],
    total: data?.total ?? 0,
    loaded: data !== null,
    loading: !fresh,
    error: fresh ? result?.error ?? "" : "",
    reload: () => setVersion((v) => v + 1),
  };
}
