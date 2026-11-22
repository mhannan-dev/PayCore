# Admin MFA with an authenticator

When `MFA_ENABLED=true` (the default), every account with the `ADMIN` role must complete TOTP enrollment and verification
before receiving a session. Enabled MFA continues to apply if that account later
loses its ADMIN role. Ordinary accounts without enrolled MFA keep password/CAPTCHA
login. There is no email/SMS fallback or HTTP endpoint that disables MFA.

## Temporary local pause

MFA enforcement is currently paused in this workspace's ignored `api/.env`:

```dotenv
MFA_ENABLED=false
```

Restart the API through `./run.ps1`, return to the login form if an old QR challenge
is open, and sign in with email/password and CAPTCHA. The setting applies to all
accounts, including already enrolled users. Password verification, CAPTCHA,
session expiry, user activation checks and RBAC continue to apply.

Existing authenticator secrets, encryption key and recovery codes stay intact.
The Account security page displays **Paused**. Authenticator replacement, code
regeneration and MFA challenge verification are paused; session revocation still
requires the current password. Password-only sessions remain `mfa_verified=false`.

To restore enforcement, set `MFA_ENABLED=true` (or remove it) and restart the API.
Password-only sessions for Admin or enrolled accounts then stop authorizing
requests; those accounts must sign in again and complete MFA. Ordinary accounts
without enrollment continue using password/CAPTCHA login. The sample environment
keeps the secure default enabled; invalid flag values stop startup.

## Local setup

The API requires `MFA_ENCRYPTION_KEY`, containing 32 random bytes encoded as base64.
Generate it once from `api`:

```powershell
go run ./cmd/mfa-key
```

The command saves the key to the ignored `api/.env`, preserves an existing key and
does not print it. The local key has already been provisioned for this workspace.
In production, supply the same key to every replica through your secret manager.
Back it up securely and separately from the database; never regenerate it to fix
an existing deployment. Losing/changing it makes stored authenticator secrets
unreadable. Key rotation and controlled administrator reset are separate follow-up work.

Restart the API through `./run.ps1`. Migration 017 creates MFA state, challenge and
recovery-code tables, adds a verified-MFA marker to sessions, extends audit actions
and revokes older password-only admin sessions. Admins must sign in again.

## First admin login

1. Enter email, password and CAPTCHA normally.
2. Scan the displayed QR code in an authenticator app. A manual setup key is also
   available. The QR is generated inside the API; no external QR service receives
   the secret.
3. Enter the app's six-digit code. Only a correct code enables MFA and creates a
   session; a password-stage challenge cannot access other API endpoints.
4. Download/store the eight recovery codes privately, then continue to the
   workspace. They are displayed once, and each works once. Do not put them in
   screenshots, logs or shared documents.

Later logins ask for a fresh authenticator code or one unused recovery code after
password/CAPTCHA verification. Recovery codes are 32 hexadecimal characters. A
password reset does not turn off MFA or replace recovery codes. Losing both the
authenticator and all recovery codes requires controlled administrator recovery; no
insecure self-service bypass is included.

The challenge, enrollment secret, QR and recovery codes remain in component
memory; they are never put in URLs or browser storage by this flow. Only the
completed session follows the existing remember-device storage preference. HTTPS
is required outside local development.

## Account Security page

Open **Account security** in the sidebar, or visit `/security`. The page displays
MFA enrollment, enrollment time, unused recovery-code count and active sessions.
It is self-service for every signed-in user, including users without administrative
permission grants. Session records expose only opaque IDs, creation/expiry times,
the MFA-verification marker and a current-session flag; bearer tokens and their
hashes are never returned. The current session is shown first, followed by the
newest others, up to 100 total. A flag indicates additional sessions; bulk revocation
also removes sessions not shown.
Multiple tabs sharing the same bearer token are one session. Browser/device
details and last-seen timestamps are not collected or invented by this feature.

Every change requires the current password. Enrolled accounts must also provide a
fresh TOTP or unused recovery code. Codes already used for login/another change
cannot be reused; wait for a fresh authenticator code when necessary. Accounts
without enrolled MFA can revoke sessions using their password, but cannot generate
MFA recovery codes or replace an unenrolled authenticator.

- **Regenerate recovery codes:** immediately invalidates all previous codes,
  returns eight new codes once, and cancels pending MFA login/replacement setup.
- **Replace authenticator:** first verifies the current password and existing
  factor, then returns a new QR/setup key. The old authenticator remains active.
  Confirm within five minutes, using the current password and a code from the
  **new** authenticator. Confirmation is bound to the session that started setup
  and the current password hash. It replaces all recovery codes, invalidates
  pending MFA logins and revokes every other session while preserving the current
  one. A canceled/expired setup never changes the active authenticator.
- **Revoke sessions:** revoke an individual other session or all other sessions.
  Other users' session IDs return 404. The current session cannot be revoked here;
  use the existing Sign out action. Revocation takes effect on the next API request.

Recovery-code regeneration, replacement and session revocation record their audit
event inside the same transaction. Audit-write failure rolls back the change and
proof consumption. Failed security proofs share the MFA account lockout counter:
five failures lock further verification attempts for five minutes. All endpoints
also use the existing per-IP auth rate limit. Secrets, passwords and submitted
codes are never audit details.

Migration `018_account_security.sql` adds opaque session IDs, encrypted pending
replacement state and the audit actions `mfa_recovery_regenerated`,
`mfa_replacement_started`, `mfa_replaced`, and `sessions_revoked`. It does not
disable MFA or change the encryption key. Restart the API to load the endpoints.

| Method | Endpoint | Body / result |
| --- | --- | --- |
| GET | `/api/v1/auth/security` | MFA status, recovery count and active session list |
| POST | `/api/v1/auth/security/recovery-codes` | `{password, code}` → `{recovery_codes}` |
| POST | `/api/v1/auth/security/authenticator` | Current `{password, code}` → `{enrollment, expires_in}` |
| POST | `/api/v1/auth/security/authenticator/confirm` | `{password, code}` from the new app → `{recovery_codes, revoked_count}` |
| POST | `/api/v1/auth/security/sessions/:id/revoke` | `{password, code}` → `{revoked_count}` |
| POST | `/api/v1/auth/security/sessions/revoke-others` | `{password, code}` → `{revoked_count}` |

Every endpoint requires the real bearer session, revalidated inside the database
transaction. Owner IDs and action names cannot be supplied in the body. Responses
are `no-store`. Pending replacement data is encrypted and expires after five
minutes; deleting its starting session also removes the pending setup. The web
page keeps QR/recovery data in component memory and clears it when the session
changes. Only an explicit download saves recovery codes to a file.

If a response is lost after a change commits, refresh the page before retrying.
Use a fresh code from the active authenticator to regenerate recovery codes if
their one-time response was lost. Authenticator replacement preserves the current
session but does not extend its expiry.

## API contract

`POST /api/v1/auth/login` returns the existing session response for an ordinary
account, or a challenge after successful password/CAPTCHA verification:

```json
{
  "state": "mfa_enrollment_required",
  "challenge_token": "<random 64-hex challenge>",
  "expires_in": 300,
  "enrollment": { "secret": "<base32 setup key>", "uri": "otpauth://...", "qr_image": "data:image/png;base64,..." }
}
```

For an enrolled account, `state` is `mfa_required` and `enrollment` is null. No
session token or public user is returned at this stage.

Submit `POST /api/v1/auth/mfa/verify` with
`{"challenge_token":"…","code":"…"}`. Success returns
`{"session":<existing login response>,"recovery_codes":[...]}`. The code array is
populated only when enrollment first completes; other successful verifications
return an empty array. Both stages have `Cache-Control: no-store`.

Invalid/consumed/expired challenges return `401 mfa_challenge_invalid`; wrong or
replayed codes return `401 mfa_code_invalid`. Five failed codes across successive
challenges lock that account for five minutes (`429 mfa_locked`, `Retry-After:
300`). The public verification endpoint also has the existing per-IP auth limit.
Starting a new challenge replaces the previous one but does not reset failures.
Challenges expire after five minutes and are limited to one per account.

## Security and concurrency

- TOTP uses the [RFC 6238](https://www.rfc-editor.org/rfc/rfc6238) interoperable
  HMAC-SHA1 profile with six digits and a 30-second step, allowing one neighboring
  step for clock skew. Servers and authenticator devices need accurate clocks.
  A previously accepted time step cannot be reused.
- Secrets (including pending enrollment secrets) are AES-256-GCM encrypted with
  random nonces and user-ID authenticated data. Only recovery-code SHA-256 hashes
  are stored; their 128-bit random values do not rely on password-like entropy.
- Verification locks the user, MFA state and challenge; consuming a TOTP/recovery
  code, enabling MFA and creating the session commit together. Concurrent uses of
  the same challenge/recovery code cannot create two sessions.
- Challenges bind to the password hash verified at issuance. A password change,
  deactivation, expiry or prior consumption prevents their completion.
- The ordinary session-creation repository refuses accounts requiring MFA.
  Session reads reject password-only sessions for newly promoted admins and
  enrolled users, so existing sessions cannot bypass the new requirement.
- MFA events are `mfa_enrolled`, `mfa_verified`, `mfa_failed` and
  `mfa_recovery_used`. Secrets and submitted codes are never audit details.

## Verification

Run `cd web; npm run check; npm run test:rbac` and the required API vet/tests.
Set `DATABASE_TEST_URL` for isolated PostgreSQL integration tests. Coverage includes
RFC vectors, encrypted-secret user binding, QR enrollment, mandatory admin MFA,
challenge/session separation, single-use codes, concurrent recovery, account
lockouts, password-change invalidation, expiry and role-promotion bypass prevention.
