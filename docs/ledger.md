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
- An idempotency record cannot commit without its stored response.

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
| POST | `/accounts` | `accounts:write` | `{account_number, name, type, currency, overdraft_allowed}`; `409 account_number_taken` |
| GET | `/accounts/:id` | `ledger:read` | account with derived balance |
| GET | `/accounts/:id/postings` | `ledger:read` | `?limit=50&offset=`, newest first, with journal reference |
| GET | `/transfers` | `transfers:read` | `?q=&account_id=&limit=20&offset=` → journals with postings, `total`, `reversal_for_id`, `reversed_by_id` |
| GET | `/transfers/:id` | `transfers:read` | one journal |
| POST | `/transfers` | `transfers:create` | `Idempotency-Key`; body below; `201` journal |
| POST | `/transfers/:id/reverse` | `transfers:reverse` | `Idempotency-Key`; optional `{description}`; `409 already_reversed`, `409 cannot_reverse_reversal` |
| GET | `/audit/zero-sum` | `audit:verify` | `?currency=JPY` → totals, difference, unbalanced journal IDs, `balanced` |

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

## Development data

`cd api; go run ./cmd/seed-ledger` creates five JPY accounts (operating bank, two wallets, payout clearing, fee revenue) and posts six sample journals through the service. Re-running is safe (fixed idempotency keys). Journals are append-only: to remove them, rebuild the database.

## Not yet implemented

- The outbox relay (publishing `outbox_events` to a broker) and `GET /outbox/metrics`.
- Optional Redis tier for idempotency (PostgreSQL is the source of truth).
- Cursor pagination for postings (offset pagination is used for now), account deactivation and editing, partial reversals.

## Statement intake

The Reconciliation page supports CSV upload, validation preview, duplicate detection, private statement history, exact matching suggestions, and exception review. See [statement reconciliation](reconciliation.md) for the CSV format, matching rules, permissions, endpoints, and retention policy. Confirmation links one statement row to one existing posting; reconciliation never changes ledger balances.
