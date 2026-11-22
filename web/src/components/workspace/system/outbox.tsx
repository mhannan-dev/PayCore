"use client";

import { useCallback, useState } from "react";
import { AuthApiError } from "@/lib/auth-api";
import { useAuthSession } from "@/lib/auth-session";
import { fetchOutboxMetrics, replayOutboxEvents, type OutboxMetrics } from "@/lib/ledger-api";
import { hasPermission, PERMISSIONS } from "@/lib/rbac";
import { Icon } from "../../icons";
import { Badge, Heading, Panel, StatGrid } from "../../ui";
import { ApiStatus } from "../shared/api-status";
import { useApi } from "../shared/use-api";

/** Operator view of committed ledger events waiting for broker delivery. */
export function Outbox() {
  const { session } = useAuthSession();
  const canReplay = hasPermission(session.permissions, PERMISSIONS.systemWrite);
  const load = useCallback(async (token: string): Promise<OutboxMetrics> => fetchOutboxMetrics(token), []);
  const { data, loading, error, forbidden, reload } = useApi<OutboxMetrics>("outbox-metrics", load);
  const [replaying, setReplaying] = useState(false);
  const [message, setMessage] = useState("");

  const replay = async () => {
    if (!session.token) return;
    setReplaying(true);
    setMessage("");
    try {
      const result = await replayOutboxEvents(session.token, { ids: [] });
      setMessage(`${result.replayed_count} failed event(s) queued for retry.`);
      reload();
    } catch (cause: unknown) {
      setMessage(cause instanceof AuthApiError ? cause.message : "Could not replay events. Try again.");
    } finally {
      setReplaying(false);
    }
  };

  const healthy = data?.healthy ?? false;

  return (
    <>
      <Heading
        eyebrow="SYSTEM DELIVERY"
        title="Event outbox"
        description="Committed journal events are delivered at least once. Monitor backlog and replay failures after the broker recovers."
      />
      <ApiStatus loading={loading} error={error} forbidden={forbidden} onRetry={reload} />

      <StatGrid
        stats={[
          { label: "Delivery status", value: loading ? "..." : healthy ? "Healthy" : "Needs attention", foot: "Pending older than 60 s is unhealthy", icon: "shield" },
          { label: "Pending", value: loading ? "..." : String(data?.pending_count ?? 0), foot: "Waiting for the next publish attempt", icon: "play" },
          { label: "In flight", value: loading ? "..." : String(data?.sending_count ?? 0), foot: "Leased by a relay worker", icon: "arrows" },
          { label: "Failed", value: loading ? "..." : String(data?.failed_count ?? 0), foot: "Retries exhausted; replay to requeue", icon: "bell" },
        ]}
      />

      <Panel
        title="Delivery health"
        description="Oldest pending event age and total events published since deployment."
        action={<Badge tone={healthy ? "green" : "amber"}>{healthy ? "Streaming" : "Backlog"}</Badge>}
      >
        <div className="chart-summary">
          <strong>{loading ? "..." : `${(data?.lag_seconds ?? 0).toFixed(1)} s`}</strong>
          <span>oldest pending event</span>
        </div>
        <p className="muted">{loading ? "..." : `${data?.published_count ?? 0} events published`}</p>
        {canReplay && (
          <div className="chart-legend">
            <button type="button" className="button primary" onClick={() => void replay()} disabled={replaying || loading}>
              <Icon name="refresh" size={16} /> {replaying ? "Replaying…" : "Replay failed events"}
            </button>
            {message && <span className="muted" role="status">{message}</span>}
          </div>
        )}
      </Panel>
    </>
  );
}
