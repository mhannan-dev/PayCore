# PayCore Quantum P0: task assignment for Codex and opencode

Two agents work in parallel on the existing branch `PayCore-Quantum`:

| Agent | Owns | Section |
| --- | --- | --- |
| **Codex** | backend (`api/`), `.env.example`, `docs/quantum/API.md`, `docs/quantum/SECURITY.md` | Part B |
| **opencode** | frontend (`web/`), `docs/quantum/ARCHITECTURE.md`, `DEMO.md`, `AGENT_MANDATE.md`, the README Quantum section | Part C |
| **opencode (round 2)** | smart contracts (`contracts/`), `docs/quantum/CONTRACTS.md` | Part D |
| **Codex + opencode (round 3)** | review fixes: F1 API, F2–F5 = Codex; F1 web = opencode | Part F |

Each agent reads **Part A** (shared rules and the API contract) and then **only its own part**. Before Part D, opencode also reads `PAYCORE_QUANTUM_QDAY_VERIFICATION.md`.

---

## Part A: shared (both agents read this)

### A1. Git and file ownership

- Work on `PayCore-Quantum`. Do not create it, rebase it, or branch from `main`: `main` has only the initial commit. Never force-push or rewrite history.
- **Edit only the files your part owns.** Both agents may be working in the same checkout at the same time. Stage paths explicitly (`git add api/...` or `git add web/...`); never use `git add -A` or `git add .`.
- Before pushing, run `git pull --rebase origin PayCore-Quantum`.
- Do not stage files you did not create: `api/load/`, the `PAYCORE_QUANTUM_*.md` files, or other in-progress docs.
- Never commit `.env` or any private key.

### A2. Binding rules

- `CLAUDE.md` and `docs/coding_rules.md` apply to every change: type safety, the layered architecture, no floats for money, and hand-mirrored API contracts (rule 4).
- `PAYCORE_QUANTUM_EXTENSION_SPEC.md` is the product spec. It was written from the README, not from the source code, so **this file overrides it wherever they disagree**.
- The spec's QDay values (chain ID 44005, the RPC and explorer URLs, the token address, 6 decimals) are **unverified**. They are configuration only and must never be hardcoded in Go or TypeScript.
- **TESTNET ONLY.** No mainnet, no real money, no custody of private keys or seed phrases.

### A3. API contract (single source of truth for both agents)

Codex implements this contract in Go (`api/internal/models/quantum.go`). opencode mirrors it in TypeScript (`web/src/lib/quantum-api.ts`).

Common rules:
- Base path: `/api/v1/quantum`.
- JSON fields are snake_case.
- Amounts are digit strings in base units: tUSD8 has 6 decimals, so `"12500000"` is 12.5 tUSD8.
- Addresses are lower-case `0x` + 40 hex characters.
- Times are RFC 3339 UTC.
- Errors use the existing envelope `{"error", "message", "details"}`.

**If the contract must change:** Codex edits this section in the same commit as the Go change, and opencode updates the TypeScript mirror to match. Nobody else changes the contract.

Enums (Go `const` blocks plus SQL `CHECK` constraints; TypeScript literal unions):

```text
InvoiceStatus  = PENDING | PAID | EXPIRED | CANCELLED
AttemptStatus  = CREATED | BROADCAST | VERIFYING | CONFIRMED | FAILED | REORGED | REVIEW_REQUIRED
FailureCode    = wrong_chain | wrong_token | wrong_recipient | wrong_payer | underpaid | overpaid
               | late_payment | tx_failed | tx_not_found | duplicate_transfer
```

Types:

```ts
QuantumConfig  { enabled: boolean; chain_id: number; token_address: string; token_symbol: string;
                 token_decimals: number; explorer_url: string; min_confirmations: number }
Merchant       { id: string; display_name: string; receiver_address: string; created_at: string }
Attempt        { id: string; payer_address: string; tx_hash: string | null;   // null until broadcast
                 status: AttemptStatus; failure_code: FailureCode | null;     // null unless FAILED/REVIEW_REQUIRED
                 confirmations: number; created_at: string; updated_at: string }
Invoice        { id: string; description: string; amount_base_units: string; chain_id: number;
                 token_address: string; receiver_address: string; status: InvoiceStatus;
                 checkout_url: string; expires_at: string;
                 paid_at: string | null;          // null until PAID
                 paid_tx_hash: string | null;     // null until PAID
                 attempts: Attempt[];             // newest first; [] in list responses
                 created_at: string }
Checkout       { merchant_name: string; description: string; amount_base_units: string;
                 chain_id: number; token_address: string; token_symbol: string; token_decimals: number;
                 receiver_address: string; status: InvoiceStatus; expires_at: string; explorer_url: string }
CheckoutStatus { invoice_status: InvoiceStatus;
                 attempt: Attempt | null;          // the latest attempt; null if none yet
                 paid_tx_hash: string | null }     // null until PAID
Page<T>        { items: T[]; total: number; limit: number; offset: number }   // match the existing list shape in ledger-api.ts
```

Endpoints:

| Method | Path | Auth | Request | Response |
| --- | --- | --- | --- | --- |
| GET | `/config` | public | – | `200 QuantumConfig`. When disabled, returns `{enabled:false, …}` with zero values; it never errors. |
| POST | `/merchants` | `quantum:write` | `{display_name, receiver_address}` | `201 Merchant`; `409 merchant_exists` |
| GET | `/merchants/me` | `quantum:read` | – | `200 Merchant`; `404 merchant_not_found` |
| POST | `/invoices` | `quantum:write` + `Idempotency-Key` header | `{amount_base_units, description, expires_in_minutes}` (5–1440) | `201 Invoice`. A replay returns the same body. The same key with a different body gives `409`. |
| GET | `/invoices?status=&limit=&offset=` | `quantum:read` | – | `200 Page<Invoice>` (own merchant only) |
| GET | `/invoices/:id` | `quantum:read` | – | `200 Invoice`; `404` if missing or another merchant's |
| POST | `/invoices/:id/cancel` | `quantum:write` | – | `200 Invoice`; `409 invoice_not_pending` |
| GET | `/checkout/:token` | public, rate limited | – | `200 Checkout`; `404 checkout_not_found` |
| POST | `/checkout/:token/attempts` | public, rate limited | `{payer_address}` | `201 Attempt`; `409 invoice_not_pending`; `429 too_many_attempts` |
| POST | `/checkout/:token/attempts/:id/tx` | public, rate limited | `{tx_hash}` | `202 Attempt` (status `BROADCAST`); `409 tx_already_submitted`; `409 attempt_expired` after 15 minutes without submission |
| GET | `/checkout/:token/status` | public, rate limited | – | `200 CheckoutStatus` |

`merchant_id` is **never** taken from a request body. It always comes from the logged-in caller.

New permissions: `quantum:read` and `quantum:write`, granted to ADMIN.

---

## Part B: Codex (backend)

### B1. Read first, and copy their shape

- `api/internal/services/settlement.go`
- `api/internal/repositories/settlement_repository.go`
- `api/internal/http/controllers/settlement_controller.go`
- the settlements route file in `api/routes/`
- `api/internal/server/errors.go`
- `api/cmd/server/main.go`
- `api/internal/models/settlement.go`, `ledger.go` and `rbac.go`
- `api/internal/services/event_publisher.go`, for HMAC signing

### B2. Real layout (spec §5 is wrong)

| Spec says | Use instead |
| --- | --- |
| `internal/handler/` | `api/internal/http/controllers/quantum_*_controller.go` |
| `internal/service/` | `api/internal/services/quantum_*.go` |
| `internal/repository/` | `api/internal/repositories/quantum_*_repository.go` |
| `internal/model/` | `api/internal/models/quantum.go` |
| `api/migrations/` | `api/database/migrations/025_quantum.sql`. Forward-only and embedded; re-check that 025 is still the next free number. |
| `pkg/response` | `internal/http/responses`, `fail(c, err)`, plus one row per error in `server.ErrorTable` |
| routes | `api/routes/quantum.go` |
| chain code | `api/internal/blockchain/`, the **only** package that imports go-ethereum; it exposes plain types |
| wiring | `api/cmd/server/main.go` only |
| tests | `api/tests/<area>/`; repository tests use `isolatedPool` |

### B3. Decisions that override the spec

1. **Amounts are `int64` base units.** Use `BIGINT` in SQL and the digit-string JSON pattern of `models.Amount`. Do not use `NUMERIC(78,0)`, and keep `*big.Int` out of everything except `internal/blockchain`. The blockchain package converts on-chain values with an explicit int64 overflow check.
2. **Do not add tUSD8 to `models.Currency`.** P0 makes no ledger postings and changes no existing ledger or settlement behaviour.
3. **Merchant isolation lives in SQL** (`WHERE merchant_id = $n`). Another merchant's invoice returns `404`.
4. **Idempotency reuses `idempotency_records` and `runIdempotent`.** The client ID is the caller's user ID, and the hash uses `requestHash`. Do not build a second mechanism.
5. **Only an exact amount settles an invoice.** Underpaid, overpaid, late payments, or a `from` address that differs from the registered payer all set the attempt to `REVIEW_REQUIRED` with a `failure_code`. They never settle.
6. **Checkout token:** 32 random bytes, base64url, stored with a unique index.
   - It is a public payment link, like Stripe Payment Links: it grants only the ability to pay, so storing it in plain text is acceptable.
   - Never log it. Redact `/checkout/` path segments in `middleware/access_log.go`.
   - Public responses must contain no user IDs, merchant IDs or emails.
7. **Public routes** use `NoStore`, a per-IP limiter modelled on `AuthRateLimit`, at most 5 open attempts per invoice, and at most one open attempt per (invoice, payer).
8. **Verification runs in a worker.**
   - Submitting a hash only validates its format, stores it, and returns `202`.
   - A goroutine started from `main.go` with the server context runs verification. Follow how the email and outbox workers are started.
   - Check, in this order:
     1. chain ID;
     2. receipt status;
     3. log emitter is the configured token;
     4. the `Transfer` topic;
     5. `to` is the invoice receiver;
     6. `from` is the attempt's payer;
     7. the exact amount;
     8. confirmations ≥ `QUANTUM_MIN_CONFIRMATIONS`, otherwise leave the attempt `VERIFYING`.
   - Finalize in **one transaction**:
     1. Lock the invoice `FOR UPDATE` and require it to be `PENDING`.
     2. Insert into `quantum_transfer_claims`, with a unique key on `(chain_id, tx_hash, log_index)`.
     3. Set the attempt to `CONFIRMED` and the invoice to `PAID`.
     4. Write the outbox event `quantum.invoice.paid`. This is the hook for P1 webhooks; send nothing in P0.
   - A unique-key conflict maps to a sentinel error and must never credit twice.
   - Re-check recent `CONFIRMED` attempts for reorgs. On a reorg, set the attempt to `REORGED`, set the invoice back to `PENDING` only if no other claim exists, and write an audit event.
9. **Services depend on a consumer-side `services.TransferVerifier` interface.** Tests use a fake, and **no test touches the live testnet**.
10. **`QUANTUM_ENABLED` false or unset:** merchant and checkout routes answer `quantum_unavailable` through `Ready`/`ErrorFallback`, `/config` returns `enabled:false`, and the rest of PayCore starts normally.
11. **`QUANTUM_ENABLED` true:** check at startup that `eth_chainId` equals `QUANTUM_CHAIN_ID`, that the token address has contract code, and that `decimals()` matches. Reject known mainnet IDs (1, 10, 56, 137, 8453, 42161). If any check fails, log it and mark Quantum unavailable; do not crash.
12. **Configuration:**
    - Read `QUANTUM_*` with typed parsing in `internal/config`.
    - Document every variable in `.env.example` with the comment `# verify against https://wiki.qday.info`, and no secrets.
    - Database `CHECK (addr ~ '^0x[0-9a-f]{40}$')`.
13. **go-ethereum is LGPL-3.0.** Note this in `docs/quantum/SECURITY.md` and in the report.

### B4. Scope

**Do:**
- the config and startup checks;
- migration 025: four tables (merchants, invoices, payment_attempts, transfer_claims) with CHECKs, FKs to `users(id)` and the indexes from spec §8, plus the permission seed;
- `internal/blockchain`;
- repositories, services, controllers and routes for every endpoint in A3;
- the verifier worker;
- `docs/quantum/API.md`, with curl examples for each endpoint;
- `docs/quantum/SECURITY.md`, with the threat model, the trust boundary, the redaction rules and the license note.

**Do not:**
- touch `web/` (opencode owns it);
- build P1 or P2;
- write `ELIGIBILITY.md` (a human writes it);
- add an ORM, a second auth system or a new response format.

### B5. Tests

- **Unit:**
  - decimal ↔ base-unit conversion (excess fraction digits, zero, negative, overflow);
  - address validation;
  - token generation;
  - enum `Valid()`;
  - `Normalize()`.
- **Service, using fakes:**
  - merchant isolation;
  - idempotent replay, and a conflict on the same key with a different body;
  - cancel vs. pay;
  - every verifier rejection and failure code;
  - insufficient confirmations;
  - reorg;
  - the same transaction submitted to two invoices settles at most one.
- **Repository:**
  - 025 applies to a fresh schema;
  - the constraints reject bad rows;
  - two goroutines finalizing the same claim produce exactly one `PAID`.
- **Controller (`httptest`):**
  - `401` without a session;
  - `403` without `quantum:write`;
  - a missing `Idempotency-Key` returns `400`;
  - public responses leak no internal IDs.

### B6. Done

`cd api; go vet ./...; go test ./...; golangci-lint run` passes with zero issues (`DATABASE_TEST_URL` per `docs/testing.md`), and existing tests still pass.

Suggested commits:
1. `feat(quantum): config and chain startup checks`
2. `feat(quantum): schema and repositories`
3. `feat(quantum): invoice service and routes`
4. `feat(quantum): transfer verifier worker`
5. `docs(quantum): API and security`

---

## Part C: opencode (frontend and docs)

### C1. Read first, and copy their shape

- `web/src/lib/ledger-api.ts`, `auth-api.ts` (the typed `request<T>()`), `money.ts` and `rbac.ts`
- `web/src/components/workspace/settlements/`, the closest existing feature: page, table, form and yup schema
- `web/src/components/app-shell/sidebar.tsx`, `page-access.tsx` and `can.tsx`
- `web/src/app/settlements/page.tsx`, which shows that a page only renders a component
- `web/tests/payout.test.mjs`, for the test style

### C2. Rules specific to this part

- Build against the **A3 contract**. The API may not exist yet; do not invent fields beyond A3. Until Codex merges, test the UI by running the API locally once it lands. Do not add permanent fake data or demo fixtures for Quantum.
- **Amounts:**
  - The form accepts decimal input ("12.5").
  - Convert with a string-based parser (`parseTokenAmount(input, decimals) → bigint`) and format with `formatTokenAmount(baseUnits, decimals)`. Put both in `web/src/lib/quantum-amount.ts`.
  - Never use `Number()`, `parseFloat` or float math (coding rule 1).
  - Reject more fractional digits than `decimals`, zero and negative values.
- **Switches:** every `switch` over `InvoiceStatus`, `AttemptStatus` or `FailureCode` is exhaustive, and every failure code has a plain-language message.
- **Wallet:**
  - Add `viem` and `wagmi` at versions compatible with React 19.3 and Next 16.4. Pin exact versions and report them with their licenses.
  - Wallet code lives only in client components, under `web/src/components/quantum/wallet/`.
  - Build the chain configuration from `GET /config` at runtime, never from hardcoded constants.

### C3. Tasks

1. **`web/src/lib/quantum-api.ts`:** TypeScript mirrors of A3 plus one typed function per endpoint, reusing `request<T>()`. Invoice creation sends `Idempotency-Key` with `crypto.randomUUID()`, generated once per form submission and reused on retry.
2. **Permissions:** add `quantum:read` and `quantum:write` to `web/src/lib/rbac.ts`, and a **Quantum Payments** sidebar entry gated by `quantum:read`.
3. **Merchant console** (`web/src/app/quantum/...`, with components in `web/src/components/quantum/`):
   - Merchant setup: display name and receiver address, validated as checksum-insensitive `0x` + 40 hex. Shown when `/merchants/me` returns 404.
   - Invoice list: status filter and pagination (reuse the existing pagination component).
   - Create-invoice dialog: a yup schema that mirrors the API limits, with the type taken from `InferType`.
   - Invoice detail:
     - amount;
     - status badge;
     - a **copy checkout link** button;
     - an attempts table with explorer links (`{explorer_url}/tx/{hash}`);
     - the paid transaction hash;
     - a cancel action behind the existing confirmation modal.
4. **Public checkout page** (`web/src/app/pay/[token]/page.tsx`):
   - No login required; it must work outside the authenticated app shell.
   - Mobile-first, accessible: labels, focus states, `aria-live` for status changes.
   - The flow:
     1. Show the invoice.
     2. Connect the wallet.
     3. Detect the wrong network and offer to switch.
     4. Register the attempt.
     5. Call `transfer(receiver, amount)` on the token.
     6. Submit the hash.
     7. Poll `/status` every 3 s until `PAID`, a terminal failure or `EXPIRED`.
   - States: loading, expired, cancelled, already paid, wallet rejected, wrong network, pending with a confirmations counter, `REVIEW_REQUIRED` (with a plain message to contact the merchant), and success with an explorer link.
   - **Show success only when the backend status is `PAID`,** never just because the wallet returned a hash.
   - Disable the pay button while a request is in flight, to prevent double clicks.
5. **Banner:** a visible **TESTNET ONLY · tUSD8 has no real value** banner on every Quantum page.
6. **Tests:** `web/tests/quantum.test.mjs` covers `parseTokenAmount` and `formatTokenAmount` (edge cases: `"0"`, `"1."`, `".5"`, 7 decimals, leading zeros, very large values), plus status and failure-code label exhaustiveness.
7. **Docs:**
   - `docs/quantum/ARCHITECTURE.md`: Mermaid flowcharts of the system and the payment sequence (browser → wallet → chain → API worker → DB), the trust boundary, and the state machines for invoices and attempts.
   - `docs/quantum/DEMO.md`: a 3–5 minute demo script with one step per beat:
     1. merchant login;
     2. invoice;
     3. checkout on a phone-sized window;
     4. pay;
     5. PAID plus the explorer link;
     6. one rejected edge case (wrong amount → `REVIEW_REQUIRED`).
   - A **README** "PayCore Quantum (testnet)" section: what it is, environment variables (link to `.env.example`), how to get testnet tUSD8 and gas (link to the official QDay faucet only, as a placeholder to verify), run steps, and links to `docs/quantum/*`.
   - `docs/quantum/AGENT_MANDATE.md`: a **design proposal only, no code.**
     - It covers "Agent Spend Mandates": a user or merchant grants an AI agent scoped permission to pay invoices.
     - Mandate rules: per-transaction cap, daily cap, merchant allowlist, expiry, and human approval above the cap.
     - It also covers instant revocation, an audit log, prompt-injection risks, and how PayCore's existing RBAC, audit chain and idempotency support it.
     - Add a section "QDay native primitives to verify" listing smart accounts, session keys and spending limits as **unverified questions**, not facts.

### C4. Do not

- Touch `api/`, `.env.example`, `docs/quantum/API.md` or `SECURITY.md` (Codex owns them).
- Change the A3 contract on your own. If something is missing, write it under "Open questions" in your report.
- Use `any`, `x!` or `{…} as T` (coding rules 6, 8 and 10).

### C5. Done

`cd web; npm run check; npm test` passes, and existing web tests still pass.

Suggested commits:
1. `feat(web): quantum API client and amount helpers`
2. `feat(web): quantum merchant console`
3. `feat(web): public tUSD8 checkout`
4. `docs(quantum): architecture, demo, agent mandate, README`

---

## Final report (each agent)

List:
- your commits;
- every deviation from the spec or from this file, and why;
- dependencies added, with exact versions and licenses;
- environment variables;
- open questions or contract gaps;
- what remains unverified (QDay chain ID, RPC, token address, decimals, native account primitives).

Paste the output of your checks. Do not claim a live testnet payment unless you made one and include the explorer link.

---

## Part D: opencode, round 2 (smart contracts)

**Why:** judging counts "smart contracts on Aevum Testnet" and on-chain activity, and the biggest criterion is innovation (20 points). Plain `transfer()` has no contract of our own and gives the backend no reliable way to tie a payment to an invoice. This part adds one contract, `QuantumPayments`, that covers both:
- **invoice payment**, emitting an `orderId`-bound event;
- **Agent Spend Mandates**, with on-chain caps, expiry and instant revoke. This is the innovation point.

Write it as a **standalone Foundry project in `contracts/`**. It must not import anything from `api/` or `web/`, so it can move to a new repo unchanged if the organizers require one. **Do not touch `api/` or `web/` in this part.** The backend and frontend integration comes later as Part E, after the contract is deployed.

### D1. Read first

- `PAYCORE_QUANTUM_QDAY_VERIFICATION.md`, all of it, especially §2 (Shanghai EVM, legacy transactions, no WebSocket, the token is a USDC-style proxy with EIP-3009).
- The official starter kit https://github.com/qday-io/aevum-starter-kit (MIT), especially `contracts/src/Checkout.sol` and its tests. **Learn its patterns, don't copy it wholesale.** If you copy any snippet, add a comment above it with the source repo, author and MIT license, as the challenge rules require.
- `docs/quantum/AGENT_MANDATE.md` (your own design doc). Update it if the contract differs.

### D2. Toolchain

- Use Foundry. Install it with `foundryup` if `forge` is missing, and report the version.
- Use Solidity `0.8.24` with **`evm_version = "shanghai"`** in `foundry.toml`. Cancun opcodes (`MCOPY`, `TSTORE`) revert on Aevum.
- Use OpenZeppelin Contracts **v5.0.2**, pinned. Use only `SafeERC20`, `IERC20` and `ReentrancyGuard` (the storage-based one, not the transient one).
- Prove with a test that the compiled bytecode contains no Cancun opcodes. A fork test against Aevum with `--evm-version shanghai` is enough.
- Add a `Makefile` with these targets: `build`, `test`, `fmt`, `fork-test` (uses https://rpc-test.qday.info) and `deploy-aevum`.

### D3. Contract: `contracts/src/QuantumPayments.sol`

- Immutable `token` (tUSD8 is `0x456569838f1d43009A91ebA4263147Afd7233483` on Aevum, passed to the constructor).
- **No owner, no admin, no upgradeability, no pause, and no funds held between calls.** Every payment moves tokens payer → merchant within one transaction.

**One event for every payment path**, so the backend watches a single event:

```solidity
event InvoicePaid(bytes32 indexed orderId, address indexed payer, address indexed merchant,
                  uint256 amount, bytes32 mandateId); // mandateId == 0 for direct payments
```

`mapping(bytes32 => bool) public paid;`: each `orderId` can be paid **once**, whichever path pays it. A second payment reverts with `AlreadyPaid`.

The backend derives `orderId` as `keccak256(abi.encodePacked("paycore-quantum:", invoiceUUID))`; document this in `CONTRACTS.md`. The contract treats `orderId` as opaque. It must reject `orderId == 0`, `merchant == address(0)` and `amount == 0`.

**Path 1: direct payment** (approve, then pay):
- `pay(bytes32 orderId, address merchant, uint256 amount)` calls `safeTransferFrom(msg.sender, merchant, amount)`.

**Path 2: gasless payment through EIP-3009** (the payer signs; anyone can relay):
- `payWithAuthorization(bytes32 orderId, address payer, address merchant, uint256 amount, uint256 validAfter, uint256 validBefore, uint8 v, bytes32 r, bytes32 s)` calls `token.receiveWithAuthorization(payer, address(this), amount, validAfter, validBefore, nonce, v, r, s)` and then `safeTransfer(merchant, amount)`.
- **Binding:** the EIP-3009 `nonce` must equal `keccak256(abi.encode(block.chainid, address(this), orderId, merchant, amount))`. The contract computes the nonce itself, so a relayer cannot redirect the payment to another merchant, amount or invoice. Test this explicitly.
- Use `receiveWithAuthorization`, not `transferWithAuthorization`: it requires `msg.sender == to`, which prevents front-running.
- Assert that the contract's balance after forwarding equals its balance before.

**Path 3: Agent Spend Mandate** (owner → agent, with limits):

```solidity
struct Mandate {
    address owner;          // the payer whose tokens are spent (must have approved this contract)
    address agent;          // the only address allowed to spend under this mandate
    uint128 perTxCap;       // max per payment
    uint128 dailyCap;       // max per UTC day (block.timestamp / 1 days)
    uint64  expiresAt;      // unix seconds; must be in the future at creation
    bool    revoked;
    uint128 spentToday;
    uint64  day;
}
```

- `createMandate(address agent, uint128 perTxCap, uint128 dailyCap, uint64 expiresAt, address[] calldata merchants) returns (bytes32 mandateId)`.
  - `msg.sender` becomes the owner.
  - `mandateId = keccak256(abi.encode(block.chainid, address(this), msg.sender, agent, ownerNonce[msg.sender]++))`.
  - The merchant allowlist is stored as `mapping(bytes32 => mapping(address => bool))`, and the list must not be empty. Cap it at 20 merchants.
  - Require `0 < perTxCap <= dailyCap` and `agent != owner`.
- `revokeMandate(bytes32 mandateId)`: owner only, takes effect immediately, and cannot be undone. Emits `MandateRevoked`.
- `agentPay(bytes32 mandateId, bytes32 orderId, address merchant, uint256 amount)`:
  - Only `mandate.agent` may call it.
  - It requires: not revoked, not expired, merchant allowlisted, `amount <= perTxCap`, and the daily spend after this payment `<= dailyCap`. Reset `spentToday` when the day changes.
  - It then pulls `safeTransferFrom(owner, merchant, amount)` and emits `InvoicePaid` with the `mandateId`.
- **Human approval above the cap:**
  - `requestApproval(mandateId, orderId, merchant, amount) returns (bytes32 requestId)` is agent-only. It requires that the mandate is not revoked or expired and the merchant is allowlisted. It is meant for amounts above `perTxCap` or beyond the daily cap.
  - It stores a pending request and emits `ApprovalRequested`.
  - `approveRequest(requestId)` is owner-only and executes the payment, bypassing the caps. State in `CONTRACTS.md` whether approved payments count toward daily spend.
  - `rejectRequest(requestId)` is owner-only.
  - A request cannot be approved after its mandate is revoked or expired.
- Events: `MandateCreated`, `MandateRevoked`, `ApprovalRequested`, `ApprovalResolved`, plus `InvoicePaid`.
- Use **custom errors**, not revert strings: `AlreadyPaid`, `NotAgent`, `NotOwner`, `MandateIsRevoked`, `MandateExpired`, `MerchantNotAllowed`, `PerTxCapExceeded`, `DailyCapExceeded`, `InvalidArgument`, `RequestNotPending`.
- Follow checks-effects-interactions: mark `paid[orderId] = true` and update spend **before** the token call. Apply `nonReentrant` to every external function that moves tokens.
- Add NatSpec to every external function.

**Prompt-injection model:** state it in `CONTRACTS.md`. Whatever an AI agent is tricked into doing, it cannot exceed the on-chain caps, pay a merchant outside the allowlist, or act after revoke or expiry, because the contract enforces all of these, not the agent's prompt.

### D4. Tests (`contracts/test/`)

- **Unit tests** with a local mock token (EIP-3009 + permit; you may adapt the kit's `MockUSD8`, with credit). They must cover:
  - each path pays the merchant exactly once;
  - a second payment of the same `orderId` through any path reverts;
  - zero and invalid arguments revert;
  - an EIP-3009 signature for another merchant, amount or `orderId` reverts;
  - a reused authorization reverts;
  - mandate: a call from anyone other than the agent reverts;
  - per-transaction cap, daily cap, and the daily reset after `vm.warp(+1 days)`;
  - expiry, revoke (including revoke taking effect between two agent payments), and a merchant outside the allowlist;
  - approval request → approve and → reject, and approval after revoke or expiry;
  - the owner's allowance or balance being too low;
  - the contract holds no tokens after any test.
- **Fuzz tests:** for random amounts and timestamps, the agent's total spend per day is never above `dailyCap`.
- **Invariant test:** the contract's token balance is always 0.
- **Fork test** (`test/fork/`, run via `make fork-test`, skipped by default in `forge test`):
  - against the real tUSD8 at https://rpc-test.qday.info;
  - use `deal` or impersonate a holder only on the fork; **never broadcast**;
  - prove the real `receiveWithAuthorization` works with the EIP-712 domain name `tUSD8`, version `2`.

### D5. Deployment script (the human deploys; you do not)

- `contracts/script/Deploy.s.sol` deploys `QuantumPayments(tUSD8)` and logs the address.
- `make deploy-aevum ACCOUNT=<foundry-keystore-name>` uses `--legacy --broadcast --rpc-url https://rpc-test.qday.info` and a **Foundry keystore** (`cast wallet import`).
- **Never** put a private key in `.env`, in command arguments or in files. **Do not run the broadcast yourself.** The user deploys with their own funded wallet.
- Explorer verification is broken, so `CONTRACTS.md` gets a placeholder table for the deployed address, the deploy tx hash and the exact commit. The user fills it in.

### D6. Documentation: `docs/quantum/CONTRACTS.md`

Cover:
- the purpose;
- a Mermaid sequence diagram for each of the three paths;
- the function reference;
- the `orderId` derivation;
- the EIP-3009 nonce binding;
- the mandate rules;
- the threat model (front-running, replay, redirection, reentrancy, prompt injection, compromised agent key → revoke, compromised owner key = out of scope);
- known limits (no upgrade, no refunds, `paid` is permanent, the token itself is an upgradeable proxy controlled by QDay);
- how to run the tests and the fork test;
- the deploy steps and the address table.

Also add one paragraph and a link to this file in `ARCHITECTURE.md` and `AGENT_MANDATE.md`.

### D7. Not in this part

- Backend changes, such as verifying `InvoicePaid` in the Go worker or adding `contract_address` to `/config`. Those come in **Part E** after deployment. List what Part E needs in your report: the A3 fields to add and the event ABI.
- Frontend changes to call the contract.
- A relayer service for gasless payments.
- Any broadcast to the testnet.

### D8. Done

- `cd contracts; forge fmt --check; forge build; forge test -vv` passes with **no warnings**.
- `make fork-test` passes (or report the exact RPC error if the network is down).
- The Shanghai check is proven.
- Commit only `contracts/**` and `docs/quantum/CONTRACTS.md` (plus the one-paragraph links in your own docs). Suggested commits:
  1. `feat(contracts): QuantumPayments with direct and EIP-3009 invoice payments`
  2. `feat(contracts): agent spend mandates with caps, expiry, revoke and approvals`
  3. `test(contracts): unit, fuzz, invariant and Aevum fork tests`
  4. `docs(quantum): contract reference and threat model`

For forge dependencies, either git-ignore `contracts/lib/` with a documented install step, or use git submodules as the starter kit does. Say which you chose.

**Report:**
- the Foundry and solc versions;
- the gas cost of each function from `forge test --gas-report`;
- the test count;
- any deviation from D3 and why;
- the exact `make deploy-aevum` command the user must run;
- the Part E checklist.

---

## Part F: review fixes for P0 (commit `104484e`)

A review of P0 found the issues below. All checks currently pass: `go vet`, `go test` including the PostgreSQL repository tests, golangci-lint with 0 issues, `npm run check`, and 31/31 web tests. Fix each issue with a **regression test that fails before the fix**. Keep the existing ownership: **F1 web = opencode; F1 API, F2, F3, F4 = Codex.** F1 needs both: Codex lands the API part first, then opencode.

### F1. 🔴 Lost tx hash can cause a double payment (Codex + opencode)

**Bug:** in `web/src/components/quantum/wallet/checkout-flow.tsx` (`pay()`, around lines 121–131), the wallet broadcasts the transfer, and then `submitCheckoutTx` fails (network error, `429`, tab closed). The hash is lost. When the payer clicks Pay again, `RegisterAttempt` returns the same `CREATED` attempt (`quantum_repository.go` around lines 241–246), and the payer sends a **second** transfer. The first transfer has no record, and its `CREATED` attempt is deleted after 15 minutes.

**Codex (API):**
- Never delete a `CREATED` attempt that might have a broadcast pending without a trace. Change the 15-minute cleanup so it no longer silently drops `CREATED` attempts: expire them to `FAILED` with a new failure code `abandoned`, which keeps the row. Add `abandoned` to the Go const, `Valid()`, the SQL `CHECK` (new migration `026_quantum_review_fixes.sql`; **never edit 025**), and the A3 `FailureCode` list in this file.
- Make `POST /checkout/:token/attempts/:id/tx` safely repeatable:
  - the same hash for the same attempt → `202` with the current attempt (idempotent);
  - a different hash for an attempt that already has one → `409 tx_already_submitted` (unchanged).
- Add a recovery endpoint: `POST /checkout/:token/recover` with body `{payer_address, tx_hash}`, public and rate limited.
  - If the invoice is `PENDING` or recently `EXPIRED`, and no attempt or claim already has that hash, create an attempt in `BROADCAST` with that hash and the payer, and return `202 Attempt`.
  - The verifier then applies all the normal checks, including `from == payer` and the exact amount.
  - A hash already used by another invoice gives `409 tx_already_submitted`.
  - Add it to A3 and `docs/quantum/API.md`.
- Tests:
  - repeating a submit with the same hash is idempotent;
  - recovery creates a verifiable attempt;
  - recovery with a hash already claimed elsewhere is rejected;
  - an abandoned attempt keeps its row.

**opencode (web):**
- **Immediately** after `writeContractAsync` returns, before any network call, save `{attempt_id, tx_hash, payer}` to `sessionStorage` under a key derived from the checkout token. Wrap storage access in try/catch, and parse it back as `unknown` with a type guard (coding rule 7).
- Submit with retry: 3 attempts with backoff (1 s, 3 s, 9 s). If all fail, show "Payment sent but not yet registered" with a **Retry registration** button and the explorer link. **Never show the Pay button again while a saved, unregistered hash exists.**
- On page load, if a saved hash exists, resume: call submit again (it is idempotent), or `recover` if the attempt is gone, then poll.
- Clear the saved entry only once the backend reports that the attempt has that `tx_hash`.
- Add a manual "I already paid. Paste transaction hash" link that calls `recover`.
- Tests in `web/tests/quantum.test.mjs` cover the saved-hash type guard and the resume decision. Extract the decision into a pure function in `web/src/lib/`, so it can be tested without React.

### F2. 🟠 One flaky RPC answer can flip PAID back to PENDING (Codex)

**Bug:** `api/internal/services/quantum.go` around line 168. The re-check of `CONFIRMED` attempts treats any `!Found`, `!Canonical` or `Confirmations < minimum` as a reorg. A single empty receipt or a lagging RPC node therefore flips the invoice to `PENDING`, writes an audit event, and emits `quantum.invoice.reorged` followed later by `quantum.invoice.paid` again: flapping webhooks. Aevum is a single-sequencer zk validium, so real reorgs are rare.

**Fix:**
- **Positive evidence** is enough to declare a reorg at once: the receipt is found with a different block hash, or the canonical header at that height has a different hash, or a log is marked `Removed`.
- **Absence** (`!Found`, RPC error, lower head) never reorgs at once. It increments a `reorg_suspicions` counter on the attempt (migration 026) and reschedules. Declare a reorg only after **5 consecutive** negative checks spread over at least 60 s, and reset the counter on any positive check.
- A lower head than before (`Confirmations` decreasing) is a lagging node: retry, never reorg.
- Stop re-checking `CONFIRMED` attempts once they have passed `max(min_confirmations, 64)` confirmations and are older than 10 minutes, instead of re-polling for 24 hours.
- Tests:
  - one missing receipt → still `CONFIRMED`, no audit event, no outbox event;
  - 5 consecutive missing receipts → `REORGED`;
  - a different block hash → immediate `REORGED`;
  - a lower head → no change.

### F3. 🟠 Rate limiter can lock out all checkouts (Codex)

**Bug:** `api/internal/http/middleware/quantum_rate_limit.go`. When 4096 distinct keys exist, **every new** client gets `429`, so anyone with many IPs (or one IPv6 /64) can block all checkouts. The map is also scanned in full on every request while holding the lock. And because of `SetTrustedProxies(nil)` in `server/router.go`, all users share the proxy's IP behind a reverse proxy, so the whole demo gets 20 writes per minute in total.

**Fix:**
- When the table is full, evict the oldest entries; never reject new clients just because the table is full. Bound the memory and sweep expired entries at most once per second, not on every request.
- Key IPv6 clients by their /64 prefix.
- Add the config `TRUSTED_PROXIES` (a comma-separated list of CIDRs; empty means none, which keeps today's behaviour) and pass it to `SetTrustedProxies`. Document it in `.env.example` and `docs/quantum/SECURITY.md`, including what to set behind the demo host.
- Also cap open attempts per invoice in the database. This already exists; keep it.
- Tests:
  - a full table still admits a new client;
  - IPv6 addresses in the same /64 share one bucket;
  - with a trusted proxy configured, `X-Forwarded-For` is honoured; without one, it is ignored.

### F4. 🟡 Clock skew can mark a valid payment late (Codex)

**Bug:** `api/internal/services/quantum.go` around line 224 rejects a payment as `late_payment` when `BlockTime < attempt.CreatedAt`. `CreatedAt` comes from the database clock and `BlockTime` from the chain, so a server clock a few seconds fast turns a valid, fast (~1 s block) payment into `REVIEW_REQUIRED`.

**Fix:**
- Allow a skew of `QUANTUM_CLOCK_SKEW` (default `60s`, max `5m`) on the lower bound: `BlockTime < CreatedAt - skew` is the pre-attempt case.
- Keep the expiry check strict, using the block time against `expires_at`.
- Document the trade-off in `SECURITY.md`: an exact-amount transfer from the same payer within the skew window before the attempt could be attributed to it.
- Test that a block 30 s before the attempt passes and one 2 min before is late.

### F5. Prepare for Part E (Codex, design only)

Do **not** implement it yet. The verifier currently requires `tx.To == token` and `tx.From == payer` (`quantum.go` around lines 190 and 202), so payments made through the `QuantumPayments` contract, gasless relays or agent mandates would all be rejected. In your report, describe the change needed to verify by the `InvoicePaid(orderId, payer, merchant, amount, mandateId)` event from the configured contract address instead. Keep the current direct-transfer path for backward compatibility.

### F6. Done

- **Codex:** `cd api; go vet ./...; go test ./...; golangci-lint run` passes (with `DATABASE_TEST_URL`). Commit as `fix(quantum): …`, one commit per F item.
- **opencode:** `cd web; npm run check; npm test` passes. Commit as `fix(web): recover lost checkout tx hash`.
- Each agent reports: the fixed items, the new tests, the A3 contract changes (Codex edits A3 in this file in the same commit), and anything skipped.
- Housekeeping for opencode's contracts: make sure `contracts/out/`, `contracts/cache/` and `contracts/broadcast/` are git-ignored and never committed.
