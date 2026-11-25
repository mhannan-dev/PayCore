# Design

## Context

See proposal for locked F1-F4 scope. Current source deletes stale CREATED rows, rejects all repeated submits, reorgs on one miss, scans a full limiter map per request and rejects slightly earlier block timestamps. No baseline specs exist. Migration 029 already exists; the old Part F reference to 026 is stale.

## Goals / Non-Goals

Implement the specified safety fixes and regression coverage. No C1/C2, frozen product expansion, signing, broadcast, new roles, new dependency, or contract changes. The user's instruction authorizes both API and checkout changes for F1 despite earlier split ownership.

## Decisions

- Recover pending or expired invoices up to 24 hours after expiry, not cancelled/paid invoices; existing same-hash requests remain idempotent regardless of invoice state. Retain/reuse a matching CREATED or abandoned intent and its time. Without an intent, use the invoice creation time as the earliest attribution bound: a recovery necessarily declares an already-sent transaction, not a new payment intent. All normal payer/token/amount/expiry/claim checks still apply. Document this explicit recovery trade-off.
- Serialize hash binding globally using a transaction advisory lock, and serialize direct-transfer attribution using the existing payer/amount lock. Check both attempts and permanent claims for other-invoice reuse. Preserve the five-open-attempt limit and cross-invoice ambiguity guard. Recovery never creates a wallet transaction.
- Persist reorg suspicion count/start time and a confirmed-at timestamp; healthy checks clear suspicion. Lagging heads preserve observation and never independently cause reorg. Negative checks retry after 15 seconds. Mark mature confirmed work finalized rather than relying on changing updated_at timestamps.
- Use an access-ordered bounded limiter with oldest eviction, /64-normalized IPv6 keys and at most one expiry sweep per second. Validate trusted proxy CIDRs during config loading; default none. Router accepts explicit options so tests and the composition root share configuration without hidden environment reads.
- Parse skew in the Quantum configuration, pass it into the verification worker, and default pure evaluator callers to 60 seconds. Apply the same lower bound to direct and contract verification; expiry is unchanged.

## Edge cases

Spec scenarios define exact outcomes. Include same/different-hash replay after PAID/EXPIRED, unknown attempt, retained abandoned row, recover with a claimed hash, parallel recovery/submit, open-slot exhaustion and ambiguous direct payment. For F2 cover missing/error chains, count threshold before/after 60 seconds, healthy reset, positive block mismatch, removed logs, lower head and polling finalization. For F3 cover capacity, exhausted existing client, IPv4/IPv6 normalization and trusted/untrusted forwarding. For F4 test exact lower bound and strict expiry on both payment paths. Wrong-chain/token/payer/amount checks remain intact; no money arithmetic changes.

## Database schema

New additive migration 030 on `quantum_payment_attempts` only:

| Field | SQL type / default | Constraints / purpose |
| --- | --- | --- |
| failure_code | existing TEXT, nullable | Extend existing CHECK with abandoned; retain all current codes |
| reorg_suspicions | INTEGER NOT NULL DEFAULT 0 | CHECK between 0 and 5 |
| reorg_suspected_at | TIMESTAMPTZ NULL | First negative observation; null when reset |
| confirmed_at | TIMESTAMPTZ NULL | Backfill existing CONFIRMED rows from updated_at |
| verification_finalized | BOOLEAN NOT NULL DEFAULT false | Exclude mature confirmed work from queue |

Keep existing PK/FK/indexes/open-payer uniqueness and permanent claim constraints. Serialize updates under invoice/attempt locks and lease fencing. Hash advisory locking prevents check-then-bind races without breaking historical attempts that may share a hash. Existing transfer claims remain reserved after reorgs.

## API contract

`POST /api/v1/quantum/checkout/:token/recover`, public checkout capability, no bearer permission, existing write limiter/no-store behavior. Body `{"payer_address":"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","tx_hash":"0x1111111111111111111111111111111111111111111111111111111111111111"}`. Both strings required; trim/lowercase, validate nonzero EVM address and 32-byte hex hash. Return `202 Attempt`, same DTO/nullability as submit, initially BROADCAST with zero confirmations and null failure. Existing same-invoice hash returns its current Attempt. No pagination or extra idempotency header; hash binding supplies retry identity.

Invalid input returns 400 invalid_payer/invalid_tx_hash; unknown capability/attempt returns existing 404 checkout_not_found; inadmissible invoice returns 409 invoice_not_pending; conflicting hash returns 409 tx_already_submitted; ambiguous payment returns 409 payment_ambiguous; five open attempts returns 429 too_many_attempts. Error bodies remain `{error,message,details}`. Example invalid body `{"payer_address":"","tx_hash":"oops"}` is 400 invalid_payer. Same-hash submit is 202 even after expiry/settlement, different-hash submit remains 409. Add abandoned to Go/TypeScript FailureCode and labels. DTOs: api/internal/models/quantum.go and web/src/lib/quantum-api.ts.

## Validation

| Rule | Normalize / bounds | Enforcement | Outcome / verification |
| --- | --- | --- | --- |
| Capability / attempt ID | strict token / UUID | service | existing 404; service/controller tests |
| Payer / hash | trim + lowercase / EVM regex | service + DB | 400 stable code; malformed tests |
| Recovery admissibility | pending or <=24h expired | repository clock + transaction | 409, no writes; integration tests |
| Hash identity / ownership | same invoice/payer only | serialized repository + permanent claims | 202 replay or 409 conflict; race tests |
| Open budget / ambiguity | existing maximum five | repository | 429/409; integration tests |
| Recovery persistence | unknown JSON narrowed; scoped session key | client | prevent second send; fake storage/API tests |
| Reorg | five checks AND >=60s, or positive conflict | service + persisted state / lease | reorg audit/outbox only on decision; tests |
| Proxy | comma-separated CIDRs, empty none | config/router | invalid config fails closed; HTTP tests |
| Clock skew | duration 0..5m, default60s | config/service | invalid disables Quantum; boundary tests |

## Migration and compatibility

Apply additive migration before API and matching web deployment. No destructive backfill; old rows get zero suspicion/unfinalized. Rolling back application code preserves new fields, but rolling back to the old frontend loses recovery safety; prefer forward correction and stop checkout during an incompatible rollback. Do not drop retained attempts or rewrite old migrations. Update A3/API/security/env docs with the new code, endpoint and trust/timing policy.

## Risks / Trade-offs

Skew can attribute an exact same-payer transfer shortly before intent creation; recovery without an intent accepts from invoice creation. Permanent claims, exact attribution, expiry and ambiguity checks limit this exposure. Session storage can be unavailable or cleared; retain in memory, disclose limitation and offer manual hash recovery. Bounded limiter eviction is a memory/fairness trade-off and not distributed DDoS protection. RPC majority evidence is not chain finality.

## Verification

Write regression tests and observe failures before implementation per F item. Run focused service/config/middleware, PostgreSQL repository/controller and frontend helper tests; then all required Go vet/tests/lint and web checks/tests. Validate this OpenSpec change strictly. No real wallet, live RPC, deploy or frozen-feature tests are added.
