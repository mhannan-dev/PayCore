# PayCore Quantum — Technical Extension Specification

**Version:** 1.0  
**Date:** 2026-11-22  
**Repository:** https://github.com/mhannan-dev/PayCore  
**Target branch:** `PayCore-Quantum`  
**Purpose:** Aevum Builder Challenge #1 hackathon extension  
**Status:** Proposed implementation specification; not a claim that features are already implemented.

## 1. Executive summary

Extend the existing PayCore monorepo with an isolated, testnet-only tUSD8 invoice and payment checkout system for the QDay Aevum network. Preserve the Go/Gin backend, pgx/v5 database access, PostgreSQL migrations, Next.js frontend, existing identity and RBAC patterns, strict typing, and standard API error envelope. Build an end-to-end demonstrable flow: merchant creates invoice → payer connects wallet → payer sends tUSD8 → backend independently verifies on-chain transfer → invoice becomes paid exactly once → signed webhook delivered to merchant.

**Hackathon eligibility gate:** The existing code predates the build window. Confirm with organizers whether a new branch built atop PayCore is eligible, and disclose pre-existing code. Branching does not reset code provenance. Do not rewrite history to imply otherwise. Confirm current contest deadlines, chain details and submission criteria on official pages before final submission.

## 2. Verified baseline and assumptions

Repository README (reviewed 2026-11-22) describes:

- `api/`: Go 1.27 + Gin, handler → service → repository, `pgx/v5`, embedded versioned SQL migrations, `pkg/response` JSON errors.
- `web/`: Next.js 16 App Router, React 19, strict TypeScript, Tailwind CSS 4, yup, react-toastify.
- PostgreSQL 16; `compose.yaml` provides PostgreSQL and Mailpit; `run.ps1` launches local stack.
- Authentication, revocable bearer sessions, five roles/twelve permissions, users/invitations and mail are implemented.
- Double-entry ledger, settlement, reversals and transactional outbox are not complete; some finance screens show sample data.

**Assumptions requiring source inspection:** actual route registration, DI wiring, schema naming, authorization helpers, frontend authentication pattern, migrations numbering, existing CI and web directory presence. The tree below is conceptual; adapt filenames to real conventions after code review.

## 3. Goals, non-goals and scope

### P0: Required MVP
1. QDay Aevum testnet configuration and health check.
2. Merchant-owned invoices denominated in tUSD8 minor units.
3. Public, unguessable checkout link and Next.js wallet connection.
4. Wallet-signed ERC-20 `transfer` on configured testnet.
5. Backend chain/receipt/log validation and durable idempotent confirmation.
6. Invoice/payment status and transaction explorer link.
7. Automated tests, reproducible Docker/local setup, README and 3–5 minute demonstration.

### P1: Valuable enhancements
- Merchant invoice list, payment history and dashboard.
- Signed webhooks with retries and delivery log.
- Invoice expiry, cancellation, reconciliation worker and operational monitoring.

### P2: Optional innovation
- Authorized agent-initiated payment requests with per-agent budgets, expiration, explicit merchant/payer authorization, and audit log. No autonomous key custody or unrestricted spending in the MVP.

### Out of scope
Real money, mainnet, fiat on/off-ramp, custody of private keys, exchange functionality, production settlement, full accounting ledger integration, cross-chain bridging, and promises of quantum resistance. Testnet demonstration only.

## 4. Architecture

```text
Payer browser / Merchant console (Next.js)
  ├── Authenticated merchant invoice management → Gin REST API
  ├── Public invoice checkout → read-only invoice API
  └── User wallet → tUSD8 ERC-20 transfer → QDay Aevum testnet
                                          │
Gin API ── invoice/payment services ──────┤
  │                                       └── QDay RPC via Go Ethereum client
  ├── PostgreSQL (invoices, payment attempts, transfer claims)
  ├── verification/reconciliation worker
  └── webhook delivery worker → merchant endpoint
```

**Trust boundary:** Never trust a browser-reported payment status, sender, amount or transaction hash alone. Go independently checks canonical chain data. The server never receives or stores wallet private keys, seed phrases or signatures unrelated to the transaction.

## 5. Proposed module layout

```text
api/
  internal/
    blockchain/          # network client, ERC-20 event decoding, receipt verifier
    handler/             # quantum invoice and payment handlers
    service/             # invoice lifecycle, payment verification, webhooks
    repository/          # pgx SQL operations and atomic transitions
    model/               # typed requests/responses/domain statuses
    config/              # extend existing config with testnet values
  migrations/            # next available sequential SQL migration
  cmd/                   # extend existing server bootstrap and optional worker
web/
  src/app/quantum/       # authenticated dashboard, invoices
  src/app/pay/[token]/   # public checkout, where route convention permits
  src/components/quantum/
  src/lib/quantum/       # wallet config, ABI, typed API client
  src/hooks/             # transaction lifecycle hooks

docs/quantum/
  ARCHITECTURE.md
  API.md
  SECURITY.md
  DEMO.md
  ELIGIBILITY.md
```

Avoid introducing a parallel authentication system, ORM or duplicate API response framework. Preserve the existing coding rules and migration conventions.

## 6. Blockchain integration contract

All values below are **configuration inputs to re-verify against official QDay documentation**, not permanently trusted constants:

```dotenv
QUANTUM_ENABLED=true
QUANTUM_CHAIN_ID=44005
QUANTUM_RPC_URL=https://rpc-test.qday.info
QUANTUM_EXPLORER_URL=https://explorer-test.qday.info
QUANTUM_TOKEN_ADDRESS=0x456569838f1d43009A91ebA4263147Afd7233483
QUANTUM_TOKEN_DECIMALS=6
QUANTUM_MIN_CONFIRMATIONS=2
QUANTUM_MERCHANT_RECEIVER_ADDRESS=  # required only for a single-receiver demo
QUANTUM_CHECKOUT_BASE_URL=http://localhost:3000
QUANTUM_WEBHOOK_SIGNING_KEY=        # secret; never commit
```

Validate RPC-reported `eth_chainId` at startup; validate token contract code exists and `decimals()` matches configured decimals; reject startup or mark Quantum unavailable if mismatched. Check the wallet network before allowing payment. tUSD8 is testnet-only; gas requires the chain's test gas token. Use verified official faucet instructions; never request a user's seed phrase.

**Go dependencies:** existing `pgx/v5`, `github.com/ethereum/go-ethereum` for RPC, ABI and receipt decoding. **Frontend:** `viem` + `wagmi` (and optional wallet UI connector), chosen to match existing Next.js/React versions. Confirm dependency compatibility and licenses before installation.

## 7. Financial and payment correctness

- Represent token amounts as signed/unsigned integer minor units or decimal strings over JSON; **never floating-point money**.
- For 6-decimal tUSD8: `1.25` tokens = `1250000` base units. Reject excess fractional digits and invalid/zero amounts.
- Generate an immutable invoice snapshot: chain ID, token address, receiver address, amount, expiry, merchant ID and invoice ID.
- Use random, high-entropy checkout tokens; never expose internal merchant or account identifiers unnecessarily.
- The payment is successful only when the transaction is included in the expected chain, the receipt has success status, and a matching token `Transfer` event is emitted by the configured token contract, with expected recipient and sufficient amount. Verify the transaction and event context; do not treat arbitrary logs or a forged hash as payment.
- Bind each payment to exactly one invoice. Maintain a unique claim on `(chain_id, transaction_hash, log_index)` to prevent the same transfer event settling multiple invoices.
- Prefer a unique receiver per invoice or a supported on-chain invoice reference mechanism if implementing a robust multi-merchant gateway. A shared receiver with identical amounts creates ambiguous matching: for MVP require a payment attempt registered before transfer and bind payer address, invoice, receiver and expected amount, then reject ambiguous matches. Do not automatically credit transfers that cannot be uniquely attributed.
- Re-check chain confirmations; reconcile reorgs or receipt disappearance. Define the confirmed threshold as a configurable demo policy, not a guarantee of finality.
- Do not equate a transfer with completed off-chain settlement. Ledger posting is future integration.

## 8. Domain model and schema proposal

Use UUID primary keys consistent with actual repository conventions. Add foreign keys to existing users/merchant tables only after verifying those tables exist; otherwise create a minimal merchant entity linked to existing users. Avoid hardcoded table assumptions.

### `quantum_merchants`
`id`, `owner_user_id`, `display_name`, `receiver_address`, `created_at`, `updated_at`, `disabled_at`.

### `quantum_invoices`
`id`, `merchant_id`, `checkout_token_hash` (unique), `amount_base_units NUMERIC(78,0)`, `chain_id`, `token_address`, `receiver_address`, `status`, `expires_at`, `paid_at`, `created_at`, `updated_at`, `version`.

Statuses: `PENDING`, `SUBMITTED`, `CONFIRMED`, `EXPIRED`, `CANCELLED`; `FAILED` belongs to attempts, not necessarily the invoice. A failed attempt must not make the invoice permanently unpayable. Invoice expiry does not invalidate an on-chain transfer; late payments require a defined manual/reconciliation policy.

### `quantum_payment_attempts`
`id`, `invoice_id`, `payer_address`, `tx_hash` (nullable until broadcast), `status`, `failure_code`, `observed_block`, `confirmations`, `created_at`, `updated_at`.

Statuses: `CREATED`, `BROADCAST`, `VERIFYING`, `CONFIRMED`, `FAILED`, `REORGED`.

### `quantum_transfer_claims`
`id`, `invoice_id`, `attempt_id`, `chain_id`, `tx_hash`, `log_index`, `token_address`, `from_address`, `to_address`, `amount_base_units`, `block_number`, `block_hash`, `created_at`; unique `(chain_id, tx_hash, log_index)`.

### `quantum_webhook_endpoints` / `quantum_webhook_deliveries` (P1)
Endpoints: `id`, `merchant_id`, `url`, `secret_ciphertext`, `enabled`. Deliveries: `id`, `event_id` (unique per endpoint), `endpoint_id`, `event_type`, `payload`, `attempts`, `next_attempt_at`, `status`, `last_response_code`.

Indexes: merchant + creation date, invoice status + expiry, attempt transaction hash, delivery status + retry time. Add database check constraints for nonnegative amounts, valid statuses, nonempty addresses and reasonable expiry.

## 9. API contract (proposed `/api/v1/quantum`)

| Method | Route | Auth | Behavior |
|---|---|---|---|
| `POST` | `/merchants` | existing auth + permission | Create merchant profile |
| `GET` | `/merchants/me` | existing auth | Merchant details |
| `POST` | `/invoices` | merchant permission | Create invoice; require idempotency key |
| `GET` | `/invoices` | merchant permission | Paginated merchant-owned invoices |
| `GET` | `/invoices/:id` | merchant permission | Invoice detail |
| `POST` | `/invoices/:id/cancel` | merchant permission | Cancel unpaid invoice |
| `GET` | `/checkout/:token` | public, rate limited | Minimal non-sensitive invoice snapshot |
| `POST` | `/checkout/:token/attempts` | public, rate limited | Register intended payer address |
| `POST` | `/checkout/:token/attempts/:id/tx` | public, rate limited | Submit tx hash for independent verification |
| `GET` | `/checkout/:token/status` | public, rate limited | Sanitized payment status |
| `POST` | `/webhook-endpoints` | merchant permission | Register webhook (P1) |
| `GET` | `/webhook-deliveries` | merchant permission | Delivery history (P1) |

Follow existing `pkg/response` success/error conventions and API versioning. Use `Idempotency-Key` for invoice creation; scope to merchant and route, persist request fingerprint and replay the original result. Reject same key with different payload. Do not put checkout token in analytics logs. Ensure public endpoints reveal no merchant secrets or payer PII.

### Example invoice creation

```json
{
  "merchant_id": "<merchant-uuid>",
  "amount": "12.500000",
  "description": "Demo invoice",
  "expires_in_minutes": 30
}
```

Example response (illustrative, not existing API):

```json
{
  "id": "<invoice-uuid>",
  "amount_base_units": "12500000",
  "chain_id": 44005,
  "status": "PENDING",
  "checkout_url": "https://<demo-host>/pay/<opaque-token>"
}
```

## 10. Go interface design

```go
type VerifiedTransfer struct {
    ChainID       uint64
    TxHash        common.Hash
    LogIndex      uint
    Token         common.Address
    Sender        common.Address
    Recipient     common.Address
    Amount        *big.Int
    BlockNumber   uint64
    BlockHash     common.Hash
    Confirmations uint64
}

type TransferVerifier interface {
    VerifyERC20Transfer(ctx context.Context, txHash common.Hash) ([]VerifiedTransfer, error)
}

type InvoiceService interface {
    CreateInvoice(ctx context.Context, actor Actor, req CreateInvoiceRequest, key string) (Invoice, error)
    RegisterAttempt(ctx context.Context, token string, payer common.Address) (PaymentAttempt, error)
    SubmitTransaction(ctx context.Context, token string, attemptID uuid.UUID, txHash common.Hash) error
    Reconcile(ctx context.Context, attemptID uuid.UUID) error
}
```

This is an interface sketch, not drop-in code. Adapt `Actor`, UUID package and response types to the repository. Repository transaction must atomically lock invoice, insert unique transfer claim, update attempt and invoice, and enqueue webhook event. On unique-constraint conflict, do not double-credit.

## 11. Next.js UX specification

**Merchant area:** navigation entry `Quantum Payments`, merchant receiver wallet setup, create invoice form, invoice list/detail, copy checkout link, transaction explorer link and webhook status (P1). Reuse existing page shell, permission guards, form conventions and design tokens.

**Public checkout:** merchant display name, amount, network, token, expiry, receiver, connect wallet, network mismatch notice, pay action, wallet confirmation state, transaction pending state, independently verified success state, failed attempt retry, accessible error messages. Never imply success merely because wallet returned a hash.

**Frontend transaction flow:** read invoice snapshot → connect wallet → verify chain → register attempt → call token `transfer(receiver, amount)` → obtain tx hash → submit hash → poll backend status until confirmed/failed/expired. Prevent double-click; handle wallet rejection, network switching and disconnected wallets. The backend remains the source of truth.

## 12. Security and threat model

1. **Authorization:** reuse existing bearer session/RBAC; add explicit Quantum permissions, scoped to merchant ownership. Check authorization server-side on every merchant API.
2. **Wallet safety:** never store private keys, recovery phrases or unrestricted signing credentials. User signs directly in their wallet.
3. **Chain spoofing:** validate RPC chain ID and allowlist token contract; confirm receipt status, log emitter, transfer fields and confirmation policy.
4. **Double spending / replay:** unique event claim, idempotency keys, DB transactions, no trust in client-reported paid status.
5. **Race conditions:** row locking or optimistic concurrency; background reconciliation is idempotent.
6. **Input validation:** bounded amounts, expiry limits, EVM address validation, request size limits, rate limits and pagination.
7. **Webhooks:** HTTPS endpoints; block localhost/private/reserved IPs and redirects to them to mitigate SSRF; HMAC-SHA256 signature with timestamp, event ID and replay window; exponential retry and dead-letter state; rotate secrets.
8. **Privacy:** redact checkout tokens, auth headers, RPC credentials and wallet-sensitive metadata from logs.
9. **Operational controls:** network timeout/retries, circuit breaker/backoff, error classification, metrics and health checks.
10. **Testnet isolation:** prominent TESTNET ONLY label; reject mainnet chain IDs; do not represent tUSD8 as redeemable real money.

## 13. Worker and state transitions

After tx submission, return `202 Accepted` (verification pending). Worker polls for receipt and confirmations; stores observed block hash; verifies transfer; finalizes atomically. If RPC unavailable, retry with bounded exponential backoff. Reconcile `BROADCAST`, `VERIFYING`, and recent `CONFIRMED` attempts to detect reorgs. For late transfers, overpayments, partial payments and duplicate transfers, define explicit `REVIEW_REQUIRED` operational handling rather than silently treating them as normal success. Webhook delivery is at-least-once; merchant consumers must deduplicate by event ID.

## 14. Testing matrix

**Unit:** decimal conversion, overflow/precision, address validation, invoice expiry, permission decisions, idempotency replay, webhook HMAC verification.

**Integration (PostgreSQL):** migrations up/down, merchant isolation, unique transfer claims, concurrent confirmation race, invoice cancellation vs payment, durable webhook enqueue.

**Blockchain (testnet):** correct token transfer, wrong chain, wrong token, wrong recipient, wrong amount, failed tx, pending tx, duplicate tx, mismatched payer, reorg simulation, RPC outage.

**E2E (browser):** merchant login → invoice → public checkout → wallet payment → server verification → merchant status update. Record a deterministic testnet tx hash for judges, with no private keys.

**Regression:** existing auth, RBAC, users, invitations and dashboard unaffected. Run `cd web && npm run check`; `cd api && go vet ./... && go test ./...`; existing linter and PostgreSQL integration suite.

## 15. Deployment and operations

- Local: preserve existing `compose.yaml` PostgreSQL/Mailpit and `run.ps1`; add optional Quantum verifier worker only if necessary.
- Demo: HTTPS web/API endpoints, managed or self-hosted PostgreSQL, persistent worker process, environment secrets, health endpoints, migration startup policy, restricted CORS, backup plan and logs.
- Do not assume free hosting supports always-on workers. A packaged Docker deployment is a fallback only if the hackathon accepts it.
- `.env.example` should document all Quantum variables without real secrets. Use distinct testnet-only wallets.
- Avoid logging full checkout URLs containing opaque tokens.

## 16. Implementation milestones (Nov 22–Dec 11, 2026)

| Dates | Deliverables | Exit criterion |
|---|---|---|
| Nov 22–24 | Eligibility confirmation, branch, baseline tests, configuration and migrations | Existing tests pass; QDay chain/token verified |
| Nov 25–28 | Blockchain client, token ABI, wallet connection, first transfer | Explorer-visible tUSD8 test transfer |
| Nov 29–Dec 2 | Invoice API, checkout page, attempts, verifier | End-to-end payment marks one invoice paid |
| Dec 3–5 | Merchant dashboard, race/replay/negative tests | Duplicate/invalid payments cannot settle invoice |
| Dec 6–7 | Webhooks, retry/audit (P1) | Signed webhook delivered and replay-safe |
| Dec 8–9 | Deployment, docs, regression and demo rehearsal | Reproducible setup and stable public demo |
| Dec 10 | Submission package and video | All required links and evidence prepared |
| Dec 11 | Deadline buffer | Submission verified before official cutoff |

Prioritize P0 over AI-agent scope. If blocked, reduce scope to one merchant, one test token, one chain and a complete verified payment.

## 17. Git strategy and provenance

```bash
git status
git switch main
git pull --ff-only origin main
git switch -c PayCore-Quantum
git push -u origin PayCore-Quantum
```

Commit small, reviewable slices, e.g. `feat(quantum): add chain config and ERC20 verifier`. Do not force-push to hide historical commits. Add `docs/quantum/ELIGIBILITY.md` explaining which baseline components predate the event and which Quantum features were built during it. Ask organizers for written confirmation that extension of a pre-existing MIT-licensed project is allowed; if not, switch to an eligible clean-room project structure rather than misrepresenting provenance.

## 18. Hackathon deliverables checklist

- [ ] Organizer approves existing-base extension or eligible alternative is chosen.
- [ ] Public GitHub branch/repository and clearly labeled original-code provenance.
- [ ] Working tUSD8 on-chain transaction on official Aevum testnet.
- [ ] Testnet contract, network and explorer links verified against official documentation.
- [ ] Readable README with local setup, environment variables and screenshots.
- [ ] Architecture diagram, API docs, security decisions, test results.
- [ ] Deployed demo or accepted runnable Docker package.
- [ ] Short demo video showing real payment, confirmation and merchant update.
- [ ] Submission form completed by official deadline.

## 19. Definition of done

A fresh developer can clone the branch, configure documented testnet values, start the existing stack, sign in as merchant, create an invoice, open a checkout link, pay with testnet tUSD8 from a user-controlled wallet, and observe the server independently confirm exactly one matching transfer and update the invoice. Negative tests reject wrong chain/token/recipient/amount, duplicate event claims and unauthorized merchant access. Existing PayCore functionality still passes regression tests. Documentation accurately distinguishes implemented work, reused baseline and planned features.

## 20. References and verification notes

- Existing project: https://github.com/mhannan-dev/PayCore (README reviewed 2026-11-22).
- Hackathon details supplied by user: https://dorahacks.io/hackathon/qday/detail.
- QDay documentation (verify network, token and faucet values before coding): https://wiki.qday.info/.

**Caveat:** This is a design and implementation contract grounded in the public repository README and the user's hackathon context, not a line-by-line audit of all source files or a guarantee of contest eligibility. Network values, prize rules and exact deadline must be revalidated from official organizers.
