# Quantum P0 API

Backend implementation for `PayCore-Quantum`. All payments are testnet-only,
noncustodial, and separate from the existing accounting ledger. Quantum does not
send webhooks or implement agent mandates.

## Contract checkout integration

Set optional `QUANTUM_PAYMENT_CONTRACT_ADDRESS` to the reviewed, deployed
`QuantumPayments` address. Startup additionally requires contract bytecode and a
canonical `token()` result matching `QUANTUM_TOKEN_ADDRESS`. A missing or mismatched
contract disables Quantum; there is no silent fallback when configured.

Config now exposes `payment_contract_address: string | null`. Invoice and checkout
responses expose `order_id: string` (bytes32 hex) and
`payment_contract_address: string | null`. Migration 029 persists the address on
each invoice. The order ID is derived from the immutable lowercase invoice UUID:
`keccak256(UTF8("paycore-quantum:" + invoiceUUID))`; it is not independently stored.
Changing config affects new invoices only. Existing invoices with a null address
continue using direct token transfers.

Contract checkout reads allowance, requests approval for exactly the invoice
amount if needed, waits for a successful approval receipt, checks allowance again,
then registers an attempt immediately before calling
`pay(order_id, receiver_address, amount_base_units)`. The approval hash is never
submitted as payment proof. Wallet calls use legacy transactions. Approval does
not extend invoice expiry. Status still comes only from the API.

The verifier requires a canonical `InvoicePaid` log from the invoice's stored
contract and checks order ID, payer, merchant, amount, direct transaction signer,
transaction recipient, expiry and confirmations. Wrong/missing contract events or
wrong invoice IDs enter review with `wrong_contract` or `wrong_order`. Malformed
ABI logs are ignored. Token Transfer logs alone cannot settle a contract invoice.
The existing atomic claim store reserves the `InvoicePaid` transaction/log index;
existing reorg, audit and outbox behavior remains in use. Contract invoices are
disambiguated by order ID, so identical payer/amount/receiver invoices may coexist.

Only direct wallet `pay()` is integrated. Gasless relayers and mandate payment UX
remain future work. If payment broadcast succeeds but hash submission fails, the
open checkout page retains the hash and retries submission without sending another
payment. Recovery after closing/reloading the page remains separate lost-hash work.
No live deployment or wallet payment is established by these automated tests.

The original P0 reference below describes direct-transfer invoices unless noted
otherwise; public payloads also include the fields above.

## Configuration and readiness

Copy the Quantum section from `api/.env.example` into local configuration.
`QUANTUM_ENABLED` defaults to false. Network, token, confirmation and checkout
settings are required when enabled; the payment contract is optional and the
webhook key remains unused. Invalid configuration or failed startup RPC checks
disable Quantum without preventing PayCore from starting.

| Variable | Meaning |
| --- | --- |
| `QUANTUM_ENABLED` | Explicit testnet feature opt-in |
| `QUANTUM_CHAIN_ID` | Expected chain ID; positive and safe as a JavaScript integer |
| `QUANTUM_RPC_URL` | Trusted HTTP(S) JSON-RPC endpoint; no embedded credentials, query or fragment |
| `QUANTUM_TOKEN_ADDRESS` | Allowed ERC-20 contract address |
| `QUANTUM_TOKEN_DECIMALS` | Expected contract decimals, 0–18 |
| `QUANTUM_MIN_CONFIRMATIONS` | Demo confirmation policy, 1–1000 |
| `QUANTUM_EXPLORER_URL` | Explorer base URL included in snapshots |
| `QUANTUM_CHECKOUT_BASE_URL` | Frontend base URL used for immutable checkout links |
| `QUANTUM_WEBHOOK_SIGNING_KEY` | Reserved, unused by P0; never commit a real key |
| `QUANTUM_PAYMENT_CONTRACT_ADDRESS` | Optional reviewed deployment; blank selects direct transfer for new invoices |

Verified against official documentation on 2026-11-22: QDay Aevum testnet chain `44005`, RPC `https://rpc-test.qday.info`, explorer `https://explorer-test.qday.info`, tUSD8 contract `0x456569838f1d43009a91eba4263147afd7233483`, six decimals. These are example configuration, not constants in application code. Reverify [chain parameters](https://wiki.qday.info/Knowledge/reference/chains), [tokens](https://wiki.qday.info/Knowledge/reference/tokens), and [faucet instructions](https://wiki.qday.info/guide/start/faucet) before a demonstration. Faucet gas claims require a user-controlled wallet; the backend never funds or signs transactions.

Startup reads `eth_chainId`, checks contract bytecode and calls `decimals()`. Known mainnets, including the documented QDay mainnet ID, are rejected. `/config` always answers 200. Disabled/unavailable Quantum returns the contract's zero-valued config with `enabled:false`; all other Quantum endpoints answer `503 quantum_unavailable`.

## Contract

Base path: `/api/v1/quantum`. DTOs are in `api/internal/models/quantum.go`; the shared assignment A3 is the contract for opencode's TypeScript mirror. JSON is snake_case, times are UTC RFC3339, addresses normalize to lower-case EVM addresses, and money is an int64 base-unit digit string. No float conversion occurs. Six-decimal `12.5` is `"12500000"`. Zero, negative, numeric JSON amounts, excessive precision and int64 overflow are rejected.

Invoices: `PENDING | PAID | EXPIRED | CANCELLED`.
Attempts: `CREATED | BROADCAST | VERIFYING | CONFIRMED | FAILED | REORGED | REVIEW_REQUIRED`.
Failure codes: `wrong_chain`, `wrong_token`, `wrong_recipient`, `wrong_payer`, `underpaid`, `overpaid`, `late_payment`, `tx_failed`, `tx_not_found`, `duplicate_transfer`.

`paid_at`, `paid_tx_hash`, attempt `tx_hash`, and `failure_code` are explicit JSON nulls when absent. Invoice list entries have `attempts:[]`; detail includes attempts newest first. List responses use `{items,total,limit,offset}`. Errors use PayCore's existing `{error,message,details}` envelope.

## Examples: every endpoint

Bash examples use environment variables rather than literal bearer tokens or checkout capabilities. On Windows, use `curl.exe` instead of PowerShell's legacy `curl` alias. Replace example addresses with user-controlled testnet addresses. Obtain `AUTH_TOKEN` using the existing PayCore login; never provide wallet keys to the API.

```bash
API=http://localhost:8080/api/v1/quantum
curl "$API/config"

curl -X POST "$API/merchants" \
  -H "Authorization: Bearer $AUTH_TOKEN" -H 'Content-Type: application/json' \
  -d '{"display_name":"Demo shop","receiver_address":"0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}'
curl "$API/merchants/me" -H "Authorization: Bearer $AUTH_TOKEN"

# Generate one idempotency key per intent. Preserve it when retrying this request.
curl -X POST "$API/invoices" \
  -H "Authorization: Bearer $AUTH_TOKEN" -H "Idempotency-Key: $INVOICE_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"amount_base_units":"12500000","description":"Demo invoice","expires_in_minutes":30}'
curl "$API/invoices?status=PENDING&limit=25&offset=0" -H "Authorization: Bearer $AUTH_TOKEN"
curl "$API/invoices/$INVOICE_ID" -H "Authorization: Bearer $AUTH_TOKEN"
# Use a separate unpaid invoice to demonstrate cancellation.
curl -X POST "$API/invoices/$INVOICE_ID/cancel" -H "Authorization: Bearer $AUTH_TOKEN"

curl "$API/checkout/$CHECKOUT_TOKEN"
curl -X POST "$API/checkout/$CHECKOUT_TOKEN/attempts" \
  -H 'Content-Type: application/json' \
  -d '{"payer_address":"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}'
# Only after registering the attempt: wallet signs and broadcasts ERC-20 transfer.
# Set TX_HASH to the actual wallet-returned hash. This endpoint does not mark PAID.
curl -X POST "$API/checkout/$CHECKOUT_TOKEN/attempts/$ATTEMPT_ID/tx" \
  -H 'Content-Type: application/json' -d "{\"tx_hash\":\"$TX_HASH\"}"
curl "$API/checkout/$CHECKOUT_TOKEN/status"
```

Merchant creation returns 201; one merchant per owner (`409 merchant_exists`). Invoice creation returns 201, including `checkout_url`; replay uses the same body, while a changed request under the same key returns `409 idempotency_conflict`. The scope is actor plus route via the existing `idempotency_records`/`runIdempotent` mechanism. No `merchant_id` is accepted in request bodies.

Merchant queries require `quantum:read`, writes `quantum:write`; migration 025 grants both only to ADMIN. Ownership is enforced in SQL; another merchant's invoice returns 404. Cancellation accepts a pending, unexpired invoice only (`409 invoice_not_pending`).

Public endpoints have `Cache-Control:no-store`, 120 reads/IP/minute and 20 writes/IP/minute with bounded process-local windows. These budgets permit 3-second status polling. A full limiter returns 429 with `Retry-After`. A production reverse proxy should add distributed rate limiting without logging checkout capabilities.

Attempt registration returns 201. At most five open attempts exist per invoice, and one per invoice/payer. Repeating an unused CREATED intent returns that same attempt. A transaction already in flight prevents a new intent for that payer. Stale CREATED intents can be removed after 15 minutes when registering again. Shared receiver/amount/payer intents on the same chain/token are serialized globally and rejected as `409 payment_ambiguous`; finish the existing intent first. A full invoice returns `429 too_many_attempts`.

Unused intents expire 15 minutes after registration. Submitting an expired intent returns `409 attempt_expired` without recording a hash; registering again on the same invoice replaces the old intent. Admission and hash submission share the payment-matching lock and use database wall-clock time after acquiring it, so an old intent cannot become BROADCAST after an indistinguishable replacement has been admitted. Register immediately before signing. If a wallet prompt has been open for 15 minutes, cancel it and register again before signing. If a transfer has already been broadcast after that window, contact the merchant for manual review; do not automatically send another transfer. Broadcast intents continue through verification and are not removed by this unused-intent expiry.

Hash submission only validates/binds the hash and returns `202 Attempt` in BROADCAST state. It never performs receipt RPC work or claims success. Attempts cannot have their transaction replaced; a repeated submission returns `409 tx_already_submitted`, after which the client should poll status. The worker independently verifies chain, transaction signature/context, successful canonical receipt, configured token emitter and Transfer topic, recipient, payer, exact int64 amount and minimum confirmations. Public snapshots omit internal invoice/merchant/user IDs and emails; the public attempt ID and wallet address are intentional A3 fields.

## Expiry, review and reorg policy

Expiry is computed in reads for physically pending invoices. It blocks new attempts, transaction submission and cancellation. A registered transaction mined before expiry can still reach PAID after its confirmation threshold. A transfer mined after expiry, before its recorded intent, with a different payer, or with under/overpayment goes to REVIEW_REQUIRED and never credits the invoice. Failed attempts leave the invoice payable. A transaction absent from a healthy RPC for over 30 minutes is FAILED/tx_not_found; an RPC outage instead backs off without declaring failure. Investigate the explorer and contact the merchant before attempting another transfer. P0 has no automatic refund or manual-credit API.

Verification claims are leased for three minutes, batches of ten, per-job RPC deadline ten seconds. RPC failures back off from three to sixty seconds. Confirmation atomically locks the invoice, inserts the unique `(chain_id,tx_hash,log_index)` claim, updates attempt and invoice, and enqueues `quantum.invoice.paid`. Exactly one canonical claim per invoice is permitted. P0 explicitly excludes QUANTUM_INVOICE events from the existing broker relay and replay, so no Quantum webhook is sent.

Recently confirmed attempts are rechecked for 24 hours from confirmation. Loss/change of canonical receipt or required confirmations changes the attempt to REORGED, invalidates its canonical claim, and reopens the invoice only when no other canonical claim exists. Claims remain permanently reserved against reuse by other invoices. A later canonical reinclusion may reconfirm the same invoice/attempt. Both changes are durably recorded; reorg writes an audit record and a reserved `quantum.invoice.reorged` event. This is a demo confirmation policy, not a finality guarantee or full archival indexer.

## Frontend coordination

Do not display success on a returned hash; wait for `invoice_status:PAID`. Use the immutable invoice fields rather than the current config to render old invoices. The A3 config intentionally does not expose RPC credentials; a wallet must already know the configured test network or be configured from verified official instructions. Merchant receiver configuration is immutable in P0.

The shared audit enum includes `quantum_payment_reorg`, mirrored in `web/src/lib/auth-api.ts` and the audit log UI. The merchant console polls invoice lists and details every ten seconds. Future P1 consumers must deduplicate by outbox event ID and handle reorg events before relying on a prior confirmation. No agent mandate API or webhook endpoint is implemented in P0.

## Validation and scope

`go vet ./...`, `go test ./...`, `golangci-lint run`, and the existing frontend check are regression gates. PostgreSQL tests use isolated schemas via `DATABASE_TEST_URL` from `docs/testing.md`. Local JSON-RPC fixtures and injected verifier fakes cover startup and verification; no automated test calls the live testnet. Repository tests cover ownership, constraints, idempotency, insufficient confirmations, duplicate claims, reorgs and concurrent finalization. A human-controlled wallet transaction and organizer eligibility confirmation remain separate demo/submission tasks; no live transaction hash is invented here.

PostgreSQL validation on 2026-11-22: all seven Quantum repository tests passed against the local PayCore database, including both expired-intent replacement cases. Run from `api/` with `DATABASE_TEST_URL` configured:

```powershell
go test -v ./tests/repositories -run '^TestQuantum' -count=1 -timeout 3m
```

This exercised fresh migration 025, merchant isolation, request replay/conflicts, database constraints, insufficient confirmations, concurrent duplicate claims, cancellation versus confirmation, attempt admission, unused-intent expiry and replacement, and reorg invalidation/reconfirmation with audit and outbox persistence. Test schemas were isolated and cleaned up. These are local database results; they do not establish a live testnet payment or wallet end-to-end result.

The full regression runner also passed on that date:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\test.ps1 -Integration
```

This ran `go vet ./...`, all Go tests with PostgreSQL integration enabled and serialized packages, `golangci-lint run` (zero issues), `npm run check`, and all 31 frontend tests. No application code changes were needed to pass this validation.
