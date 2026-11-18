import { Heading, StatGrid } from "../../ui";
import { DeliveryTable, type Delivery } from "./delivery-table";

const DELIVERIES: Delivery[] = [
  { id: "NTF-0003", title: "Sandbox payout captured", channel: "Webhook", destination: "Sandbox event receiver", status: "Delivered", time: "14:42" },
  { id: "NTF-0002", title: "Transfer received · JRN-0003", channel: "Email", destination: "hana@example.test", status: "Retry scheduled", time: "13:24" },
  { id: "NTF-0001", title: "Transfer posted · JRN-0003", channel: "Webhook", destination: "Sandbox event receiver", status: "Delivered", time: "13:24" },
];

export function Notifications() {
  return (
    <>
      <Heading eyebrow="NOTIFICATION SERVICE" title="Delivery center" description="Sample delivery records only. No emails or webhooks are sent." />
      <StatGrid
        three
        stats={[
          { label: "Sample deliveries", value: "03", foot: "2 webhooks · 1 email", icon: "bell" },
          { label: "Delivered", value: "02", foot: "Illustrative delivery outcomes", icon: "shield" },
          { label: "Retry scheduled", value: "01", foot: "Sample provider-unavailable scenario", icon: "arrows" },
        ]}
      />
      <DeliveryTable deliveries={DELIVERIES} />
      <p className="footnote">Subscriptions, preferences, and provider retries require the Notification service.</p>
    </>
  );
}
