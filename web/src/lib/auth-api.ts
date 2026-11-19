// Thin client for the PayCore API (see docs/email_login.md).
const API_ORIGIN = (process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080").replace(/\/+$/, "");
const API_V1 = `${API_ORIGIN}/api/v1`;

export type AuthUser = {
  id: string;
  first_name: string;
  last_name: string | null;
  /** Photo URL: "/api/v1/auth/photo" for uploaded photos (needs the bearer token), or an external URL. */
  photo: string | null;
  photo_attachment_id: string | null;
  email: string;
  roles: string[];
  /** Permission codes granted by the roles, e.g. "users:read". Missing in sessions stored before RBAC. */
  permissions?: string[];
};
export type Captcha = { captcha_id: string; image: string; expires_in: number };
export type LoginResponse = { token: string; token_type: "Bearer"; expires_in: number; user: AuthUser };
export type LoginInput = { email: string; password: string; captcha_id: string; captcha_answer: string };
export type Attachment = { id: string; file_name: string; content_type: string; size_bytes: number };

export class AuthApiError extends Error {
  constructor(public status: number, public code: string, message: string) {
    super(message);
  }
}

export type RequestOptions = RequestInit & { token?: string; timeoutMs?: number };

/** Sends a request to /api/v1{path} and returns the raw Response, or throws AuthApiError. */
async function send(path: string, { token, timeoutMs = 8000, ...init }: RequestOptions = {}): Promise<Response> {
  const headers = new Headers(init.headers);
  if (!headers.has("Accept")) headers.set("Accept", "application/json");
  // FormData sets its own multipart Content-Type (with boundary); only JSON bodies need one.
  if (typeof init.body === "string") headers.set("Content-Type", "application/json");
  if (token) headers.set("Authorization", `Bearer ${token}`);
  const method = (init.method ?? "GET").toUpperCase();
  // Reads are safe to repeat, so they ride out a short API restart (e.g. an Air hot-reload rebuild).
  const retryDelays = method === "GET" && !init.signal ? RETRY_DELAYS_MS : [];
  let response: Response | undefined;
  for (let attempt = 0; !response; attempt++) {
    try {
      response = await fetch(`${API_V1}${path}`, { ...init, headers, cache: "no-store", signal: init.signal ?? AbortSignal.timeout(timeoutMs) });
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError" && init.signal?.aborted) throw error;
      const timedOut = error instanceof DOMException && error.name === "TimeoutError";
      if (!timedOut && attempt < retryDelays.length) {
        await new Promise((resolve) => setTimeout(resolve, retryDelays[attempt]));
        continue;
      }
      throw networkError(timedOut);
    }
  }
  if (!response.ok) throw await toError(response, path, method);
  return response;
}

const RETRY_DELAYS_MS = [700, 1500, 3000];

function networkError(timedOut: boolean) {
  if (timedOut) {
    return new AuthApiError(0, "timeout", `The PayCore API at ${API_ORIGIN} took too long to respond. Try again in a moment.`);
  }
  // Browsers report CORS rejections and a stopped server the same way, so name both causes.
  const origin = typeof window === "undefined" ? "this site" : window.location.origin;
  return new AuthApiError(0, "network_error", `Cannot reach the PayCore API at ${API_ORIGIN}. Check that it is running and that its WEB_ORIGINS allows ${origin}.`);
}

async function toError(response: Response, path: string, method: string) {
  const body: unknown = await response.json().catch(() => null);
  const data = (body && typeof body === "object" ? body : {}) as { error?: string; message?: string };
  // Missing route/method (or an old API answering in plain text): usually an outdated API process.
  const missingRoute = data.error === "route_not_found" || data.error === "method_not_allowed" || ((response.status === 404 || response.status === 405) && !data.message);
  const message = missingRoute
    ? `The PayCore API does not support ${method} ${path} (HTTP ${response.status}). Restart the API so it runs the latest code.`
    : response.status === 429 && !data.message
      ? "Too many requests. Wait a minute and try again."
      : data.message ?? `The PayCore API returned an error (HTTP ${response.status}).`;
  return new AuthApiError(response.status, data.error ?? "http_error", message);
}

/** Typed JSON request to /api/v1{path}; the API contract (coding rule 4) is asserted here. */
export async function request<T>(path: string, options?: RequestOptions): Promise<T> {
  const response = await send(path, options);
  if (response.status === 204) return undefined as T;
  return (await response.json()) as T;
}

// --- Auth ---------------------------------------------------------------------
export const fetchCaptcha = () => request<Captcha>("/auth/captcha");
export const login = (input: LoginInput) => request<LoginResponse>("/auth/login", { method: "POST", body: JSON.stringify(input) });
export const fetchMe = (token: string) => request<AuthUser>("/auth/me", { token });
export type ProfileInput = { first_name: string; last_name: string | null };
export const updateMe = (token: string, input: ProfileInput) =>
  request<AuthUser>("/auth/me", { method: "PATCH", body: JSON.stringify(input), token });
export const logout = (token: string) => request<void>("/auth/logout", { method: "POST", token });

// --- Attachments & profile photo ---------------------------------------------
export function uploadAttachment(token: string, file: File) {
  const form = new FormData();
  form.append("file", file, file.name);
  return request<Attachment>("/attachments", { method: "POST", body: form, token, timeoutMs: 60000 });
}
export const deleteAttachment = (token: string, id: string) =>
  request<void>(`/attachments/${encodeURIComponent(id)}`, { method: "DELETE", token });
/** Sets (id) or clears (null) the profile photo. */
export type ProfilePhotoResult = { photo_attachment_id: string | null; photo: string | null };
export const setProfilePhoto = (token: string, attachmentId: string | null) =>
  request<ProfilePhotoResult>("/auth/photo", {
    method: "PUT",
    body: JSON.stringify({ attachment_id: attachmentId }),
    token,
  });
/** Downloads the signed-in user's photo; the endpoint needs the bearer token, so <img src> cannot load it directly. */
export async function fetchProfilePhoto(token: string): Promise<Blob> {
  const response = await send("/auth/photo", { token, headers: { Accept: "image/*" }, timeoutMs: 30000 });
  return response.blob();
}
/** True when `photo` is an absolute URL the browser can load without a token. */
export const isExternalPhoto = (photo: string | null) => !!photo && /^https?:\/\//i.test(photo);

// --- Access management (RBAC) -------------------------------------------------
export type RoleDetail = { id: string; code: string; name: string; description?: string; permissions: string[]; user_count: number };
export type PermissionDef = { id: string; code: string; name: string; description?: string };
export type ManagedUser = {
  id: string;
  first_name: string;
  last_name: string | null;
  email: string;
  roles: string[];
  is_active: boolean;
  created_at: string;
  /** False until the user accepts their emailed invitation and sets a password. */
  password_set: boolean;
  invite_expires_at: string | null;
  /** Delivery of the invitation email (sent in the background after invite/resend). */
  invite_email_status: InviteEmailStatus | null;
  invite_email_sent_at: string | null;
  invite_email_error: string | null;
};
export type InviteEmailStatus = "pending" | "sent" | "failed" | "not_configured" | "unknown";
/** email_status is "pending" (sending in the background) or "not_configured". */
export type InvitationResult = { user: ManagedUser; email_status: InviteEmailStatus };
export type InvitationInfo = { email: string; first_name: string; expires_at: string };
type Items<T> = { items: T[] };

export const fetchRoles = (token: string) => request<Items<RoleDetail>>("/rbac/roles", { token }).then((r) => r.items);
export const fetchPermissions = (token: string) => request<Items<PermissionDef>>("/rbac/permissions", { token }).then((r) => r.items);
export type Page<T> = { items: T[]; limit: number; offset: number; total: number };
export type UserQuery = { q?: string; role?: string; limit: number; offset: number };
/** One page of users, filtered by name/email search and role on the server. */
export function fetchUsers(token: string, query: UserQuery) {
  const params = new URLSearchParams({ limit: String(query.limit), offset: String(query.offset) });
  if (query.q?.trim()) params.set("q", query.q.trim());
  if (query.role) params.set("role", query.role);
  return request<Page<ManagedUser>>(`/users?${params}`, { token });
}
export type NewUserInput = { first_name: string; last_name: string | null; email: string; roles: string[] };
/** Creates the user without a password and emails them a set-password link. */
export const createUser = (token: string, input: NewUserInput) =>
  request<InvitationResult>("/users", { method: "POST", body: JSON.stringify(input), token, timeoutMs: 15000 });
/** Sends a fresh invitation link; the previous one stops working. */
export const resendInvitation = (token: string, userId: string) =>
  request<InvitationResult>(`/users/${encodeURIComponent(userId)}/invitation`, { method: "POST", token, timeoutMs: 15000 });

// Public: the invitation token travels in the body so it never lands in access logs.
export const inspectInvitation = (inviteToken: string) =>
  request<InvitationInfo>("/auth/invitations/inspect", { method: "POST", body: JSON.stringify({ token: inviteToken }) });
// Forgot password (public). The request answers the same whether or not the account exists.
export type PasswordResetInfo = { email: string; first_name: string; expires_at: string };
export type PasswordResetRequestInput = { email: string; captcha_id: string; captcha_answer: string };
export const requestPasswordReset = (input: PasswordResetRequestInput) =>
  request<{ message: string }>("/auth/password-reset/request", { method: "POST", body: JSON.stringify(input) });
export const inspectPasswordReset = (resetToken: string) =>
  request<PasswordResetInfo>("/auth/password-reset/inspect", { method: "POST", body: JSON.stringify({ token: resetToken }) });
export const confirmPasswordReset = (resetToken: string, password: string) =>
  request<{ email: string }>("/auth/password-reset/confirm", { method: "POST", body: JSON.stringify({ token: resetToken, password }) });
export const acceptInvitation = (inviteToken: string, password: string) =>
  request<{ email: string }>("/auth/invitations/accept", { method: "POST", body: JSON.stringify({ token: inviteToken, password }) });
/** Permanently deletes a user (not yourself, and not the last active admin). */
/** Deactivate (signs the user out everywhere; reversible) or reactivate an account. */
export const setUserActive = (token: string, userId: string, active: boolean) =>
  request<ManagedUser>(`/users/${encodeURIComponent(userId)}/${active ? "activate" : "deactivate"}`, { method: "POST", token });

// --- Audit log ------------------------------------------------------------------
/** Mirrors audit.Action in the API (api/internal/audit). */
export const AUDIT_ACTIONS = [
  "login_succeeded", "login_failed", "logout",
  "user_invited", "invitation_resent", "invitation_accepted",
  "roles_changed", "user_deactivated", "user_activated", "user_deleted",
  "password_reset_requested", "password_reset_completed",
] as const;
export type AuditAction = (typeof AUDIT_ACTIONS)[number];
export type AuditEvent = {
  id: number;
  occurred_at: string;
  action: AuditAction;
  actor_user_id: string | null;
  actor_email: string | null;
  target_user_id: string | null;
  target_email: string | null;
  details: { roles_before?: string[]; roles_after?: string[]; reason?: string };
  ip: string | null;
  user_agent: string | null;
};
export type AuditQuery = { action?: AuditAction | ""; q?: string; limit: number; offset: number };
/** One page of the audit log, newest first (needs audit:read). */
export function fetchAuditEvents(token: string, query: AuditQuery) {
  const params = new URLSearchParams({ limit: String(query.limit), offset: String(query.offset) });
  if (query.action) params.set("action", query.action);
  if (query.q?.trim()) params.set("q", query.q.trim());
  return request<Page<AuditEvent>>(`/audit/events?${params}`, { token });
}

export const deleteUser = (token: string, userId: string) =>
  request<void>(`/users/${encodeURIComponent(userId)}`, { method: "DELETE", token });
export const setUserRoles = (token: string, userId: string, roles: string[]) =>
  request<ManagedUser>(`/users/${encodeURIComponent(userId)}/roles`, { method: "PUT", body: JSON.stringify({ roles }), token });
