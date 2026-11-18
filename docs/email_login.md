# Email, password and matching CAPTCHA login

## User model

`api/internal/model/user.go` defines `User`: UUID `ID`, required `FirstName`, nullable `LastName` and `Photo`, normalized unique `Email`, `PasswordHash`, stored `Roles []string`, `IsActive`, `CreatedAt`, and `UpdatedAt`. `PasswordHash` has `json:"-"`; API responses use `UserDTO` and never return the hash. A login identity is separate from a financial ledger account.

The SQL profile uses `first_name`, nullable `last_name`, and nullable `photo_attachment_id`; there is no `name` field. Photos share the unified [attachments table](attachments.md). API responses retain a derived `photo` URL and include `photo_attachment_id`. Migration 002 preserves an existing full display name as `first_name` without guessing how to split it. Migration 003 removes the redundant generated `name` column. Numbered `*.sql` files are automatically discovered and applied in version order.

The API now checks stored credentials. It no longer accepts arbitrary passwords, invents users on login, or assigns roles by matching email substrings. There are no built-in users or default passwords.

To populate sample identities explicitly, use the [users seeder](user_seeder.md). It prompts for a password and skips existing emails without modifying them.

## Database and user creation

Start the local database and API with `./run.ps1`. Startup applies embedded, versioned SQL migrations for `users` and `auth_sessions`, under a transaction-scoped advisory lock. `-ListRoutes` stays offline and does not apply migrations.

Create a user from a separate interactive PowerShell terminal:

```powershell
cd api
go run ./cmd/create-user -email operator@example.test -first-name "Ledger" -last-name "Operator" -roles "OPERATOR,STAFF_ENGINEER"
```

Omit `-last-name` to store NULL. The command prompts for a password and confirmation without echoing them. Passwords must be exactly 10 characters; bcrypt cost 12 is used. Passwords are not accepted as CLI arguments. The creator must have database access; this is an operator tool, not public registration. Duplicate emails are rejected. `-roles` takes a comma-separated list (`-role` remains an alias); values are normalized to uppercase and duplicates removed. At least one role is required. Supported roles are `OPERATOR`, `ADMIN`, legacy `LEAD_OPERATOR`, `COMPLIANCE_AUDITOR`, and `STAFF_ENGINEER`. Roles never come from login input.

Migration 005 changes the old scalar `role` into `roles TEXT[]`, preserving each existing value as a one-item list. Database constraints reject empty arrays, unknown roles, NULL elements and duplicates. Login and `/auth/me` return `roles`, for example `"roles": ["ADMIN", "OPERATOR"]`; the old `role` response field is removed. Sessions read current database roles on each `/me` request. `User.HasRole` checks membership; this change does not introduce public role-editing endpoints or a financial permissions policy.

## API flow

1. `GET /api/v1/auth/captcha` returns `captcha_id`, a PNG data URL in `image`, and `expires_in: 120`. Display the image and let the user type its six digits. The answer is not included in the response.
2. Submit `POST /api/v1/auth/login` with JSON:

```json
{
  "email": "operator@example.test",
  "password": "<the user's password>",
  "captcha_id": "<64-character challenge ID>",
  "captcha_answer": "<six digits read from the image>"
}
```

3. Success returns `token`, `token_type: Bearer`, `expires_in: 86400`, and the stored public user. Only a SHA-256 digest of the cryptographically random token is stored in PostgreSQL.
4. Send `Authorization: Bearer <token>` to `GET /api/v1/auth/me`. It checks the stored session, database expiry, and active-user flag.
5. `POST /api/v1/auth/logout` with the same header deletes that session. The token then fails `/me`. Other sessions are unaffected.
6. `PATCH /api/v1/auth/me` with the same header and `{"first_name": "Kenji", "last_name": "Sato"}` updates the signed-in user's name and returns the updated user. Names are trimmed; `first_name` is required (1–80 characters); a blank or `null` `last_name` clears it. Other fields in the body (email, roles) are ignored. Returns `400` for invalid names and `401` without a valid session.

Validly formatted CAPTCHA attempts consume the challenge atomically, even when wrong. Password failures also require a fresh challenge. Missing/malformed fields return `400`; wrong, expired, or used CAPTCHA returns `400`; incorrect credentials, unknown email, or inactive user return the same `401`. Invalid/revoked/expired sessions return `401`; unavailable storage returns `503`. Login bodies are limited to 8 KiB, and authentication responses are not cacheable.

## Scope and deployment

- Images use `github.com/dchest/captcha`; IDs/digits use `crypto/rand`. Answer digests stay in a bounded, expiring process-local store. Restart invalidates outstanding challenges; the issuing instance must verify them.
- Auth routes share a bounded limit of 20 requests per client IP per minute, returning `429` and `Retry-After`. Untrusted forwarding headers cannot supply the client IP. CAPTCHA/rate-limit stores must be shared before multiple replicas are used; this is not distributed abuse protection.
- Image CAPTCHA adds an automation barrier, not proof of humanity. Accessible alternatives and stronger abuse controls remain future work.
- `WEB_ORIGINS` is a comma-separated browser-origin allowlist, defaulting to localhost/127.0.0.1 on port 3000. Update it for a different web port. Wildcard credentialed CORS was removed. Session checks also apply to clients without a browser Origin header.
- Deploy with HTTPS. No browser cookie, refresh-token, email-verification, password-reset, or MFA flow is implemented. Expired session rows should be purged periodically; expiry is enforced before cleanup.
- The web app signs in through `/auth/captcha` + `/auth/login`, keeps the bearer token in `localStorage` ("Remember this terminal session") or `sessionStorage`, re-checks it with `/auth/me` on load and tab focus, and calls `/auth/logout` on Logout. Set `NEXT_PUBLIC_API_URL` if the API is not on `http://localhost:8080` (`run.ps1` sets it for the web job from the API port it actually chose; override with `PAYCORE_API_URL`). Workspace pages still show sample data.

## Verification

```powershell
cd api
go test ./...
$env:DATABASE_TEST_URL = 'postgres://paycore:paycore_local_dev@127.0.0.1:5434/paycore?sslmode=disable'
go test ./internal/repository -run TestUserSessionPostgresIntegration -v
```

Tests cover single-use/expired/concurrently consumed CAPTCHA, password rejection, inactive/unknown users, token hashing/revocation, JSON hash exclusion, spoofed proxy headers, CORS, and database session expiry. Integration fixtures are removed after the test.
