"use client";

import type { ManagedUser } from "@/lib/auth-api";
import { initials } from "@/lib/auth-session";
import { Icon } from "../icons";
import { Badge, EmptyHint, type BadgeTone } from "../ui";
import { RoleChips } from "../user-profile/roles";

export const displayName = (u: Pick<ManagedUser, "first_name" | "last_name">) => [u.first_name, u.last_name].filter(Boolean).join(" ");

const joined = (iso: string) => {
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? "—" : date.toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
};

type Props = {
  users: ManagedUser[];
  currentUserId: string | null;
  canEdit: boolean;
  /** ID of the user whose invitation is being re-sent, if any. */
  resendingId: string | null;
  onEdit: (user: ManagedUser) => void;
  onResend: (user: ManagedUser) => void;
  onDelete: (user: ManagedUser) => void;
  /** Deactivate an active user or reactivate a disabled one. */
  onToggleActive: (user: ManagedUser) => void;
  /** ID of the user being activated, if any. */
  activatingId: string | null;
};

type Status = { label: string; tone: BadgeTone; title?: string };

function accountStatus(user: ManagedUser): Status {
  if (!user.is_active) return { label: "Disabled", tone: "neutral" };
  if (user.password_set) return { label: "Active", tone: "green" };
  const expires = user.invite_expires_at ? new Date(user.invite_expires_at) : null;
  if (!expires || expires.getTime() <= Date.now()) return { label: "Invite expired", tone: "neutral", title: "Re-send the invitation so they can set a password." };
  return { label: "Invited", tone: "amber", title: `Waiting for them to set a password. Link expires ${expires.toLocaleString()}.` };
}

const time = (iso: string | null) => {
  const date = iso ? new Date(iso) : null;
  return date && !Number.isNaN(date.getTime()) ? date.toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" }) : "";
};

/** One line describing the invitation email's delivery, under the Invited badge. */
function EmailDelivery({ user }: { user: ManagedUser }) {
  switch (user.invite_email_status) {
    case "sent":
      return <span className="adm-email is-sent"><Icon name="mail" size={12} />Email sent {time(user.invite_email_sent_at)}</span>;
    case "pending":
      return <span className="adm-email is-pending" role="status"><span className="progress-spinner adm-email-spinner" aria-hidden="true" />{user.invite_email_error ? "Retrying email…" : "Sending email…"}</span>;
    case "failed":
      return <span className="adm-email is-failed" title={user.invite_email_error ?? undefined}><Icon name="mail" size={12} />Email failed{user.invite_email_error ? `: ${user.invite_email_error}` : ""}</span>;
    case "not_configured":
      return <span className="adm-email is-failed" title="Set SMTP_* or MAIL_* for the API, then use Resend invite."><Icon name="mail" size={12} />Not emailed: no mail server configured</span>;
    default:
      return null;
  }
}

export function UsersTable({ users, currentUserId, canEdit, resendingId, onEdit, onResend, onDelete, onToggleActive, activatingId }: Props) {
  if (users.length === 0) return <EmptyHint icon="users" title="No matching users" text="Try a different name, email or role filter." />;
  return (
    <div className="table-scroll">
      <table className="adm-table">
        <thead>
          <tr><th>USER</th><th>ROLES</th><th>STATUS</th><th>JOINED</th>{canEdit && <th><span className="sr-only">Actions</span></th>}</tr>
        </thead>
        <tbody>
          {users.map((user) => {
            const name = displayName(user);
            const self = user.id === currentUserId;
            const status = accountStatus(user);
            const pending = user.is_active && !user.password_set;
            const resending = resendingId === user.id;
            return (
              <tr key={user.id}>
                <td>
                  <div className="adm-user">
                    <span className="adm-user-avatar" aria-hidden="true">{initials(name)}</span>
                    <span>
                      <strong className="cell-primary">{name}{self && <span className="adm-you">You</span>}</strong>
                      <span className="cell-secondary">{user.email}</span>
                    </span>
                  </div>
                </td>
                <td><RoleChips roles={user.roles} /></td>
                <td>
                  <span title={status.title}><Badge tone={status.tone}>{status.label}</Badge></span>
                  {pending && <EmailDelivery user={user} />}
                </td>
                <td>{joined(user.created_at)}</td>
                {canEdit && (
                  <td className="adm-actions-cell">
                    <div className="adm-row-actions">
                      {pending && (
                        <button type="button" className="button secondary adm-edit-btn" onClick={() => onResend(user)} disabled={resending} aria-label={`Re-send invitation to ${user.email}`}>
                          <Icon name="mail" size={14} /> {resending ? "Sending…" : "Resend invite"}
                        </button>
                      )}
                      <button type="button" className="button secondary adm-edit-btn" onClick={() => onEdit(user)} aria-label={`Edit roles for ${name}`}>
                        <Icon name="edit" size={14} /> Edit roles
                      </button>
                      {!self && (
                        <button
                          type="button"
                          className={`button secondary adm-edit-btn ${user.is_active ? "adm-deactivate-btn" : "adm-activate-btn"}`}
                          onClick={() => onToggleActive(user)}
                          disabled={activatingId === user.id}
                          aria-label={`${user.is_active ? "Deactivate" : "Activate"} ${name}`}
                        >
                          <Icon name={user.is_active ? "lock" : "checkCircle"} size={14} />
                          {user.is_active ? "Deactivate" : activatingId === user.id ? "Activating…" : "Activate"}
                        </button>
                      )}
                      {!self && (
                        <button type="button" className="button secondary adm-edit-btn adm-delete-btn" onClick={() => onDelete(user)} aria-label={`Delete ${name}`} title="Delete user">
                          <Icon name="trash" size={14} /> Delete
                        </button>
                      )}
                    </div>
                  </td>
                )}
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
