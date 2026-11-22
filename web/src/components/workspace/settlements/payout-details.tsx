"use client";

import { useState } from "react";
import { useAuthSession } from "@/lib/auth-session";
import { capturePayout, fetchJournal, fetchSettlement, releasePayout, type Settlement, type SettlementStatus } from "@/lib/ledger-api";
import { formatMoney } from "@/lib/money";
import { notify } from "@/lib/notify";
import { PERMISSIONS } from "@/lib/rbac";
import { ConfirmDialog } from "../../admin/confirm-dialog";
import { usePermission } from "../../can";
import { Badge, Notice, Panel } from "../../ui";
import { ApiStatus } from "../shared/api-status";
import { JournalTable } from "../shared/journal-table";
import { useApi } from "../shared/use-api";
import "../../admin/admin.css";

type Action = "capture" | "release";
export const payoutStatusLabels: Record<SettlementStatus, string> = { RESERVED: "Reserved", CAPTURED: "Captured", RELEASED: "Released" };

export function PayoutStatus({ status }: { status: SettlementStatus }) {
  return <Badge tone={status === "RESERVED" ? "amber" : status === "CAPTURED" ? "green" : "neutral"}>{payoutStatusLabels[status]}</Badge>;
}

function PayoutJournal({ id, label }: { id: string; label: string }) {
  const journal = useApi(`payout-journal-${id}`, (token) => fetchJournal(token, id));
  return <section className="settlement-journal"><h3>{label}</h3><p className="footnote mono">{id}</p>
    <ApiStatus loading={journal.loading} error={journal.error} forbidden={journal.forbidden} onRetry={journal.reload} />
    {journal.data && !journal.error && <JournalTable journals={[journal.data]} />}
  </section>;
}

function PayoutAction({ payout, action, onClose, onUpdated, onFailed }: { payout: Settlement; action: Action; onClose: () => void; onUpdated: (payout: Settlement) => void; onFailed: () => void }) {
  const { session } = useAuthSession();
  const [key] = useState(() => crypto.randomUUID());
  const capturing = action === "capture";
  return <ConfirmDialog title={`${capturing ? "Capture" : "Release"} payout?`} icon={capturing ? "bank" : "refresh"} tone={capturing ? "warning" : "danger"}
    confirmLabel={capturing ? "Capture payout" : "Release funds"} pendingLabel={capturing ? "Capturing…" : "Releasing…"} onClose={onClose}
    onConfirm={async () => {
      try {
        const updated = await (capturing ? capturePayout : releasePayout)(session.token, key, payout.id);
        notify.success(`${updated.payout_reference}: ${payoutStatusLabels[updated.status].toLowerCase()}.`);
        onUpdated(updated);
        onClose();
      } catch (error) {
        onFailed();
        throw error;
      }
    }}>
    <p><strong>{payout.payout_reference}</strong> · {formatMoney(payout.amount, payout.currency)}</p>
    <p>{capturing ? `Complete the sandbox payout to ${payout.beneficiary_name} by moving reserved funds from clearing to the operating bank.` : `Cancel this payout and return the reserved funds to ${payout.source_account_name}.`}</p>
    <p>This creates an immutable ledger journal. Retrying this action is safe.</p>
  </ConfirmDialog>;
}

export function PayoutDetails({ id, version, onClose, onUpdated }: { id: string; version: number; onClose: () => void; onUpdated: (payout: Settlement) => void }) {
  const can = usePermission();
  const detail = useApi(`payout-${id}-${version}`, (token) => fetchSettlement(token, id));
  const [action, setAction] = useState<Action | null>(null);
  const payout = detail.data;
  return <Panel title="Payout details" description={payout?.payout_reference ?? "Loading payout…"} action={<button type="button" className="text-link" onClick={onClose}>Close details</button>}>
    <ApiStatus loading={detail.loading} error={detail.error} forbidden={detail.forbidden} onRetry={detail.reload} />
    {payout && !detail.error && <>
      <div className="summary-list">
        <div><span>Status</span><PayoutStatus status={payout.status} /></div>
        <div><span>Amount</span><strong className="money">{formatMoney(payout.amount, payout.currency)} · {payout.currency}</strong></div>
        <div><span>Source wallet</span><strong>{payout.source_account_name}</strong></div>
        <div><span>Clearing account</span><strong>{payout.clearing_account_name}</strong></div>
        <div><span>Beneficiary</span><strong>{payout.beneficiary_name}</strong></div>
        <div><span>Bank</span><strong>{payout.beneficiary_bank}</strong></div>
        <div><span>Beneficiary account</span><strong className="mono">{payout.beneficiary_account}</strong></div>
        <div><span>Provider</span><strong>{payout.provider_mode} · {payout.provider_reference}</strong></div>
        <div><span>Last updated</span><strong>{new Date(payout.updated_at).toLocaleString()}</strong></div>
      </div>
      {payout.status === "RESERVED" && <div className="panel-bottom">
        {payout.provider_mode !== "SANDBOX" ? <Notice icon="bank">This workflow supports sandbox payouts only.</Notice> : can(PERMISSIONS.transfersReverse) ? <div className="settlement-actions">
          <button type="button" className="button primary" disabled={detail.loading} onClick={() => setAction("capture")}>Capture payout</button>
          <button type="button" className="button secondary" disabled={detail.loading} onClick={() => setAction("release")}>Release funds</button>
        </div> : <Notice icon="lock">Capture and release require the transfers:reverse permission.</Notice>}
      </div>}
      <PayoutJournal id={payout.reserve_journal_id} label="Reservation journal" />
      {payout.capture_journal_id && <PayoutJournal id={payout.capture_journal_id} label="Capture journal" />}
      {payout.release_journal_id && <PayoutJournal id={payout.release_journal_id} label="Release journal" />}
      {action && <PayoutAction payout={payout} action={action} onClose={() => setAction(null)} onUpdated={onUpdated} onFailed={detail.reload} />}
    </>}
  </Panel>;
}
