"use client";

import { useEffect, useState } from "react";
import { AuthApiError, fetchRoles, resendInvitation, type InvitationResult, type ManagedUser } from "@/lib/auth-api";
import { useAuthSession, validateSession } from "@/lib/auth-session";
import { notify } from "@/lib/notify";
import { PERMISSIONS } from "@/lib/rbac";
import { usePermission } from "../can";
import { Icon } from "../icons";
import { Heading, Notice, Panel } from "../ui";
import { Pagination } from "../ui/pagination";
import { roleLabel } from "../user-profile/roles";
import { AdminStatus } from "./admin-status";
import { CreateUserDialog } from "./create-user-dialog";
import { DeleteUserDialog } from "./delete-user-dialog";
import { RoleEditorDialog } from "./role-editor-dialog";
import { UsersTable } from "./users-table";
import { useAdminData } from "./use-admin-data";
import { useUsersPage } from "./use-users-page";
import "./admin.css";

const PAGE_SIZES = [10, 20, 50];
const SEARCH_DELAY_MS = 300;

export function UsersAdmin() {
  const { session } = useAuthSession();
  const can = usePermission();
  const canEdit = can(PERMISSIONS.usersWrite);
  const rolesData = useAdminData(fetchRoles);

  // Filters and paging; the search is sent once typing pauses.
  const [query, setQuery] = useState("");
  const [search, setSearch] = useState("");
  const [roleFilter, setRoleFilter] = useState("all");
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(20);
  useEffect(() => {
    const timer = window.setTimeout(() => setSearch(query.trim()), SEARCH_DELAY_MS);
    return () => window.clearTimeout(timer);
  }, [query]);
  const users = useUsersPage({ q: search, role: roleFilter, page, pageSize });
  // While an invitation email is being sent in the background, refresh to show the outcome.
  const sending = users.users.some((u) => u.invite_email_status === "pending");
  const { reload: reloadUsers, loading: usersLoading } = users;
  useEffect(() => {
    if (!sending || usersLoading) return;
    const timer = window.setTimeout(reloadUsers, 3000);
    return () => window.clearTimeout(timer);
  }, [sending, usersLoading, reloadUsers]);

  const [editing, setEditing] = useState<ManagedUser | null>(null);
  const [adding, setAdding] = useState(false);
  const [resendingId, setResendingId] = useState<string | null>(null);
  const [deleting, setDeleting] = useState<ManagedUser | null>(null);

  const refresh = () => { rolesData.reload(); users.reload(); };
  const heading = (
    <Heading
      eyebrow="ADMINISTRATION"
      title="Users & access"
      description="See who can sign in to PayCore and which roles they hold. Role changes apply on the user's next request."
      action={
        <div className="adm-heading-actions">
          <button type="button" className="button secondary" onClick={refresh} disabled={users.loading}><Icon name="refresh" size={15} />Refresh</button>
          {canEdit && (
            <button type="button" className="button primary" onClick={() => setAdding(true)} disabled={rolesData.state.status !== "ready"}>
              <Icon name="users" size={15} /> Add user
            </button>
          )}
        </div>
      }
    />
  );
  if (rolesData.state.status !== "ready") return <div className="adm-page">{heading}<AdminStatus state={rolesData.state} onRetry={rolesData.reload} /></div>;

  const roles = rolesData.state.data;
  const currentUserId = session.userId || null;
  const pageCount = Math.max(1, Math.ceil(users.total / pageSize));

  const saved = (user: ManagedUser) => {
    users.patch(user);
    setEditing(null);
    // Editing yourself changes your own permissions; the role filter may no longer match.
    if (user.id === currentUserId) void validateSession();
    if (roleFilter !== "all") users.reload();
  };

  const created = ({ user }: InvitationResult) => {
    setAdding(false);
    // Show the new account by searching for its email (its page position depends on sorting).
    setQuery(user.email);
    setSearch(user.email);
    setRoleFilter("all");
    setPage(0);
    users.reload();
  };

  async function resend(user: ManagedUser) {
    setResendingId(user.id);
    try {
      const result = await resendInvitation(session.token, user.id);
      users.patch(result.user);
      if (result.email_status === "not_configured") notify.warning(`A new invitation was created for ${user.email}, but no mail server is configured, so no email was sent.`);
      else notify.success(`Sending a new invitation to ${user.email}. Earlier links no longer work.`);
    } catch (error) {
      notify.error(error instanceof AuthApiError ? error.message : "Could not re-send the invitation. Try again.");
      if (error instanceof AuthApiError && error.code === "already_activated") users.reload();
    } finally {
      setResendingId(null);
    }
  }

  const filtered = search !== "" || roleFilter !== "all";
  return (
    <div className="adm-page">
      {heading}
      {!canEdit && <Notice icon="lock">You can view access but not change it. Changing roles needs the <code>users:write</code> permission.</Notice>}
      <Panel
        title="Accounts"
        description={users.loaded ? `${users.total} ${filtered ? "matching " : ""}${users.total === 1 ? "user" : "users"}` : "Loading…"}
        action={
          <div className="adm-filters">
            <label className="adm-search">
              <Icon name="search" size={15} />
              <span className="sr-only">Search users</span>
              <input type="search" placeholder="Search name or email" maxLength={100} value={query} onChange={(e) => { setQuery(e.target.value); setPage(0); }} />
            </label>
            <label>
              <span className="sr-only">Filter by role</span>
              <select value={roleFilter} onChange={(e) => { setRoleFilter(e.target.value); setPage(0); }}>
                <option value="all">All roles</option>
                {roles.map((r) => <option key={r.code} value={r.code}>{roleLabel(r.code)}</option>)}
              </select>
            </label>
          </div>
        }
      >
        {users.error && (
          <div className="adm-inline-error" role="alert">
            <span>{users.error}</span>
            <button type="button" className="button secondary adm-edit-btn" onClick={users.reload}><Icon name="refresh" size={14} />Try again</button>
          </div>
        )}
        <div className={`adm-table-wrap${users.loading && users.loaded ? " is-loading" : ""}`} aria-busy={users.loading}>
          {!users.loaded && !users.error ? (
            <div className="adm-status" role="status"><span className="progress-spinner" aria-hidden="true" />Loading users…</div>
          ) : (
            <UsersTable users={users.users} currentUserId={currentUserId} canEdit={canEdit} resendingId={resendingId} onEdit={setEditing} onResend={resend} onDelete={setDeleting} />
          )}
        </div>
        {users.loaded && users.total > 0 && (
          <Pagination
            label="Users pages"
            page={Math.min(page, pageCount - 1)}
            pageSize={pageSize}
            total={users.total}
            onPageChange={setPage}
            pageSizes={PAGE_SIZES}
            onPageSizeChange={(size) => { setPageSize(size); setPage(0); }}
            disabled={users.loading}
          />
        )}
      </Panel>
      {deleting && (
        <DeleteUserDialog
          key={deleting.id}
          user={deleting}
          onClose={() => setDeleting(null)}
          onDeleted={() => {
            setDeleting(null);
            // Deleting the only row on the last page: step back a page.
            if (users.users.length === 1 && page > 0) setPage(page - 1);
            else users.reload();
          }}
        />
      )}
      {adding && <CreateUserDialog roles={roles} onClose={() => setAdding(false)} onCreated={created} />}
      {editing && (
        <RoleEditorDialog
          key={editing.id}
          user={editing}
          roles={roles}
          isSelf={editing.id === currentUserId}
          onClose={() => setEditing(null)}
          onSaved={saved}
        />
      )}
    </div>
  );
}
