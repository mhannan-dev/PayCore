# Quantum P0 security and trust boundary

## Scope and provenance

Quantum is an isolated, testnet-only invoice verifier. It reuses PayCore identity, RBAC, pgx, response envelopes and idempotency infrastructure. It adds no accounting postings, exchange, key custody, production settlement, agent autonomy or quantum-resistance claim. The baseline predates the challenge. Branching does not change provenance; eligibility, deadlines, deployment and actual on-chain demo evidence require human verification. This document is not the human-owned ELIGIBILITY.md.

## Wallet and RPC boundaries

The payer signs in their own wallet. The API never accepts a private key, recovery phrase or unrestricted signing credential. Public checkout capabilities permit payment access only; they do not grant merchant management rights. Signed transactions are read from the configured RPC and the signer is recovered independently of a browser-reported sender.

Chain ID, token contract code and decimals are checked at startup. Invalid config, unsupported mainnets or RPC startup failure leave Quantum unavailable while PayCore still starts. Configuration values are not hardcoded chain facts. [Official network parameters](https://wiki.qday.info/Knowledge/reference/chains) and [token addresses](https://wiki.qday.info/Knowledge/reference/tokens) were checked on 2026-11-22 and must be rechecked before demo deployment. The mainnet denylist is a guard, not a universal classifier of all possible networks; use only a trusted, verified testnet endpoint.

The RPC is an explicit trust dependency. Quantum is not a consensus-verifying light
client and cannot prove honesty of a malicious configured node. It checks chain
identity, receipt and transaction hashes, transaction chain and signer, canonical
header/hash and block number, log transaction/block context, removed-log flags,
event signature, ABI address padding, recipient, payer, bounded amount and
confirmation depth. Native transfers, wrong-contract logs, partial payments,
malformed logs, int64 overflow, duplicate matching logs and insufficient
confirmations cannot settle an invoice. Direct token transfers remain supported
for null-address invoices; configured contract invoices use the direct `pay()`
flow described below. Smart-account batching and relayed payments are outside
this integration.

## Authorization and money correctness

New capabilities are `quantum:read` and `quantum:write`, granted only to ADMIN by migration/seed. Merchant ownership is always derived from the bearer session. SQL predicates constrain all merchant invoice reads and writes, including cancellation; another merchant's ID returns 404. Public DTOs contain no merchant/user IDs or emails. Rate limiting and request body bounds apply before mutation.

Base units are int64/SQL BIGINT and digit strings over JSON. Decimal parsing uses strings only. The blockchain adapter alone uses big.Int and rejects overflow on conversion. Amount, chain, token, receiver, expiry and merchant ownership are immutable invoice snapshots. A failed attempt never permanently fails an otherwise payable invoice.

A registered intent binds invoice, payer, receiver, token, amount and chain before broadcast. Open indistinguishable intents are serialized with an advisory transaction lock, and a unique per-invoice/payer index backs admission. This reduces ambiguous shared-receiver matching but is not a cryptographic invoice reference. Address registration alone does not prove wallet control; a malicious visitor can cause bounded denial of service using a public link. IP limits, attempt caps and stale-intent expiry mitigate but do not eliminate that limitation. Unique receivers or an on-chain invoice-reference contract and signed intent challenge are future hardening. Block timestamps have one-second granularity; the registration ordering check is not a substitute for such a protocol.

Unused intent expiry is enforced on hash submission as well as readmission. Both paths serialize on the same payment-matching lock, and expiry uses database wall-clock time after lock waits. An expired CREATED attempt cannot become BROADCAST after admission has stopped counting it as ambiguous. A wallet transaction signed after the 15-minute window needs merchant investigation; the API cannot undo a transfer or automatically associate it with a replacement intent.

Confirmation is one database transaction: invoice row lock, leased-attempt check, unique transfer claim, attempt state, invoice state and durable outbox enqueue. The unique event key prevents the same event paying two invoices; a partial unique index allows one canonical claim per invoice. Lease fencing prevents a late worker committing over a newer worker. Invoice cancellation racing confirmation yields a consistent paid or cancelled/review result; cancellation cannot reverse an already broadcast chain transfer.

## Reorgs and asynchronous recovery

Contract invoices additionally bind a deterministic order ID and an immutable
payment-contract address snapshot. Startup checks bytecode and the configured
contract's `token()` getter against the token configuration; operators must use a
reviewed deployment, since these checks do not attest arbitrary contract source.
Only canonical, correctly padded `InvoicePaid` logs from that stored address can
settle these invoices. Order, payer, merchant, amount, direct transaction signer,
recipient, expiry and confirmations are checked; the event log index is reserved
in the same atomic claim store. ERC-20 approval and Transfer logs alone do not
settle contract invoices, and neither does `InvoicePaid` alone: the same receipt
must also contain a configured-token `Transfer` of the exact amount to the
merchant, from the payer or from the contract (which forwards
`payWithAuthorization` funds). A misconfigured or faulty contract that emits the
event without moving tokens therefore yields `REVIEW_REQUIRED` (`wrong_token`). Relayer/agent UX is outside this integration. The open
browser page can retry hash submission without paying again; durable lost-hash
recovery remains unimplemented.

Broadcasting returns 202 only. RPC errors retain state and back off; missing transactions are retried before the bounded not-found policy applies. At least the configured confirmations are required before PAID. Confirmed payments are monitored for 24 hours. Canonical receipt loss/change invalidates the claim, changes the attempt to REORGED, reopens only an unclaimed invoice, and appends an audit record in the same transaction. Claim history is never released for use by another invoice. Reinclusion can reconfirm the original invoice. No finite confirmation threshold guarantees finality; deeper/later reorgs require administrator investigation and a production indexer.

P0 writes `quantum.invoice.paid` and `quantum.invoice.reorged` to the existing transactional outbox, but excludes the QUANTUM_INVOICE aggregate from the existing publisher/replay/metrics so no Quantum webhook is sent. P1 must explicitly implement endpoint ownership, HTTPS allowlisting, DNS/IP pinning against private/reserved destinations, redirect rejection, timestamped HMAC-SHA256 with event ID and replay window, retry/dead-letter handling, secret encryption/rotation and receiver deduplication. The existing generic webhook publisher is not silently reused as a merchant gateway.

## Logging and operational limits

Checkout tokens are 32 cryptographically random bytes encoded base64url, indexed in plaintext as explicitly permitted by the assignment. They grant payment access only. Do not log them, put them in analytics, or include them in exception messages. API logs redact checkout route templates and never log raw URLs, query strings, request bodies, auth headers or wallet credentials. Recovery logging suppresses Gin's raw request dump. RPC errors are sanitized at the adapter boundary; startup and worker logs omit endpoint credentials and chain payloads. Reverse proxies and frontend analytics must apply equivalent token redaction.

Public endpoints are no-store and use a bounded per-process IP limiter. Production requires coordinated/distributed limits, trusted proxy configuration and appropriate CSP/referrer controls on the frontend. There are at most five open attempts per invoice, a fifteen-minute unused intent lifetime on readmission, ten jobs per worker batch, three-minute leases and ten-second RPC deadlines. Those bounds are deliberate demo policies; they are not production SLOs.

## Dependencies and test evidence

`github.com/ethereum/go-ethereum` is pinned at v1.17.10. Its reusable library is **LGPL-3.0** (the project also includes GPL-3.0 tools); see the pinned [COPYING.LESSER](https://github.com/ethereum/go-ethereum/blob/v1.17.10/COPYING.LESSER) and [package licensing](https://pkg.go.dev/github.com/ethereum/go-ethereum/ethclient@v1.17.10). This is an added third-party dependency, not relicensed PayCore code. Preserve notices and review distribution obligations before packaging a release. Only `api/internal/blockchain` imports go-ethereum. Its Ethereum types and big integers do not cross into services, repositories, controllers or public DTOs. The dependency resolver also updates compatible transitive crypto/net/protobuf versions; regression checks cover existing authentication and services.

No automated test contacts the live testnet. Local HTTP JSON-RPC fixtures test chain/code/decimals checks and canonical signed-transaction receipts. Fakes test all verifier failure codes and RPC outage handling. Isolated PostgreSQL tests exercise ownership, replay conflicts, database constraints, unique claims, confirmation races, reorg rollback, audit/outbox persistence and no P0 dispatch. Existing auth/ledger/settlement behavior remains a regression gate. A wallet-approved live testnet transfer and a frontend demo are not implied by these backend tests.

Frontend handoff: add the `quantum_payment_reorg` audit action/label alongside the new Quantum permission and DTO mirrors. The frontend, architecture/demo documents and agent mandate design remain opencode's ownership.
