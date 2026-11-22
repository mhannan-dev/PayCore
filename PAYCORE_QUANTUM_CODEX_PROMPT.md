# PayCore Quantum P0: task assignment for Codex and opencode

Two agents work in parallel on the existing branch `PayCore-Quantum`:

| Agent | Owns | Section |
| --- | --- | --- |
| **Codex** | backend (`api/`), `.env.example`, `docs/quantum/API.md`, `docs/quantum/SECURITY.md` | Part B |
| **opencode** | frontend (`web/`), `docs/quantum/ARCHITECTURE.md`, `DEMO.md`, `AGENT_MANDATE.md`, the README Quantum section | Part C |

Each agent reads **Part A** (shared rules and the API contract) and then **only its own part**.

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
