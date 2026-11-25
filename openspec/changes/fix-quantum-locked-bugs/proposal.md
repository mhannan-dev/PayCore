# Proposal

## Why

Locked items **F1, F2, F3 and F4** in `PAYCORE_QUANTUM_SCOPE_LOCK.md` prevent duplicate wallet payments, false reorgs, checkout denial and false late-payment classification. Source inspection at `94f85c8` confirms these defects; green historical tests did not cover them.

## What Changes

- F1: retain abandoned intents, make same-hash submission idempotent, add the explicitly authorized public recovery endpoint, and persist/resume wallet broadcasts before registration retries.
- F2: persist reorg suspicion evidence, require five negative checks over at least 60 seconds unless positive reorg evidence exists, and stop settled receipt polling after the specified depth/age.
- F3: bound limiter storage with eviction and IPv6 /64 buckets, limit sweeping frequency, and explicitly configure trusted proxies.
- F4: permit configurable lower-bound clock skew while keeping invoice expiry strict.
- Add regression tests, a new migration (030; 026 is already occupied), mirrored API types and related operational documentation.

## Capabilities

### New Capabilities

- `quantum-checkout-safety`: formalize the existing checkout's recovery, confirmation, admission and timing guarantees. There are no existing baseline OpenSpec specs to modify.

### Modified Capabilities

None.

## Impact

Quantum Go services/repositories/routes/models, checkout TypeScript, server proxy configuration and related tests/docs. No new dependencies, roles, ledger changes, contract changes, C1/C2 work or frozen-feature changes. Existing invoices and direct/contract payments remain supported. New `abandoned` failure code requires the matching frontend deployment; migrate first. Recovery's ambiguous phrase "recently expired" is defined as 24 hours after expiry, consistent with the existing operational review horizon. This decision and the precise recovery attribution rules are recorded in design.
