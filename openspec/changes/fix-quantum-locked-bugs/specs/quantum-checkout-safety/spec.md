# Spec Delta

## Purpose

Protect the existing testnet checkout from duplicate sends, transient RPC failures, admission exhaustion and clock differences under locked fixes F1-F4.

## ADDED Requirements

### Requirement: Retain and recover broadcasts
The system SHALL retain abandoned intents and permit idempotent hash registration and bounded recovery without claiming payment success before verification.

#### Scenario: Submission retried after response loss
- **WHEN** the same hash is resubmitted for its existing attempt, even after invoice expiry or settlement
- **THEN** the API returns 202 with that attempt; a different hash returns 409 tx_already_submitted.

#### Scenario: Intent abandoned
- **WHEN** an unused intent is at least 15 minutes old during replacement admission
- **THEN** it remains recorded as FAILED/abandoned and no longer consumes an open slot.

#### Scenario: Recover sent transaction
- **WHEN** a valid payer/hash is supplied for a pending invoice or within 24 hours after expiry
- **THEN** recovery returns 202 BROADCAST (or the existing same-invoice attempt), preserves a matching intent's original time when possible, and schedules normal verification.

#### Scenario: Recovery rejected
- **WHEN** input is invalid, the invoice is cancelled/too old, another invoice owns the hash, or open-intent limits/ambiguity would be violated
- **THEN** recovery rejects with the existing corresponding error and does not settle or send funds.

### Requirement: Resume without another wallet transfer
The checkout SHALL retain an unregistered broadcast before any subsequent API call, resume registration after reload, and suppress Pay while that broadcast is unresolved.

#### Scenario: Registration unavailable after broadcast
- **WHEN** hash submission fails through retries after 1, 3 and 9 second backoffs
- **THEN** the stored hash, explorer link and Retry registration remain available; no second wallet payment is offered.

#### Scenario: Reload or missing intent
- **WHEN** valid saved data is loaded or an expired/missing intent prevents submission
- **THEN** the client retries registration or recovery and clears saved data only after the API acknowledges that exact hash.

#### Scenario: Browser storage unavailable
- **WHEN** storage access throws or saved data is malformed
- **THEN** the client handles it safely, preserves a new broadcast in memory, and discloses persistence limits instead of crashing.

### Requirement: Require durable reorg evidence
Confirmed payments SHALL remain confirmed on transient absence or lagging heads. Positive canonical conflict SHALL reorg immediately; otherwise five consecutive negative checks spanning at least 60 seconds SHALL be required.

#### Scenario: Isolated miss or lower head
- **WHEN** one receipt is missing, RPC errors, or confirmation depth decreases without a canonical conflict
- **THEN** the invoice remains PAID with no reorg audit/outbox event; healthy evidence resets suspicions and a lower head alone never reorgs.

#### Scenario: Sustained missing evidence
- **WHEN** five negative checks span at least 60 seconds without a healthy receipt
- **THEN** the attempt becomes REORGED and the existing canonical-claim/audit/outbox policy applies.

#### Scenario: Positive conflict or mature confirmation
- **WHEN** a receipt changes block hash, canonical header conflicts or a log is removed
- **THEN** reorg is immediate; otherwise a healthy receipt at max(minimum,64) depth and confirmation age above ten minutes ends confirmed polling.

### Requirement: Bound and isolate admission
Checkout rate limiting SHALL evict oldest bounded entries, share IPv6 /64 budgets, sweep no more than once per second, and trust forwarded addresses only through configured proxies.

#### Scenario: Capacity reached
- **WHEN** an unseen client arrives at limiter capacity
- **THEN** an old entry is evicted and capacity alone does not return 429; the existing client's exhausted budget still returns 429.

#### Scenario: IPv6 and proxy attribution
- **WHEN** two clients share an IPv6 /64, or a request provides forwarded headers
- **THEN** the IPv6 clients share a bucket and forwarded headers are honored only from configured trusted proxy CIDRs.

### Requirement: Apply bounded lower-bound clock skew
Verification SHALL allow QUANTUM_CLOCK_SKEW (default 60s, accepted 0s through 5m) before intent creation and SHALL keep block-time invoice expiry strict, for direct and contract checkout alike.

#### Scenario: Clock boundary
- **WHEN** an otherwise exact payment is mined 30 seconds before its intent
- **THEN** default configuration accepts it; two minutes before is late, and any payment after invoice expiry remains late.

#### Scenario: Invalid configuration
- **WHEN** skew is negative, malformed or above five minutes, or a trusted proxy entry is not a CIDR
- **THEN** configuration rejects it without enabling broader trust or silently increasing the allowed time window.
