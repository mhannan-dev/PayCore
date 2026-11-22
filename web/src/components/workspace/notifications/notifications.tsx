"use client";

import { useState } from "react";
import { DELIVERY_CHANNELS, DELIVERY_STATUSES, fetchNotificationMetrics, fetchNotifications, type DeliveryChannel, type DeliveryStatus } from "@/lib/ledger-api";
import { useWorkspaceCurrency } from "@/lib/workspace-currency";
import { Badge, Heading, Notice, Panel, StatGrid } from "../../ui";
import { Pagination } from "../../ui/pagination";
import { ApiStatus } from "../shared/api-status";
import { useApi } from "../shared/use-api";
import { DeliveryDetails } from "./delivery-details";
import { DeliveryTable } from "./delivery-table";
import "./notifications.css";

const PAGE_SIZE = 25;
export function Notifications() {
  const { currency } = useWorkspaceCurrency();
  const [status, setStatus] = useState<DeliveryStatus | "">("");
  const [channel, setChannel] = useState<DeliveryChannel | "">("");
  const [page, setPage] = useState(0);
  const [version, setVersion] = useState(0);
  const [selected, setSelected] = useState<string | null>(null);
  const list = useApi(JSON.stringify(["notifications", currency, status, channel, page, version]), (token) => fetchNotifications(token, { currency, status, channel, limit: PAGE_SIZE, offset: page * PAGE_SIZE }), 10000);
  const metrics = useApi(JSON.stringify(["notification-metrics", currency, version]), (token) => fetchNotificationMetrics(token, currency), 10000);
  const total = list.data?.total ?? 0;
  return <div className="notifications-page">
    <Heading eyebrow="NOTIFICATION SERVICE" title="Delivery center" description={`Live ${currency} notification records from committed ledger events.`} />
    <Notice icon="bell">Email and webhook deliveries use local sandbox sinks. A sandbox receipt confirms local acceptance; no email or external notification webhook is sent.</Notice>
    <ApiStatus loading={metrics.loading && !metrics.data} error={metrics.error} forbidden={metrics.forbidden} onRetry={metrics.reload} />
    <StatGrid three stats={[
      { label: "Pending", value: metrics.data ? String(metrics.data.pending) : "—", foot: `${currency} · awaiting a first outcome`, icon: "bell" },
      { label: "Delivered to sandbox", value: metrics.data ? String(metrics.data.delivered) : "—", foot: "Persisted sandbox receipts", icon: "shield" },
      { label: "Failed last attempt", value: metrics.data ? String(metrics.data.failed) : "—", foot: "Includes scheduled automatic retries", icon: "refresh" },
    ]} />
    <Panel title="Delivery history" action={<Badge tone="neutral">{total} deliveries</Badge>}>
      <div className="toolbar">
        <label><span className="sr-only">Delivery status</span><select value={status} onChange={(event) => { setStatus(DELIVERY_STATUSES.find((value) => value === event.target.value) ?? ""); setPage(0); }}>
          <option value="">All statuses</option>{DELIVERY_STATUSES.map((value) => <option key={value} value={value}>{value === "FAILED" ? "Failed last attempt" : value === "DELIVERED" ? "Delivered to sandbox" : "Pending"}</option>)}
        </select></label>
        <label><span className="sr-only">Delivery channel</span><select value={channel} onChange={(event) => { setChannel(DELIVERY_CHANNELS.find((value) => value === event.target.value) ?? ""); setPage(0); }}>
          <option value="">All channels</option>{DELIVERY_CHANNELS.map((value) => <option key={value}>{value}</option>)}
        </select></label>
        <button type="button" className="text-link" disabled={list.loading} onClick={() => setVersion((value) => value + 1)}>Refresh</button>
      </div>
      <ApiStatus loading={list.loading && !list.data} error={list.error} forbidden={list.forbidden} onRetry={list.reload} />
      {list.data && <DeliveryTable deliveries={list.data.items} selected={selected} onSelect={setSelected} loading={list.loading} />}
      {total > PAGE_SIZE && <Pagination label="Delivery pages" page={page} pageSize={PAGE_SIZE} total={total} onPageChange={setPage} disabled={list.loading} />}
    </Panel>
    {selected && <DeliveryDetails key={selected} id={selected} version={version} onClose={() => setSelected(null)} onUpdated={() => setVersion((value) => value + 1)} />}
  </div>;
}
