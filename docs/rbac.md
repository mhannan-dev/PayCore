# Role-based access control

Roles, permissions and their mapping live in migration `008_rbac.sql` (kept current by `seed-users`). A user's roles are `users.roles`, mirrored in `user_roles`; their permissions are the union of what those roles grant.

## Enforcement audit (19 November 2026)

- All 28 permission-protected API routes have negative authorization tests: no
  session returns `401`; every grant except the required one returns `403`, even
  when the identity has an `ADMIN` role label. New routes must have an explicit
  policy or a documented public/self-service exception in the coverage test.
- File routes now require `attachments:read`, `attachments:write` or
  `attachments:delete` (migration `016_attachment_permissions.sql`). These are
  **own-file capabilities**; the service and database still enforce ownership.
  All five existing roles receive these baseline grants to preserve their own-file
  workflows. Revoking a grant is enforced on the next API request.
- `Guard.Require` accepts a typed permission code and denies empty or unknown
  requirements. Grants match exactly; malformed case/whitespace is not normalized
  into authorization. The role name itself never bypasses permission checks.
- Repository role/permission lookups now use `users.roles`, matching authentication
  and role counts. A stale `user_roles` mirror cannot add an effective role.
  Unused role mutators without last-admin checks were removed; normal role changes
  use `SetUserRoles` with its existing transaction, audit and last-admin safeguards.
- Protected web controls deny access while grants are unknown. Nested web pages
  inherit their closest protected parent's permission, using path boundaries.

### Default role matrix

`Y` means granted by the built-in policy. Multiple roles combine their grants.
Existing custom grants are preserved; the seeder explicitly restores default
grants when run, so do not use it to preserve intentional default-role revocations.

| Permission | ADMIN | OPERATOR | LEAD_OPERATOR | COMPLIANCE_AUDITOR | STAFF_ENGINEER |
| --- | --- | --- | --- | --- | --- |
| users:read | Y | — | Y | — | — |
| users:write | Y | — | — | — | — |
| accounts:write | Y | — | Y | — | — |
| ledger:read | Y | Y | Y | Y | Y |
| transfers:read | Y | Y | Y | Y | — |
| transfers:create | Y | Y | Y | — | — |
| transfers:reverse | Y | — | Y | — | — |
| audit:read | Y | — | Y | Y | Y |
| audit:verify | Y | — | — | Y | — |
| reports:read | Y | — | Y | Y | — |
| reports:export | Y | — | — | Y | — |
| simulator:run | Y | — | — | — | Y |
| system:read | Y | — | — | — | Y |
| system:write | Y | — | — | — | Y |
| attachments:read | Y | Y | Y | Y | Y |
| attachments:write | Y | Y | Y | Y | Y |
| attachments:delete | Y | Y | Y | Y | Y |

`GET /api/v1/outbox/metrics` requires `system:read`; `POST /api/v1/outbox/replay`
requires `system:write` (migration `021_outbox_delivery.sql`). Both ADMIN and
STAFF_ENGINEER receive the new write grant, and every replay is audited.

Health endpoints and CAPTCHA/login/invitation/reset entry points remain public
(token-entry endpoints retain validation and rate limits). `/auth/me`, self-name
updates and logout require the caller's session without an administrative
capability. Revoking all capabilities does not prevent checking identity or
signing out. Report/simulator/system capabilities describe policy for those
features; they do not create new API endpoints.

Admin login now requires [authenticator MFA](admin_mfa.md). The public
`POST /api/v1/auth/mfa/verify` accepts only a short-lived challenge issued after
password/CAPTCHA verification and is rate limited; it never accepts a role or
permission from the client. A challenge is not a bearer session.

The six `/auth/security` endpoints are authenticated self-service exceptions, not
administrative capabilities. They derive the owner from the bearer session and
revalidate it under database locks. Mutations require the current password and
fresh MFA when enrolled. They never list or revoke another user's sessions, and
provide no MFA-disable endpoint. See the Account Security section in the MFA docs.

### Verification

Run the repository checks plus `cd web; npm run test:rbac`. With
`DATABASE_TEST_URL` set, `go test ./...` also verifies that migration and seeder
matrices match, live grant revocation affects an existing session, another role
can supply the missing grant, deactivation revokes access, and a corrupted legacy
mirror cannot elevate permissions. The existing attachment integration test
continues to reject cross-user reads, downloads, photo assignment and deletion.

## API

`POST /auth/login` and `GET /auth/me` now return `permissions`, e.g. `["ledger:read", "transfers:read"]`. Permissions are read from the database on every request, so a role change applies on the user's next request.

| Method | Path | Permission | Notes |
| --- | --- | --- | --- |
| GET | `/api/v1/rbac/roles` | `users:read` | `{"items": [{code, name, description, permissions, user_count}]}` |
| GET | `/api/v1/rbac/permissions` | `users:read` | `{"items": [{code, name, description}]}` |
| GET | `/api/v1/users` | `users:read` | Paged: `?q=` (name/email, case-insensitive, max 100 chars), `?role=`, `?limit=` (1–100, default 20), `?offset=` → `{items, limit, offset, total}`, ordered by email |
| POST | `/api/v1/users` | `users:write` | Invites a user. Body `{first_name, last_name?, email, roles}`; returns `{user, email_status}` (201). The account has no password until the user accepts the emailed link |
| POST | `/api/v1/users/:id/invitation` | `users:write` | Re-sends the invitation with a new link (earlier links stop working). `409 already_activated` once a password is set |
| POST | `/api/v1/auth/invitations/inspect` | public, rate limited | Body `{token}` → `{email, first_name, expires_at}`, or `404 invitation_invalid` |
| POST | `/api/v1/auth/invitations/accept` | public, rate limited | Body `{token, password}` (exactly 10 characters) sets the password once → `{email}` |
| PUT | `/api/v1/users/:id/roles` | `users:write` | Body `{"roles": ["OPERATOR"]}` replaces the role set |
| DELETE | `/api/v1/users/:id` | `users:write` | Permanently deletes the user (204); sessions, roles, invitations, contacts and attachment records cascade. `409 cannot_delete_self`, `409 last_admin` |
| POST | `/api/v1/users/:id/deactivate` | `users:write` | Blocks sign-in and signs the user out everywhere; account and history are kept. `409 cannot_deactivate_self`, `409 last_admin` |
| POST | `/api/v1/users/:id/activate` | `users:write` | Allows the user to sign in again |
| GET | `/api/v1/audit/events` | `audit:read` | Paged audit log, newest first: `?action=`, `?q=` (actor/target email), `?limit=` (1–200, default 50), `?offset=` |
| GET | `/api/v1/accounts`, `/api/v1/accounts/:id`, `/api/v1/accounts/:id/postings` | `ledger:read` | Chart of accounts, balances and postings |
| POST | `/api/v1/accounts` | `accounts:write` | Create ledger account |
| GET | `/api/v1/transfers`, `/api/v1/transfers/:id` | `transfers:read` | Transfer history and journal detail |
| POST | `/api/v1/transfers` | `transfers:create` | Post a transfer |
| POST | `/api/v1/transfers/:id/reverse` | `transfers:reverse` | Compensating journal |
| GET | `/api/v1/audit/zero-sum` | `audit:verify` | Verify zero-sum invariant |
| GET | `/api/v1/attachments`, `/api/v1/attachments/:id`, `/api/v1/attachments/:id/content`, `/api/v1/auth/photo` | `attachments:read` | Own metadata and bytes |
| POST / PUT | `/api/v1/attachments` / `/api/v1/auth/photo` | `attachments:write` | Upload own file / assign or clear own photo |
| DELETE | `/api/v1/attachments/:id` | `attachments:delete` | Soft-delete own file |

Errors use the standard envelope: `400 invalid_user` and `409 email_taken` (invite), `401 invalid_credentials`, `403 permission_denied`, `400 invalid_roles` (empty or unknown roles), `404 user_not_found`, `409 cannot_remove_own_admin`, and `409 last_admin` (the change would leave no active ADMIN; role changes are serialized with an advisory lock).

## Web

- `web/src/lib/rbac.ts`: permission codes, `ROUTE_PERMISSIONS` (route → required permission) and `hasPermission`.
- `PageAccess` blocks a route whose permission the user lacks and shows `AccessDenied`; the sidebar hides those routes.
- `<Can permission="…">` and `usePermission()` hide individual controls.
- `/admin/users`: list users, filter by role, invite users, re-send invitations, edit roles and delete users (needs `users:write`; read-only otherwise).
- `/admin/roles`: role cards and the permission matrix.

The UI checks are for display only; the API enforces every permission.

## Invitations

1. An administrator adds a user on `/admin/users`. The API creates the account without a password, stores the SHA-256 of a random 64-hex token (`user_invitations`, migration 009) and an email job in one transaction, then emails `APP_URL/set-password#token=…`.
2. The link is valid for 72 hours and works once. Re-sending replaces it. The token is in the URL fragment, so browsers never send it to a server or in a Referer header, and the API receives it only in request bodies, never in a logged URL.
3. `/set-password` shows who the invitation is for, the user chooses a password (exactly 10 characters), and they then sign in normally. Until then, login fails like a wrong password (with the same bcrypt cost, so timing does not reveal invited accounts).

Email settings (API environment or `api/.env`): either `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_FROM`, `SMTP_ENCRYPTION`, or the Laravel-style `MAIL_HOST`, `MAIL_PORT`, `MAIL_USERNAME`, `MAIL_PASSWORD`, `MAIL_ENCRYPTION`, `MAIL_FROM_ADDRESS`, `MAIL_FROM_NAME`. If `SMTP_HOST` is set only the `SMTP_*` set is used, otherwise only `MAIL_*`; the two are never mixed. Encryption is `starttls` (default; required, never opportunistic), `tls` (port 465) or `none` (loopback servers only, e.g. Mailpit). Credentials use AUTH PLAIN or AUTH LOGIN (Microsoft 365) and are only sent over TLS. The sender defaults to the username when it is an address. Without a mail server, development writes the email, including the link, to the API log. `APP_URL` (default `http://localhost:3000`) is the link's origin.

Check the settings without starting the API: `cd api; go run ./cmd/mail-check` connects, encrypts and logs in without sending; add `-to you@example.com` to send a test email.

Delivery is asynchronous: "Add user" and "Resend invite" return `email_status: "pending"` (or `"not_configured"`) after committing the user/token and `email_outbox` job together (migration 015). Four API workers send persisted jobs with up to five claims and retry delays of 10 seconds, one minute, five minutes and fifteen minutes. SMTP 5xx failures stop immediately. Outcomes update the invitation's delivery fields in the same transaction as the job result. A restart recovers unfinished work after a two-minute lease expires. See [durable email delivery](email_outbox.md) for status queries, recovery and duplicate-delivery limits.

For local development, `run.ps1` starts Mailpit (`compose.yaml`) and points the API at it: open http://localhost:8025 to see invitation emails.

The user's `invite_email_status: "sent"` means SMTP accepted the email; it does not confirm delivery to a remote inbox. With Mailpit, it appears only in the local inbox. Development log previews are `not_configured`. A blank `SMTP_HOST=` does not disable automatic Mailpit startup. An explicit `APP_URL` in the API `.env` is preserved for links.

## Forgot password

1. `/login` links to `/forgot-password`, which asks for the email and the math captcha and calls `POST /api/v1/auth/password-reset/request` `{email, captcha_id, captcha_answer}`. The answer is always `202` with the same message, whether or not an active account has that email, so the endpoint cannot be used to discover accounts. One request per account per minute issues a link; a new link replaces the previous one.
2. The email (sent in the background through the same mail outbox as invitations, with retries) links to `APP_URL/reset-password#token=…`. The link works once and expires after 30 minutes; its SHA-256 is stored in `password_reset_tokens` (migration 011). Pending outbox bodies contain the one-time link until the job completes, fails or is canceled; protect database access and backups.
3. `/reset-password` checks the link (`POST …/password-reset/inspect` `{token}`) and sets the new password (`POST …/password-reset/confirm` `{token, password}`, exactly 10 characters). Confirming **signs the user out of every session**, invalidates other reset links and closes any pending invitation. Errors: `400 invalid_captcha`, `400 invalid_password`, `404 reset_invalid`.

All three endpoints are public, rate limited (`AuthRateLimit`) and `Cache-Control: no-store`; tokens travel only in request bodies and URL fragments, never in logged URLs.

## Audit log

`audit_events` (migration 013) is an append-only record of who did what, to whom, when and from where (IP and user agent). The database rejects `UPDATE`, `DELETE` and `TRUNCATE` on it, and it has no foreign keys, so deleting a user never alters their history.

| Action | Recorded when | Details |
| --- | --- | --- |
| `login_succeeded`, `logout` | a session starts or ends | |
| `login_failed` | a sign-in is rejected (after a valid CAPTCHA) | `reason`: `unknown_email`, `wrong_password`, `password_not_set` or `account_deactivated`; the API answer is the same for all |
| `user_invited`, `invitation_resent`, `invitation_accepted` | invitation lifecycle | `roles_after` for invites |
| `roles_changed` | an admin changes roles | `roles_before`, `roles_after` |
| `user_deactivated`, `user_activated`, `user_deleted` | an admin changes the account | `roles_before` for deletes |
| `password_reset_requested`, `password_reset_completed` | forgot-password flow (requests only when a link is issued) | |

Role changes, deactivation, activation and deletion are written **in the same transaction** as the change, so a change cannot happen without its audit entry. Other events are written right after they succeed; a failure to record them is logged but does not fail the request. The acting user comes from the authenticated request, and the IP from the connection (no proxy headers are trusted).

The console shows the log at `/admin/audit-log` (needs `audit:read`: Admin, Lead operator, Compliance auditor, Staff engineer).

Deactivating a user (`users:write`) blocks sign-in, deletes their sessions and unused reset links, and keeps the account, roles and history; you cannot deactivate yourself or the last active Admin.


Notification delivery history requires `notifications:read` (all five default roles). Manual retries require `notifications:retry` (ADMIN and STAFF_ENGINEER). A retry can only requeue a failed delivery event using its current attempt count; it does not move funds.
