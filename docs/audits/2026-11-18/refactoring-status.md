# Backend refactoring status

This change addresses the session race and a first set of architecture and
resource-control findings from the backend audit. It does not close every finding.

## Implemented

- Session issuance locks the user row and verifies the password-hash snapshot
  authenticated by the service. A reset that commits first rejects the stale
  login; a reset that commits second revokes the newly created session.
- Profile-photo ownership, storage and image-policy checks live in `PhotoService`.
  HTTP binding stays in the handler; image inspection is injected through an
  interface. Clearing the photo preserves the attachment.
- Invitation and password-reset services share an injected email queue. Defaults
  allow four active deliveries and 64 total admitted jobs. Saturation rejects new
  work, and shutdown closes admission and cancels active SMTP conversations.
  Password-reset responses remain generic when admission fails, so saturation
  does not reveal account existence.
- WebP conversion uses an injected concurrency gate. Busy converters reject work
  before reading the image into memory.
- JSON binding rejects unknown fields and trailing JSON documents while retaining
  binding-tag validation and endpoint body limits.
- Access logs use structured fields, request IDs and route templates, excluding
  raw URLs and query strings. Unexpected handler errors produce a generic 500;
  explicitly unconfigured dependencies retain their route-specific response.
- Audit recording detaches cancellation with a two-second deadline. User-agent
  truncation preserves valid UTF-8.

## Remaining work

- Email jobs remain in memory. A transactional PostgreSQL outbox and independent
  worker are needed for crash-safe delivery and atomic database/email intent.
- General attachment upload/download orchestration still needs a service boundary;
  the profile-photo use case has been extracted first. Image codecs cannot be
  interrupted mid-encode by context cancellation.
- Authentication policy (password requirements, MFA and CAPTCHA design), database
  migration/runtime privilege separation and the duplicate role representations
  remain separate follow-up work.

## Regression coverage

Tests cover real PostgreSQL password-reset/session ordering, mail capacity and
shutdown, photo ownership and image policy, strict JSON validation, converter
admission before buffering, and bounded audit contexts with Unicode metadata.
Repository/handler integration tests use isolated schemas via `DATABASE_TEST_URL`.

## Follow-up: 19 November 2026

The in-memory email queue has been replaced by a PostgreSQL transactional outbox
for invitations, resends and password resets. Durable claims, lease fencing, retries,
stale-token cancellation and terminal-body removal are implemented and tested.
The earlier in-memory-queue limitation above describes the audit snapshot and is
now addressed. See [email delivery](../../email_outbox.md) for operating details.

The general attachment workflows have also moved into an injected
`AttachmentService`. Handlers retain multipart/HTTP handling; the service owns
validation/conversion orchestration, ownership and local-content checks, storage
and metadata coordination. Definite metadata failures remove the new bytes with
a bounded cleanup context; uncertain commits preserve them. See
[attachment architecture and cleanup](../../attachments.md#service-boundary-and-upload-failures).
