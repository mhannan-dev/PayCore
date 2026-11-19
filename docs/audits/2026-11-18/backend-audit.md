# PayCore Go/Gin production-readiness audit

Date: 18 November 2026. Scope: the current identity, administration, attachment,
mail, audit, storage and PostgreSQL code. This is not certification of a payment
engine: the inspected router does not expose implemented transfer/ledger business
APIs. Application fixes were not applied as part of this review.

The source changed during the review: service interfaces, `UserAdminService`,
separate users/invitation/audit handlers, `Guard`, request IDs and central error
mapping were added. This report recognizes those improvements rather than
reporting the earlier concrete-service dependencies and repetitive handler error
switches as current defects. Intermediate compilation failures are not treated
as lasting findings. Revalidate these observations after subsequent edits.

Severity: **Critical** means a demonstrated release-blocking security defect;
**Warning** means a correctness, resilience or security-hardening gap;
**Optimization** means a maintainability/scaling improvement.

## Executive assessment

Do not approve production deployment until C1 is fixed and covered by a
deterministic concurrent PostgreSQL integration test. The foundation is useful:
parameterized SQL, consumer-side interfaces, opaque hashed session/reset tokens,
fresh permission checks, confined upload storage, database-backed ownership and
transactional administrator invariants. Those do not eliminate the lifecycle
and resource-management findings below.

| Dimension | Assessment |
|---|---|
| Architecture / SRP | Auth and user administration are now substantially layered; attachments still combine transport, validation, filesystem orchestration and repository calls. |
| Error handling / OCP | Response helpers plus `middleware.Errors` and `server.ErrorTable` centralize mapping. Causal information and safe logging still need improvement. |
| Concurrency / performance | SMTP implements connection cancellation and deadlines. Detached audit writes, unbounded mail jobs and conversion admission remain concerns. |
| Security / validation | No SQL injection or direct Gin-context leakage into services/repositories was found. Password reset has a reproduced session-creation race; CAPTCHA, password policy and strict binding need hardening. |
| Testability / DIP / ISP | Consumer-side interfaces and isolated-schema PostgreSQL tests are strong. Image conversion/global admission and post-reset session issuance need dedicated seams and regression tests. Avoid creating interfaces for every concrete type without a testing/substitution need. |

Examples below are proposed refactoring sketches, not patches or complete
drop-in implementations. Names introduced in “After” examples are proposed APIs.

## Critical

### C1 — Old-password login can create a session after password reset

Evidence: `internal/service/auth.go` (`Login`),
`internal/repository/user_repository.go` (`CreateSession`, around line 68), and
`internal/repository/password_reset_repository.go` (`ResetPassword`, around line 67).

`Login` reads a password hash, verifies it, and later issues a session.
`ResetPassword` changes the password and deletes existing sessions in a separate
transaction. `CreateSession` only checks that the user is active. This ordering
therefore succeeds:

1. Login verifies the old password.
2. Password reset commits and deletes all currently existing sessions.
3. The old-password login inserts a new, usable session.

**Reproduced against actual repositories in a temporary schema:**
`credential_reset_race_reproduced=true`. The probe injected a password reset
immediately before session insertion, after the real bcrypt verification. Real
users were not changed, and the schema was removed. The probe is preserved in
`password-reset-race.go.txt` beside this report.

This requires the previous password and a valid login challenge; it is not a
reset-token bypass. It violates “sign out everywhere” during credential recovery.

**Before:**

```go
match := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), password)
// ...another transaction resets the password here...
err := store.CreateSession(ctx, u.ID, tokenHash, expiry)
```

**After:** issue the session in a transaction that locks the same user row reset
updates, then checks the exact credential snapshot used for verification.

```go
func (r *UserRepository) CreateSessionIfCurrent(
    ctx context.Context, userID, expectedHash string,
    tokenHash []byte, expires time.Time,
) error {
    tx, err := r.pool.Begin(ctx)
    if err != nil { return err }
    defer tx.Rollback(ctx)

    var currentHash string
    var active bool
    err = tx.QueryRow(ctx, `SELECT COALESCE(password_hash, ''), is_active
        FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&currentHash, &active)
    if err != nil { return err }
    if !active || currentHash != expectedHash {
        return ErrCredentialsChanged // map to the normal authentication failure
    }
    _, err = tx.Exec(ctx, `INSERT INTO auth_sessions
        (token_hash,user_id,expires_at) VALUES ($1,$2,$3)`, tokenHash, userID, expires)
    if err != nil { return err }
    return tx.Commit(ctx)
}
```

A credential-version counter is another valid design, but checking a version in
an unlocked `INSERT ... SELECT` alone does not establish the required ordering.
Test both outcomes: login commits first and reset removes its session, or reset
commits first and stale login is rejected. Also test deactivation and reactivation.

## Warning

### W1 — MailOutbox is neither durable nor bounded

Evidence: `internal/service/mail_outbox.go:56`, `invitation.go:84`, and startup in
`cmd/server/main.go`.

Every enqueue spawns a goroutine. Retry configuration is publicly mutable. A
process crash loses the message/link held in RAM; database `pending` status does
not recreate a sendable job because only a token hash is stored. Shutdown stops
retry waits but not an active send; admission is not closed, and requests still
running after HTTP shutdown times out can enqueue work after draining starts.

**Before:**

```go
o.wg.Add(1)
go o.deliver(context.WithoutCancel(ctx), label, msg, record)
```

**After:** create the user/token and a durable delivery job atomically. Encrypt
sensitive job payloads; never introduce plaintext reset tokens into broad logs.

```go
err := unitOfWork.Do(ctx, func(tx InvitationTx) error {
    if err := tx.CreateInvitation(ctx, invitation); err != nil { return err }
    return tx.EnqueueMail(ctx, encryptedJob)
})
// Application-owned context; fixed worker count; bounded attempts/backoff.
workers.Run(appCtx, queue, workerCount)
```

Use leases/`FOR UPDATE SKIP LOCKED`, durable attempt state and replay after
restart. Delivery is at least once; account for duplicate sends rather than
claiming exactly-once SMTP delivery. Stop admission, cancel/drain workers, then
close the database. Add crash/restart and enqueue-during-shutdown tests.

### W2 — Audit writes discard deadlines and can block the request indefinitely

Evidence: `internal/audit/audit.go:76`, called synchronously from auth, invitation
and password-reset services.

`context.WithoutCancel` removes both cancellation and deadlines. A hung database
acquisition/query can outlive the request and exhaust the ten-connection pool.
Some security events are best-effort after the mutation and can be missing even
when an operation succeeds; admin mutations already use transactional audit
inserts, which is the stronger model.

**Before:**

```go
err := rec.Record(context.WithoutCancel(ctx), event)
```

**After, for explicitly best-effort events:**

```go
auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
defer cancel()
err := rec.Record(auditCtx, event)
```

For mandatory security/financial events, insert the event in the mutation's
transaction or a transactional outbox. Monitor audit failures. Byte-slicing the
User-Agent at 300 bytes can split UTF-8; normalize valid text and truncate runes
before writing it to PostgreSQL.

### W3 — Attachment handlers still own business workflows

Evidence: `internal/handler/attachment_handler.go` (`Upload`, `SetPhoto`, around
lines 105 and 190), and HTTP-dependent helpers in `internal/helpers`.

No inline SQL was found in production handlers, and no `*gin.Context` is passed
into service/repository layers. However, the photo handler loads attachment
metadata, decides allowed storage/image formats, opens files, inspects dimensions
and mutates the profile. These rules are difficult to reuse without HTTP and
contradict the project's thin-handler rule. Upload conversion is coupled to HTTP
status codes and a process-global semaphore.

**Before:**

```go
attachment, err := h.repo.Get(ctx, user.ID, id)
file, err := h.files.Open(*attachment.StorageKey)
cfg, format, err := image.DecodeConfig(file)
// decide whether the photo is allowed...
err = h.repo.SetPhoto(ctx, user.ID, &id)
```

**After:**

```go
type PhotoService interface {
    SetPhoto(context.Context, string, *string) (model.PhotoResult, error)
}
// Handler: bind a model.SetPhotoRequest and retrieve the authenticated caller.
result, err := h.photos.SetPhoto(c.Request.Context(), caller.ID, req.AttachmentID)
if err != nil { fail(c, err); return }
response.OK(c, result)
```

The service owns the workflow through consumer-side `AttachmentReader`,
`ImageInspector` and profile-store interfaces. A storage/image component receives
`context.Context` and streams, not Gin or HTTP response writers. Keep streaming
headers and status codes in the handler. Preserve the repository ownership check
and composite foreign key; an ADMIN role must not silently bypass them.

There is also residual driver coupling: auth/invitation/reset services recognize
`pgx.ErrNoRows`. A non-PostgreSQL implementation of their store interface must
emulate that driver error. Prefer a storage-independent domain contract:

```go
// Before: services interpret a PostgreSQL driver's sentinel.
if errors.Is(err, pgx.ErrNoRows) { return model.UserDTO{}, ErrSession }
// After: the repository translates driver errors before returning.
if errors.Is(err, domain.ErrUserNotFound) { return model.UserDTO{}, ErrSession }
```

### W4 — Error mapping improved, but diagnostic causes and safe structured logging remain incomplete

Evidence: `internal/service/auth.go` returns bare `ErrUnavailable` at multiple
infrastructure failures; `internal/middleware/errors.go` and
`internal/service/mail_outbox.go:69` use `log.Printf` with raw errors/addresses;
`internal/mailer/mailer.go:305` logs complete development email bodies.

Centralized JSON serialization and error mapping are now present. Do not replace
the new middleware with another response-helper layer. Instead preserve causes
internally, classify them at the edge, and sanitize logging. A route fallback of
503 also misclassifies programming/data bugs as temporary outages; classify
expected unavailability separately from unexpected 500 errors.

**Before:**

```go
if err != nil { return model.UserDTO{}, ErrUnavailable }
log.Printf("delivery to %s failed: %v", msg.To, err)
```

**After:**

```go
if err != nil {
    return model.UserDTO{}, fmt.Errorf("load session: %w: %w", ErrUnavailable, err)
}
logger.ErrorContext(ctx, "mail_delivery_failed",
    "job_id", job.ID, "attempt", attempt,
    "error_class", classifySMTP(err)) // no body, token, credentials or recipient
```

Use `slog.JSONHandler`, request/trace IDs, route templates and safe error classes.
Keep necessary identity information in the access-controlled audit trail rather
than duplicating it throughout operational logs. Do not dump raw query strings.
Require explicit development mode for token-bearing log previews, and fail
production configuration validation instead of defaulting silently to development.

### W5 — Exact-ten-character passwords and synthetic CAPTCHA are weak enterprise authentication controls

Evidence: `internal/service/auth.go:56`,
`internal/service/captcha.go:93`, `internal/middleware/auth_rate_limit.go:18`.

The password rule prohibits long passphrases. CAPTCHA is not an additional
authentication factor. Its math expression is embedded as ordered SVG `<text>`
elements: rotation/jitter does not hide the expression from software. The
process-local limiter is IP-only, and CAPTCHA state is process-local; replicas
cannot consume one another's challenges without affinity/shared state. Disabling
trusted proxies avoids spoofing, but an ingress deployment must explicitly trust
only its proxies or all clients may share the ingress IP budget.

**Before:**

```go
if utf8.RuneCountInString(password) != 10 { return "", ErrPasswordLength }
captcha := NewCaptchaStore() // process-local math text
```

**After:**

```go
if err := passwordPolicy.Validate(password); err != nil { return "", err }
hash, err := passwordHasher.Hash(password)
auth := NewAuthentication(store, sharedChallengeVerifier, accountAndIPLimiter)
// Require a second factor for privileged production access.
```

As a reference target, NIST SP 800-63B-4 specifies a 15-character minimum for
single-factor passwords and allowing a maximum of at least 64 characters. Apply
a compromised-password blocklist and plan hash migration; bcrypt's 72-byte limit
must not silently truncate long Unicode passwords. This is a policy
recommendation, not a claim that a regulatory obligation was established here.

### W6 — JSON binding is typed and bounded, but not strict

Evidence: `ShouldBindJSON` in auth, invitation and other handlers; no explicit
unknown-field/trailing-document rejection was found in the inspected wiring.

Unknown properties are ignored by default. Typed DTOs avoid mass assignment, so
this is contract hardening, not a demonstrated role-escalation vulnerability.
Strict decoding makes spelling mistakes and prohibited fields visible. Re-run
binding-tag validation after introducing a custom decoder.

**Before:**

```go
err := c.ShouldBindJSON(&req)
```

**After, inside one transport helper:**

```go
c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
decoder := json.NewDecoder(c.Request.Body)
decoder.DisallowUnknownFields()
if err := decoder.Decode(&req); err != nil { return err }
var extra json.RawMessage
if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
    return ErrInvalidJSON // includes a second JSON value or malformed trailing input
}
return binding.Validator.ValidateStruct(&req)
```

Map body-limit errors to 413, malformed input to 400 and unsupported media type
to 415. Do not return raw decoder/SQL internals. Test unknown fields, trailing
values, oversized input and boundary lengths.

### W7 — Image admission happens after buffering; timeout budgets do not align

Evidence: `internal/helpers/image_upload.go` reads the full source before taking
`imageConversionSlots`; `handler/guard.go` uses a 15-second request deadline;
`cmd/server/main.go:119` configures a ten-second write timeout.

The two-slot limit bounds active conversion, not the memory held by waiting
requests. Concurrent authenticated requests can each retain up to 10 MiB while
waiting. Decode/encode checks context around, not during, CPU-heavy codec work.
A write timeout is not a database-query/request-context deadline. A handler may
commit work after the response can no longer be written.

**Before:**

```go
data, err := io.ReadAll(io.LimitReader(source, maxBytes+1))
slots <- struct{}{} // acquire only after allocating the upload buffer
```

**After:**

```go
select {
case converter.slots <- struct{}{}:
    defer func() { <-converter.slots }()
default:
    return nil, ErrConversionBusy // explicit admission control; map at HTTP edge
}
data, err := io.ReadAll(io.LimitReader(source, maxBytes+1))
```

Align endpoint context, ingress and HTTP write budgets; add upload/account quotas.
Inject the converter instead of global mutable admission state. If hard codec
cancellation is required, use a bounded worker/process with enforceable resource
limits; spawning another goroutine and timing out its caller does not stop CPU
work. Add an EXIF-orientation fixture: the current JPEG decode/re-encode path does
not apply orientation metadata before stripping it, so some camera images can
be displayed rotated after conversion.

### W8 — Runtime database privileges undermine audit immutability guarantees

Evidence: API startup applies migrations through the runtime pool;
`migrations/013_audit_events.sql` relies on triggers; `compose.yaml` initializes
the same local PostgreSQL role used by the application.

The local Docker role is a bootstrap role, and the API's migration ownership
requires DDL privileges. Table owners/superusers can alter/disable trigger
protections. Append-only triggers are useful application safeguards, not an
independent tamper-proof audit boundary. This is a deployment/privilege finding,
not evidence that an external attacker currently has database access.

**Before:**

```go
pool := open(runtimeDSN)
err := migrations.Apply(ctx, pool)
```

**After:**

```go
// Separate deployment/migration job, separate credentials.
err := migrations.Apply(ctx, migratorPool)
// API runtime: no table ownership, CREATE/ALTER or trigger-management rights.
server := NewApplication(runtimePool)
```

Grant the runtime role only necessary operations; audit requires INSERT/SELECT,
not UPDATE/DELETE/TRUNCATE/DDL. For stronger assurance export to an independent,
restricted audit sink. Do not treat local `sslmode=disable`/development credentials
as production settings; verify TLS termination and database transport separately.

## Optimization

### O1 — Use one authoritative representation of role membership

`users.roles` and `user_roles` are both stored. Session permission lookup reads the
array; other repository operations read the junction. The seeder inserts missing
junction entries but does not remove stale ones when refreshed roles shrink.
Current authorization reads the array, so divergence is not by itself a proven
current HTTP privilege bypass. It is a consistency trap for future consumers.

**Before:**

```go
// Update users.roles, then separately maintain user_roles.
```

**After:**

```go
roles, err := roleStore.RolesForUser(ctx, userID) // junction is authoritative
permissions, err := roleStore.PermissionsForUser(ctx, userID)
// Derive roles in DTOs; do not persist a second independently writable copy.
```

If the array remains a deliberate cache, enforce consistency transactionally and
test removals, seeding, deactivation and concurrent administration.

### O2 — Tune the existing pool/cache using measurements

`database.Open` configures pool size/lifetimes and a connect timeout correctly.
pgx already has prepared-statement caching by default; manual `Prepare` on every
request is not a missing optimization. Ten connections per instance is a fixed
deployment assumption, not a universal enterprise limit. Deep OFFSET and
`ILIKE '%q%'` administrative/audit queries become expensive at scale.

**Before:**

```go
cfg.MaxConns = 10
```

**After:**

```go
cfg.MaxConns = settings.DatabaseMaxConns
cfg.ConnConfig.RuntimeParams["statement_timeout"] = "5000"
cfg.ConnConfig.RuntimeParams["lock_timeout"] = "2000"
// Export pool.Stat() metrics; tune per endpoint and measured query plans.
```

Size all replicas against PostgreSQL capacity. Consider keyset pagination and
trigram indexes after EXPLAIN/real workload measurements. Tune statement timeout
for streaming/maintenance workloads separately. Treat PgBouncer execution modes
as a deployment compatibility decision, not a reason to disable caching blindly.

### O3 — Strengthen regression and production-validation gates

The existing tests cover many positive/negative paths, ownership and database
constraints. Unit tests and Go's race detector do not prove database transaction
ordering. Integration tests skip when DATABASE_TEST_URL is absent.

**Before:**

```go
// Sequential reset test: create session, reset password, assert session gone.
```

**After:**

```go
// Coordinate two transactions with channels: never sleeps as synchronization.
<-passwordVerified
resetPasswordAndCommit()
close(allowSessionInsert)
requireNoUsableOldCredentialSession()
```

Use isolated PostgreSQL schemas in CI, run targeted `-race`, inject clocks and
bounded worker configuration, fuzz JSON/multipart/image boundaries and test
crash recovery. Add availability tests for audit-store hangs, SMTP stalls,
shutdown admission and role-membership consistency. Pin and run golangci-lint
in CI instead of silently treating its local absence as a lint pass.

## Validation and limits

- Read-only architectural/security review of application code; only documentation
  and an archived disposable reproduction probe were added by this audit.
- A real PostgreSQL reproduction confirmed C1; its temporary schema was removed.
- Final `go vet ./...` and `go test ./...`: passed.
- Final `npm run check` in `web`: passed.
- Final `go test -race ./internal/service ./internal/mailer ./internal/server`: passed.
- `go test ./internal/repository ./internal/handler -count=1` with the local
  DATABASE_TEST_URL enabled: passed, including isolated PostgreSQL tests.
- `golangci-lint` is not installed; that check was not run and is not claimed to pass.
- Earlier build attempts intersected the in-progress refactor and failed on
  temporarily missing helper/types. The final runs above supersede those results.
- No external delivery, production load test, penetration test, dependency-CVE
  scan, ingress inspection or regulatory certification was performed.

## Reference guidance

- [Go context semantics](https://pkg.go.dev/context#WithoutCancel)
- [NIST password guidance](https://pages.nist.gov/800-63-4/sp800-63b.html)
- [OWASP recovery guidance](https://cheatsheetseries.owasp.org/cheatsheets/Forgot_Password_Cheat_Sheet.html)
- [pgx statement caching](https://github.com/jackc/pgx/wiki/Automatic-Prepared-Statement-Caching)

## Recommended implementation order

1. Fix C1 and add the deterministic database concurrency test.
2. Bound audit writes; implement durable, bounded mail delivery and shutdown.
3. Harden privileged authentication and production configuration/DB privileges.
4. Finish the attachment use-case/service boundary and safe observability.
5. Add strict input decoding, image admission, consistency and CI hardening.
