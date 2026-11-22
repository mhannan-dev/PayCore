"use client";

import { useState } from "react";
import { useAuthSession } from "@/lib/auth-session";
import { fetchJournal, fetchNotification, retryNotification, type NotificationDelivery } from "@/lib/ledger-api";
import { notify } from "@/lib/notify";
import { PERMISSIONS } from "@/lib/rbac";
import { ConfirmDialog } from "../../admin/confirm-dialog";
import { usePermission } from "../../can";
import { Notice, Panel } from "../../ui";
import { PayoutDetails } from "../settlements/payout-details";
import { ApiStatus } from "../shared/api-status";
import { JournalTable } from "../shared/journal-table";
import { useApi } from "../shared/use-api";
import { canRetryDelivery } from "./delivery-state";
import { DeliveryStatusBadge } from "./delivery-table";
import "../../admin/admin.css";

function LinkedJournal({ id }: { id: string }) {
  const journal = useApi(`notification-journal-${id}`, (token) => fetchJournal(token, id));
  return <section className="notification-linked"><h3>Ledger journal</h3>
    <ApiStatus loading={journal.loading} error={journal.error} forbidden={journal.forbidden} onRetry={journal.reload} />
    {journal.data && !journal.error && <JournalTable journals={[journal.data]} />}
  </section>;
}
function RetryDialog({ delivery, onClose, onUpdated }: { delivery: NotificationDelivery; onClose: () => void; onUpdated: () => void }) {
  const { session } = useAuthSession();
  return <ConfirmDialog title="Retry failed notification?" icon="refresh" tone="warning" confirmLabel="Queue retry" pendingLabel="Queuing…" onClose={onClose}
    onConfirm={async () => {
      try {
        await retryNotification(session.token, delivery.id, { attempts: delivery.attempts });
        notify.success("Notification event queued for retry. Successful channels will be skipped.");
        onUpdated();
        onClose();
      } catch (error) {
        onUpdated();
        throw error;
      }
    }}>
    <p>Retry the failed {delivery.channel.toLowerCase()} notification for <strong>{delivery.journal_reference}</strong>?</p>
    <p>This requeues the event&apos;s failed channels. It does not repost a transfer or resend channels with a saved delivery receipt.</p>
  </ConfirmDialog>;
}
export function DeliveryDetails({ id, version, onClose, onUpdated }: { id: string; version: number; onClose: () => void; onUpdated: () => void }) {
  const can = usePermission();
  const detail = useApi(`notification-${id}-${version}`, (token) => fetchNotification(token, id), 10000);
  const [retrying, setRetrying] = useState(false);
  const [journalOpen, setJournalOpen] = useState(false);
  const [payoutOpen, setPayoutOpen] = useState(false);
  const delivery = detail.data?.delivery;
  return <>
    <Panel title="Delivery details" description={delivery?.id ?? "Loading notification…"} action={<button type="button" className="text-link" onClick={onClose}>Close details</button>}>
      <ApiStatus loading={detail.loading && !detail.data} error={detail.error} forbidden={detail.forbidden} onRetry={detail.reload} />
      {delivery && !detail.error && <>
        <div className="summary-list">
          <div><span>Delivery</span><DeliveryStatusBadge delivery={delivery} /></div>
          <div><span>Channel</span><strong>{delivery.channel} · {delivery.destination}</strong></div>
          <div><span>Event</span><strong className="mono">{delivery.event_id}</strong></div>
          <div><span>Event type</span><strong>{delivery.event_type}</strong></div>
          <div><span>Journal</span><strong className="mono">{delivery.journal_reference} · {delivery.journal_id}</strong></div>
          {delivery.payout_reference && <div><span>Payout</span><strong>{delivery.payout_reference}</strong></div>}
          <div><span>Attempts</span><strong>{delivery.attempts}</strong></div>
          <div><span>Sandbox receipt</span><strong className="mono">{delivery.receipt ?? "Not accepted yet"}</strong></div>
          {delivery.delivered_at && <div><span>Delivered</span><strong>{new Date(delivery.delivered_at).toLocaleString()}</strong></div>}
          {delivery.next_attempt_at && <div><span>Automatic retry</span><strong>{new Date(delivery.next_attempt_at).toLocaleString()}</strong></div>}
          {delivery.last_error && <div><span>Last failure</span><strong>{delivery.last_error}</strong></div>}
        </div>
        <div className="notification-links">
          {can(PERMISSIONS.transfersRead) && <>
            <button type="button" className="text-link" onClick={() => setJournalOpen((open) => !open)}>{journalOpen ? "Hide" : "View"} journal</button>
            {delivery.settlement_id && <button type="button" className="text-link" onClick={() => setPayoutOpen((open) => !open)}>{payoutOpen ? "Hide" : "View"} payout</button>}
          </>}
          {can(PERMISSIONS.notificationsRetry) && canRetryDelivery(delivery) && <button type="button" className="button secondary" disabled={detail.loading} onClick={() => setRetrying(true)}>Retry failed delivery</button>}
        </div>
        {!can(PERMISSIONS.transfersRead) && <Notice icon="lock">Opening linked journals and payouts requires transfers:read.</Notice>}
        {delivery.can_retry && !can(PERMISSIONS.notificationsRetry) && <Notice icon="lock">Manual retries require notifications:retry.</Notice>}
        <section className="notification-attempts"><h3>Attempt history</h3>
          {detail.data?.attempts.length === 0 && <p className="muted">No completed attempts yet.</p>}
          <ol>{detail.data?.attempts.map((attempt) => <li key={attempt.attempt}><strong>Attempt {attempt.attempt} · {attempt.status === "DELIVERED" ? "Delivered to sandbox" : "Failed"}</strong><p>{new Date(attempt.occurred_at).toLocaleString()} · {attempt.receipt ?? attempt.error}</p></li>)}</ol>
        </section>
        {journalOpen && can(PERMISSIONS.transfersRead) && <LinkedJournal id={delivery.journal_id} />}
        {retrying && <RetryDialog delivery={delivery} onClose={() => setRetrying(false)} onUpdated={onUpdated} />}
      </>}
    </Panel>
    {delivery?.settlement_id && payoutOpen && can(PERMISSIONS.transfersRead) && <PayoutDetails id={delivery.settlement_id} version={version} onClose={() => setPayoutOpen(false)} onUpdated={onUpdated} />}
  </>;
}
