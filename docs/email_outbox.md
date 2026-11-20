# Durable email delivery

Invitation, resend and password-reset requests now save an `email_outbox` job in
the same PostgreSQL transaction as the user/token change. A failed insert rolls
everything back. The HTTP response does not wait for SMTP.

## Running it

Start PayCore with `./run.ps1` from the project root. On API startup, migration
`015_email_outbox.sql` creates the table, and four background workers start inside
the API process. The existing SMTP configuration is used; no new credentials or
service are required. Restart an already-running API to load this code.

The local inbox is Mailpit at <http://localhost:8025>. SMTP acceptance means the
mail server accepted the message; it does not prove delivery to a remote inbox.

## Processing and recovery

- Workers poll once per second when idle. `FOR UPDATE SKIP LOCKED` lets multiple
  workers/API processes claim different jobs safely.
- Each claim has a unique lease ID and a two-minute expiry. A restarted process
  recovers an unfinished job after its lease expires. Late workers cannot overwrite
  a newer claim's result.
- SMTP has a 45-second deadline; database operations have five-second deadlines.
- Temporary failures retry after 10 seconds, one minute, five minutes and fifteen
  minutes, up to five claims. Permanent SMTP 5xx replies stop immediately. Crashes
  also consume a claim, so repeated crashes cannot retry a job indefinitely.
- Used, expired, superseded or deactivated-user tokens are canceled before claim.
  An already-running SMTP send can race with revocation; the token remains subject
  to the usual validation when the link is opened.
- Outcomes are `pending`, `sending`, `sent`, `failed`, `not_configured` or `canceled`.
  Invitation status and attempt count are updated in the same transaction as the
  outbox result. Reset requests keep their existing generic public response.

Delivery is **at least once**. A crash after SMTP acceptance but before the outcome
commit can cause a duplicate email. SMTP and PostgreSQL cannot share a transaction;
the emailed token is still single use.

## Inspecting jobs

Use HeidiSQL or another PostgreSQL client connected to the `paycore` database:

```sql
SELECT id, kind, status, attempts, available_at, lease_until,
       last_error, sent_at, created_at
FROM email_outbox
ORDER BY created_at DESC;
```

Error fields contain fixed operational codes rather than raw SMTP responses.
Pending email bodies contain sensitive one-time links. They are cleared when a
job finishes, fails or is canceled. Protect database access and backups accordingly;
the application does not encrypt these bodies itself. Metadata remains for
troubleshooting. No email body or recipient is added to worker logs. The existing
development-only log sender intentionally previews complete email content.

Without a configured sender, new jobs are recorded as `not_configured` with no
body. Development log previews also end as `not_configured`. Configure SMTP and
resend an invitation or request a new reset link to deliver a fresh token.

Jobs created by the previous memory queue cannot be reconstructed after they are
lost: the database only retained token hashes. Resend those invitations or request
new reset links after upgrading.

## Verification

Repository integration tests use isolated schemas through `DATABASE_TEST_URL` and
cover rollback on outbox failure, concurrent claims, lease recovery/fencing,
scheduled retries and stale-token cancellation. Set `SMTP_TEST_HOST=127.0.0.1` to
include the durable invitation delivery test against local Mailpit on port 1025.
