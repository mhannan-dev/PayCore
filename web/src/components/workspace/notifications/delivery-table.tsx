"use client";

import { useState } from "react";
import { Badge, Panel } from "../../ui";

export type Delivery = { id: string; title: string; channel: string; destination: string; status: string; time: string };

const FILTERS = ["All deliveries", "Delivered", "Retry scheduled"];

function DeliveryRow({ row }: { row: Delivery }) {
  return (
    <tr>
      <td>
        <strong className="cell-primary">{row.title}</strong>
        <span className="cell-secondary mono">{row.id}</span>
      </td>
      <td>{row.channel}</td>
      <td>{row.destination}</td>
      <td><Badge tone={row.status === "Delivered" ? "green" : "amber"}>{row.status}</Badge></td>
      <td>{row.time}</td>
    </tr>
  );
}

export function DeliveryTable({ deliveries }: { deliveries: Delivery[] }) {
  const [filter, setFilter] = useState(FILTERS[0]);
  const rows = deliveries.filter((row) => filter === FILTERS[0] || row.status === filter);
  return (
    <Panel
      title="Delivery history"
      action={
        <label>
          <span className="sr-only">Delivery status</span>
          <select value={filter} onChange={(e) => setFilter(e.target.value)}>
            {FILTERS.map((f) => <option key={f}>{f}</option>)}
          </select>
        </label>
      }
    >
      <div className="table-scroll">
        <table>
          <thead><tr><th>EVENT</th><th>CHANNEL</th><th>DESTINATION</th><th>STATUS</th><th>TIME (JST)</th></tr></thead>
          <tbody>{rows.map((row) => <DeliveryRow key={row.id} row={row} />)}</tbody>
        </table>
      </div>
    </Panel>
  );
}
