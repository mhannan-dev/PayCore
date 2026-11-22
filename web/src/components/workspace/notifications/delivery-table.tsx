"use client";

import type { NotificationDelivery } from "@/lib/ledger-api";
import { Badge, Empty } from "../../ui";
import { deliveryLabel } from "./delivery-state";

export function DeliveryStatusBadge({ delivery }: { delivery: NotificationDelivery }) {
  return <Badge tone={delivery.status === "DELIVERED" ? "green" : delivery.status === "FAILED" ? "amber" : "neutral"}>{deliveryLabel(delivery)}</Badge>;
}
export function DeliveryTable({ deliveries, selected, onSelect, loading }: { deliveries: NotificationDelivery[]; selected: string | null; onSelect: (id: string | null) => void; loading: boolean }) {
  return <div className={`table-scroll${loading ? " is-refreshing" : ""}`}><table>
    <caption className="sr-only">Sandbox notification delivery history</caption>
    <thead><tr><th>EVENT / JOURNAL</th><th>CHANNEL / DESTINATION</th><th>STATUS</th><th>ATTEMPTS</th><th>CREATED</th><th>DETAILS</th></tr></thead>
    <tbody>{deliveries.map((delivery) => <tr key={delivery.id}>
      <td><strong className="cell-primary">{delivery.event_type}</strong><span className="cell-secondary mono">{delivery.payout_reference ?? delivery.journal_reference}</span></td>
      <td>{delivery.channel}<span className="cell-secondary">{delivery.destination}</span></td>
      <td><DeliveryStatusBadge delivery={delivery} /></td><td>{delivery.attempts}</td><td>{new Date(delivery.created_at).toLocaleString()}</td>
      <td><button type="button" className="text-link" aria-expanded={selected === delivery.id} onClick={() => onSelect(selected === delivery.id ? null : delivery.id)}>{selected === delivery.id ? "Close" : "View delivery"}</button></td>
    </tr>)}</tbody>
  </table>{deliveries.length === 0 && <Empty>No matching deliveries. New journal events create sandbox email and webhook records when the outbox worker processes them.</Empty>}</div>;
}
