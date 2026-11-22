# Ledger core

The double-entry ledger described in [ProjectFeature.md](../ProjectFeature.md) §2 and §4: a chart of accounts, append-only journals, idempotent transfers and reversals, and a zero-sum audit. Migration `014_ledger_core.sql`.

## Model

- **Amounts** are positive integers in the currency's minor unit (JPY scale 0, USD/EUR scale 2), sent in JSON as **strings of digits** (`"50000"`). JSON numbers are rejected.
- **Balances are never stored.** An account's balance is derived from its postings in its normal direction: assets and expenses are debits minus credits; liabilities, equity and revenue are credits minus debits.
- **Overdraft:** accounts have `overdraft_allowed` (default `false`). A posting that would take a non-overdraft account below zero is rejected with `422 insufficient_funds`, after the account row is locked.

## Database guarantees (enforced even if the application is bypassed)

- At commit, every journal has at least two postings, total debits equal total credits, and every posting's account uses the journal currency.
- `journal_entries` and `ledger_postings` reject `UPDATE`, `DELETE` and `TRUNCATE`.
- Postings can only be added to a journal created in the same transaction.
- A reversal must mirror its original exactly (same accounts and amounts, opposite sides); `reversal_for_id` is unique, so a journal can be reversed once.
- An idempotency record cannot commit without its stored responses.

## Transfers

1. Validate: 2–20 legs, positive amounts, at least one debit and one credit, debits equal credits (exact big-integer arithmetic).
2. In one transaction: claim `(user, Idempotency-Key)`; lock every account `ORDER BY id FOR UPDATE`; check existence, active status, currency and funds; insert the journal, postings and an outbox event (`ledger.journal_posted.v1` or `ledger.journal_reversed.v1`); store the response; commit.
3. Deadlocks (`40P01`) and serialization failures (`40001`) retry the whole transaction (3 attempts) with the same key.

**Idempotency-Key** (required on `POST /transfers` and `/reverse`; 8–255 characters, a UUID is ideal), scoped per user:

| Situation | Result |
| --- | --- |
| First request | Executes; `201` |
| Same key, same request (after completion) | Stored response replayed, header `Idempotent-Replayed: true`; nothing is posted again |
| Same key, different request | `422 idempotency_key_reused` |
| Same key while the first request is still running (> 5 s) | `409 idempotency_in_flight`, `Retry-After: 1` |
| A request that failed (e.g. insufficient funds) | Nothing is stored; the key can be retried |

Fingerprint: SHA-256 of the method, route (with target ID) and canonical validated body.

## API (`/api/v1`)

| Method | Path | Permission | Notes |
| --- | --- | --- | --- |
| GET | `/accounts` | `ledger:read` | `?q=&type=&currency=&limit=50&offset=` → page of accounts with `balance` |
| POST | `/accounts` | `accounts:write` | `{account_number, name, type, currency, overdraft_allowed, system_role}`; `409 account_number_taken` / `system_role_taken` |
| GET | `/accounts/:id` | `ledger:read` | account with derived balance |
| GET | `/accounts/:id/postings` | `ledger:read` | `?limit=50&offset=`, newest first, with journal reference |
| GET | `/transfers` | `transfers:read` | `?q=&account_id=&currency=&limit=20&offset=` → journals with postings, `total`, `reversal_for_id`, `reversed_by_id` |
| GET | `/transfers/:id` | `transfers:read` | one journal |
| POST | `/transfers` | `transfers:create` | `Idempotency-Key`; body below; `201` journal |
| POST | `/transfers/:id/reverse` | `transfers:reverse` | `Idempotency-Key`; optional `{description}`; `409 already_reversed`, `409 cannot_reverse_reversal` |
| GET | `/audit/zero-sum` | `audit:verify` | `?currency=JPY` → totals, difference, unbalanced journal IDs, `balanced` |
| GET | `/outbox/metrics` | `system:read` | pending/sending/published/failed counts, `lag_seconds`, `healthy` |
| POST | `/outbox/replay` | `system:write` | `{ "ids": [...] }` (max 200); empty `ids` replays all failed events; audited |

```json
POST /api/v1/transfers
Idempotency-Key: 3f1c2a9e-6f0b-4b8e-9b51-2d0f6c1e7a10

{
  "reference_id": "TXN_20261115_0098",
  "description": "Peer-to-peer wallet transfer",
  "currency": "JPY",
  "legs": [
    { "account_id": "d3b07384-d113-4665-b778-f076efef1111", "direction": "DEBIT",  "amount": "50000" },
    { "account_id": "a9c04291-8842-4f76-bc34-e123afab2222", "direction": "CREDIT", "amount": "50000" }
  ]
}
```

Errors: `400 invalid_transfer` / `unbalanced_transfer` / `idempotency_key_required`, `404 account_not_found` / `transfer_not_found`, `409 already_reversed` / `cannot_reverse_reversal` / `idempotency_in_flight`, `422 insufficient_funds` (with `details.account_number`, `balance`, `change`) / `account_inactive` / `currency_mismatch` / `idempotency_key_reused`, `503 ledger_busy` (`Retry-After`).

`accounts:write` (migration 014) is granted to Admin and Lead operator.

## Zero-sum audit

`GET /audit/zero-sum` runs one read-only SQL statement: `totals` and `unbalanced` CTEs are evaluated against the same PostgreSQL snapshot, so concurrent transfers cannot produce a report where the debit/credit difference is zero but the journal/posting counts disagree. The response includes currency, totals, difference, counts, unbalanced journal IDs (max 100), `balanced` and `checked_at`.

The `/audit` page polls every 10 seconds and refreshes the 60-second freshness clock locally. It shows `Balanced (Δ = 0)`, `Refreshing`, `Stale check`, `Audit unavailable` or `Access denied` explicitly; failed checks and permission denials never show a green result. Overview and Reports render `Audit unavailable` instead of synthesizing a balanced report when the API call fails. The same inconsistent-snapshot protection is covered by `TestLedgerZeroSumConsistentSnapshot`, which reads reports while eight transfers commit.

## Outbox relay

Every transfer or reversal writes `ledger.journal_posted.v1` or `ledger.journal_reversed.v1` into `outbox_events` in the same transaction. Migration `021_outbox_delivery.sql` adds lease, backoff and error fields. A background worker in `cmd/server` claims due `PENDING` rows and expired `SENDING` rows with `FOR UPDATE SKIP LOCKED`, publishes each event, then marks it `PUBLISHED`. Delivery is **at-least-once**: consumers must deduplicate by event ID.

`OUTBOX_PUBLISHER=log` only logs events for local development. `OUTBOX_PUBLISHER=webhook` POSTs the event to `OUTBOX_WEBHOOK_URL` with `X-PayCore-Event-ID`, `X-PayCore-Event-Type` and `X-PayCore-Signature: sha256=<HMAC-SHA256(body)>` using `OUTBOX_WEBHOOK_SECRET`. Any 2xx response is success; `400`, `401`, `403`, `404`, `405`, `409`, `410` and `422` are permanent and move the event to `FAILED`. Other failures retry with bounded exponential backoff (1 second to 5 minutes). After `OUTBOX_MAX_RETRIES` attempts (default 5) the event becomes `FAILED`. `POST /outbox/replay` resets all failed events, or up to 200 supplied IDs, to `PENDING`; every replay is recorded as `outbox_replay` in the audit log.

Metrics expose `pending`, `sending`, `published`, `failed`, `lag_seconds` and `healthy` (`failed == 0 && lag_seconds < 60`). A stale lease cannot complete an event; expired leases are reclaimed. The `/system/outbox` page shows the same metrics and gates replay behind `system:write`.

## Live replay demo

The `/simulator` page posts five parallel `POST /transfers` requests with one `Idempotency-Key` against the seeded JPY accounts: one returns `201`, and the other four return the stored journal with the same ID. "Test changed payload" reuses that key with a different amount and must return `422 idempotency_key_reused`, posting nothing. The page samples `/outbox/metrics` before and after; a clean run increases `published_count` by one, though that aggregate can move when other transfers post at the same time. The page needs `simulator:run`, and live posting needs `transfers:create`.

## Development data

`cd api; go run ./cmd/seed-ledger` creates five JPY accounts (operating bank, two wallets, payout clearing, fee revenue) and posts six sample journals through the services. Re-running is safe (fixed idempotency keys). Journals are append-only: to remove them, rebuild the database.

## Not yet implemented

- A native Kafka/RabbitMQ publisher; the current webhook publisher can bridge to a broker/consumer over authenticated HTTPS.
- Optional Redis tier for idempotency (PostgreSQL is the source of truth).
- Cursor pagination for postings (offset pagination is used for now), account deactivation and editing, partial reversals.

## Statement intake

The Reconciliation page supports CSV upload, validation preview, duplicate detection, private statement history, exact matching suggestions, and exception review. See [statement reconciliation](reconciliation.md) for the CSV format, matching rules, permissions, endpoints, and retention policy. Confirmation links one statement row to one existing posting; reconciliation never changes ledger balances.

### Workspace currency

The header currency selector switches live ledger views between JPY, USD and EUR. The preference is saved in this browser and follows changes in other tabs. Overview, accounts, journal search, reports and CSV exports, transfers, the simulator and invariant audits use the selected currency. New accounts default to it. Switching clears the current page draft and pagination. Payouts also follow the selected currency. Sample notifications retain their original currency.

Switching queries that currency’s ledger; it does not convert amounts or change an existing account’s currency. `GET /transfers` accepts an optional `currency` filter, applied before counting and pagination. Omitting it retains the all-currency API behavior; unsupported codes return `400 invalid_currency`.

### Sandbox payouts

The Settlements page uses the live ledger for the selected currency. Payout accounts are chosen by an explicit **system role** (migration `024_account_system_roles.sql`), never by account number or name:

| `system_role` | Account type | Used for |
| --- | --- | --- |
| `PAYOUT_CLEARING` | LIABILITY | Holds reserved funds until capture or release; required to reserve |
| `OPERATING_BANK` | ASSET | Receives captured payouts; required to capture |
| `null` | any | Ordinary account |

The database enforces at most one account per role and currency, and ties each role to its account type. Set the role when creating the account (the "System role" field in the New account dialog, or `system_role` in `POST /accounts`); the accounts table shows a badge for role holders. A role holder must be active, or reserving and capturing return `409 clearing_account_missing` / `operating_bank_missing`. Source wallets must be active liability accounts with no system role. Arbitrary wallets and assets are never selected as fallback destinations.

Migration 024 gives existing databases the roles the old heuristic picked: per currency, the first active liability numbered 2003 or named "…clearing…", and the first active asset numbered 1001 or named "…bank…". `go run ./cmd/seed-ledger` assigns both roles to its JPY accounts. Changing a role after creation is not implemented yet; until then, use SQL (`UPDATE accounts SET system_role = …`).

| Method | Route | Permission | Behavior |
| --- | --- | --- | --- |
| GET | `/settlements?currency=JPY&limit=25&offset=0` | `transfers:read` | Filtered, paginated payouts |
| GET | `/settlements/:id` | `transfers:read` | Payout with linked journal IDs |
| POST | `/settlements` | `transfers:create` | Reserve wallet funds in clearing |
| POST | `/settlements/:id/capture` | `transfers:reverse` | Complete a reserved sandbox payout |
| POST | `/settlements/:id/release` | `transfers:reverse` | Cancel and refund a reserved payout |

All mutation endpoints require an `Idempotency-Key`. Retrying the same request with the same key replays the stored result; changing the intent under that key returns `422 idempotency_key_reused`. Amounts in API requests are positive minor-unit digit strings; beneficiary name, bank and account are required, with a maximum of 120 characters each. The UI accepts major-unit amounts using the currency's decimal precision and requires `ledger:read` to select a source wallet.

A payout and its journal, postings, outbox event and idempotency response commit in one PostgreSQL transaction. Capture/release lock the payout row before checking its RESERVED state, so competing actions cannot both move funds. Persistence failure rolls back the entire operation. Details show reservation and completion/refund journals; completed payouts have no further capture/release actions. Provider mode is SANDBOX: no external bank transfer occurs. LIVE provider records cannot be completed through this sandbox workflow.
