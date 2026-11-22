import type { NotificationDelivery } from "@/lib/ledger-api";

export function deliveryLabel(delivery: NotificationDelivery): string {
  if (delivery.status !== "DELIVERED" && delivery.dispatch_status === "SENDING") return "Processing";
  if (delivery.status !== "DELIVERED" && delivery.next_attempt_at !== null) return delivery.attempts > 0 ? "Retry scheduled" : "Pending";
  switch (delivery.status) {
    case "DELIVERED": return "Delivered to sandbox";
    case "FAILED": return "Failed";
    case "PENDING": return "Pending";
  }
}
export function canRetryDelivery(delivery: NotificationDelivery): boolean {
  return delivery.can_retry && delivery.status === "FAILED" && delivery.dispatch_status === "FAILED";
}
