# Sandbox notifications

The Delivery Center reads persistent notification records for the selected workspace currency. It filters by channel and last-attempt status, shows pending/delivered/failed counts, and opens delivery attempts, sandbox receipts, linked journals and payouts. Linked financial records still require `transfers:read`.

Each committed journal event processed by the outbox relay creates one EMAIL delivery to `sandbox-email-inbox` and one WEBHOOK delivery to `sandbox-webhook-receiver`. These are local sandbox sinks: there is no notification SMTP or HTTP request. Receipts use a stable delivery ID. Existing events already marked PUBLISHED are not backfilled; post a new transfer or payout to see notifications. The independently configured outbox publisher continues to operate as before.

Outbox publication and notification delivery are separate states. A channel can already be delivered while the event awaits broker publication. A failed last attempt may have an automatic retry scheduled. The worker uses the outbox's existing exponential backoff, retry limit and leases. A unique `(event_id, channel)` prevents duplicate delivery records. Accepted channels are skipped on event replay. Attempts and receipts are saved together, and expired/replaced lease tokens cannot change the outcome. Sandbox receipts are idempotent; future real providers must accept the delivery ID as their idempotency key because network delivery is at least once.

| Endpoint | Permission | Purpose |
| --- | --- | --- |
| `GET /api/v1/notifications?currency=JPY&status=FAILED&channel=WEBHOOK&limit=25&offset=0` | `notifications:read` | Filtered, paginated history |
| `GET /api/v1/notifications/metrics?currency=JPY` | `notifications:read` | Last-outcome counts |
| `GET /api/v1/notifications/:id` | `notifications:read` | Delivery and attempt history |
| `POST /api/v1/notifications/:id/retry` | `notifications:retry` | Queue a failed event for retry |

A manual retry sends `{ "attempts": 5 }` using the delivery's current count. It is accepted only when that delivery and its outbox event are both FAILED. Concurrent, repeated or stale retries return `409 delivery_stale`. The retry queues failed channels on that event, preserves successful channels, and records a `notification_retry` audit event. Retrying notification delivery never posts ledger money. All default roles receive read access; ADMIN and STAFF_ENGINEER receive retry access.

Run all API checks with PostgreSQL integration enabled using `powershell -NoProfile -ExecutionPolicy Bypass -File .\test.ps1 -Suite api -Integration`. The opt-in mode uses the local PostgreSQL service in `compose.yaml` unless `DATABASE_TEST_URL` is already set, restores the original environment afterward, and runs packages sequentially to avoid contention on the migration advisory lock. Tests create and clean up isolated schemas; the database service must already be running.
