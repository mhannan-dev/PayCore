# Quantum smart contracts

Part D adds a standalone Foundry project under [`contracts/`](../../contracts/). It is separate from the Go API and TypeScript frontend: no imports cross that boundary, and Part D does not change API behaviour or P0 checkout. The contracts are **testnet-only** and **not deployed as part of this work**. No live transaction or testnet payment was made. For the P0 flow, see [ARCHITECTURE.md](ARCHITECTURE.md); for the product-level mandate proposal, see [AGENT_MANDATE.md](AGENT_MANDATE.md).

## Toolchain and dependencies

| Item | Value |
| --- | --- |
| Foundry | `forge 1.10.0`, commit `083e22994195d6a7e3ad3104d0b57f478c9b9062`, built `2026-11-19` |
| Solidity | `0.8.24` |
| EVM target | `shanghai` — Cancun opcodes are not supported on Aevum |
| OpenZeppelin Contracts | `v5.0.2`, pinned under `contracts/lib/` |
| forge-std | installed under `contracts/lib/` |
| Endpoint alias | `aevum` → `https://rpc-test.qday.info` |

The sandbox runtime uses legacy EIP-155 transactions. Deployment commands must use `--legacy`; no WebSocket endpoint is configured. `contracts/lib/`, `out/`, `cache/` and `broadcast/` are ignored. Dependency licenses are recorded in their vendored packages: OpenZeppelin Contracts MIT, forge-std MIT, solc MIT.

The public QDay starter kit at `https://github.com/qday-io/aevum-starter-kit` (MIT, Copyright (c) 2026 PQA Labs) was reviewed for EIP-3009 and fork-test patterns. The EIP-712 signing helper and the real-token fork-test shape are adapted from it; adapted blocks carry a source/author/license comment. The production contract is original.

## Network facts used

Values below are taken from `PAYCORE_QUANTUM_QDAY_VERIFICATION.md` and must still be treated as **unverified for production**:

- Chain ID: `44005` (wiki + RPC + explorer agreement).
- RPC: `https://rpc-test.qday.info` (wiki + RPC + explorer agreement).
- Explorer: `https://explorer-test.qday.info` (wiki).
- tUSD8: `0x456569838f1d43009A91ebA4263147Afd7233483` (wiki + explorer + starter-kit agreement), 6 decimals.
- Token behaviour: EIP-1967 proxy with EIP-3009 `transferWithAuthorization` and `receiveWithAuthorization`; legacy transaction type required.
- The DoraHacks rules page returned HTTP 405 during verification. Read it manually before relying on any claim about prizes, eligibility or deployment requirements.

Application code in `api/` and `web/` keeps these values in runtime configuration only. The Solidity contract stores the token address immutably in its constructor; the deployment script defaults to the testnet address but accepts `TUSD8_ADDRESS` so it can be redeployed against a different token.

## Invoice identifier

The backend owns invoice identity. For an invoice with UUID `INVOICE_UUID`, the canonical `orderId` is:

```text
orderId = keccak256(abi.encodePacked("paycore-quantum:", invoiceUUID))
```

`invoiceUUID` is the lowercase UUID string from A3, without braces. The contract treats `orderId` as opaque bytes and never parses it. The same derivation is used for direct, EIP-3009 and mandate payments, so one `paid[orderId]` slot protects all paths. The backend must expose this value to a relayer or wallet in Part E; the contract does not derive it from any other field.

## Contract design

### `QuantumPayments`

[`src/QuantumPayments.sol`](../../contracts/src/QuantumPayments.sol) is the only production contract. It accepts tUSD8 from a payer and forwards it in the same call to a merchant. It never holds a token balance between calls.

| Concern | Implementation |
| --- | --- |
| Token reference | `IERC20 public immutable token`, set once in the constructor |
| Token safety | OpenZeppelin `SafeERC20` plus a strict `balanceBefore`/`balanceAfter` equality assertion after forwarding |
| Idempotency | `mapping(bytes32 orderId => bool) public paid`; all three payment paths share it |
| Reentrancy | Storage-based OpenZeppelin `ReentrancyGuard` on `pay`, `payWithAuthorization`, `agentPay` and `approveRequest` |
| Effects before interaction | Mark the order paid, update mandate spend and resolve approval state before the external token transfer |
| Failure style | Custom errors plus Solidity `assert` for the balance invariant; no string reverts in the production contract |
| Documentation | NatSpec on every external function |
| Upgrade/admin surface | None: no `Ownable`, no proxy, no pause, no initializer and no selfdestruct |

Payment inventory:

1. `pay(orderId, merchant, amount)` — payer approves the contract; the contract pulls from `msg.sender` and forwards to `merchant`.
2. `payWithAuthorization(...)` — a relayer submits a payer-signed EIP-3009 `receiveWithAuthorization`. The contract is the EIP-3009 recipient and immediately forwards the tokens to `merchant`; the relayer never custodies funds. The payer's signature binds `value`, `validAfter`, `validBefore`, the token domain, the contract address and the contract nonce.
3. `agentPay(mandateId, orderId, merchant, amount)` — an authorized agent spends against a revocable on-chain mandate.

### Payment sequences

```mermaid
sequenceDiagram
    autonumber
    actor P as Payer
    participant C as QuantumPayments
    participant T as tUSD8
    participant M as Merchant

    P->>T: approve(C, amount)
    P->>C: pay(orderId, M, amount)
    C->>C: reject paid/zero/invalid; mark paid[orderId]
    C->>T: safeTransferFrom(P, M, amount)
    T-->>M: amount
    C-->>C: assert balance unchanged
    C-->>C: emit InvoicePaid(orderId, P, M, amount, 0)
```

```mermaid
sequenceDiagram
    autonumber
    actor P as Payer
    participant R as Relayer
    participant C as QuantumPayments
    participant T as tUSD8
    participant M as Merchant

    P->>P: sign EIP-712 ReceiveWithAuthorization
    R->>C: payWithAuthorization(orderId, P, M, amount, window, v, r, s)
    C->>C: compute bound nonce; mark paid[orderId]
    C->>T: receiveWithAuthorization(P, C, amount, window, nonce, v, r, s)
    T-->>C: amount (msg.sender == to)
    C->>T: safeTransfer(M, amount)
    T-->>M: amount
    C-->>C: assert balance unchanged
    C-->>C: emit InvoicePaid(orderId, P, M, amount, 0)
```

```mermaid
sequenceDiagram
    autonumber
    actor O as Owner
    participant A as Agent
    participant C as QuantumPayments
    participant T as tUSD8
    participant M as Merchant

    O->>C: createMandate(agent, caps, expiry, allowlist)
    C-->>O: MandateCreated(mandateId)

    alt within caps
        A->>C: agentPay(mandateId, orderId, M, amount)
        C->>C: check agent, allowlist, expiry, caps; update spentToday
        C->>T: safeTransferFrom(O, M, amount)
        T-->>M: amount
        C-->>C: emit InvoicePaid(orderId, O, M, amount, mandateId)
    else above cap or daily limit
        A->>C: requestApproval(mandateId, orderId, M, amount)
        C-->>O: ApprovalRequested(requestId)
        O->>C: approveRequest(requestId)
        C->>C: bypass caps; mark paid and resolve request
        C->>T: safeTransferFrom(O, M, amount)
        T-->>M: amount
        C-->>C: emit ApprovalResolved(requestId, mandateId, true)
        C-->>C: emit InvoicePaid(orderId, O, M, amount, mandateId)
    end
```

Every successful path emits one `InvoicePaid(orderId, payer, merchant, amount, mandateId)` event. Direct and relayed payments use `bytes32(0)` as `mandateId`; agent payments and owner-approved escalation payments carry the mandate id. That gives an indexer one event shape for reconciliation.

The EIP-3009 nonce is not supplied by the caller. It is computed on-chain as:

```solidity
keccak256(abi.encode(block.chainid, address(this), orderId, merchant, amount))
```

This binds the signature to this chain, this contract instance, the invoice id, the merchant and the amount. Reusing a signature for another order, merchant or amount fails signature recovery. The token itself consumes the nonce, and the contract consumes `orderId`, so replay is rejected on both layers.

### Function reference

| Function | Access | Purpose | Key checks / result |
| --- | --- | --- | --- |
| `constructor(address token_)` | deployer | Pins the immutable tUSD8 address | Reverts `InvalidArgument` on zero address |
| `pay(bytes32 orderId, address merchant, uint256 amount)` | any payer | Direct approve-and-pay | `nonReentrant`; validates args and `paid`; emits `InvoicePaid` with zero `mandateId` |
| `payWithAuthorization(...)` | any relayer | Relayed EIP-3009 payment | `nonReentrant`; computes bound nonce, consumes `orderId`, forwards; emits `InvoicePaid` |
| `authorizationNonce(bytes32,address,uint256)` | public view | Returns the nonce a payer must sign | `keccak256(abi.encode(chainid, contract, orderId, merchant, amount))` |
| `createMandate(address,uint128,uint128,uint64,address[])` | any owner | Creates a scoped mandate | Agent ≠ owner, `0 < perTxCap <= dailyCap`, future expiry, 1–20 non-zero merchants; emits `MandateCreated` |
| `revokeMandate(bytes32)` | mandate owner | Permanently revokes a mandate | `NotOwner`, `MandateIsRevoked`; emits `MandateRevoked`; no expiry requirement |
| `agentPay(bytes32,bytes32,address,uint256)` | mandate agent | Autonomous capped payment | `nonReentrant`; checks active mandate, allowlist, caps, spend; emits `InvoicePaid` with `mandateId` |
| `requestApproval(bytes32,bytes32,address,uint256)` | mandate agent | Opens a high-value approval request | Checks active mandate and allowlist; emits `ApprovalRequested`; returns deterministic `requestId` |
| `approveRequest(bytes32)` | mandate owner | Resolves and executes an approved request | `nonReentrant`; checks pending, owner, revocation, expiry, allowlist; bypasses caps; emits `ApprovalResolved` and `InvoicePaid` |
| `rejectRequest(bytes32)` | mandate owner | Closes a pending request | `RequestNotPending`, `NotOwner`; emits `ApprovalResolved(..., false)`; no expiry requirement |
| `token`, `paid`, `ownerNonce`, `mandates`, `allowedMerchants`, `approvals` | public view | Indexer and client state reads | Auto-generated getters |

Events: `InvoicePaid(bytes32 indexed orderId, address indexed payer, address indexed merchant, uint256 amount, bytes32 mandateId)`, `MandateCreated`, `MandateRevoked`, `ApprovalRequested`, `ApprovalResolved`.

Errors: `AlreadyPaid`, `NotAgent`, `NotOwner`, `MandateIsRevoked`, `MandateExpired`, `MerchantNotAllowed`, `PerTxCapExceeded`, `DailyCapExceeded`, `InvalidArgument`, `RequestNotPending`. The balance invariant uses Solidity `assert`, which reverts with a panic instead of a custom error.

### Agent mandates

Mandates are enforced on-chain rather than only in a database. A mandate is created by its owner with an agent address, a per-transaction cap, a daily cap, an absolute expiry and a non-empty merchant allowlist. `ownerNonce` makes mandate ids unique per owner; ids also bind chain id and contract address.

`agentPay` rejects a zero/expired order, an unknown or revoked mandate, a caller that is not the named agent, an expired mandate, a merchant outside the allowlist, an amount above the per-transaction cap and a cumulative spend above the daily cap. `spentToday` resets on a new UTC day. A rejected or reverted payment cannot consume spend because state changes are rolled back.

High-risk payments use a two-step escalation rather than weakening caps:

- `requestApproval(...)` is callable only by the mandate's agent and still checks expiry, revocation and the merchant allowlist. It returns a deterministic `requestId` bound to chain, contract, mandate, order, merchant and amount.
- `approveRequest(...)` is callable only by the mandate owner. It resolves a pending request and performs the same payment as `agentPay`, with the mandate id in the event.
- `rejectRequest(...)` closes a pending request without paying.
- An approved payment **bypasses caps** but does not change `spentToday`; that keeps the daily cap meaningful for future autonomous agent payments and makes the escalation auditable as a separate decision.

Revocation is immediate: `revokeMandate` flips the stored flag, and every agent and approval path checks it on each call. Expiry is checked on every payment and approval-resolution path, so an old signed intent cannot outlive its mandate. `rejectRequest` and `revokeMandate` do not require an unexpired mandate.

## Threat model

| Threat | Mitigation |
| --- | --- |
| Front-running a relayed payment | `receiveWithAuthorization` requires `msg.sender == to`; only this contract can consume the signature, and the contract forwards immediately to the fixed merchant in the same transaction |
| Authorization replay across invoice, merchant or amount | Nonce is computed on-chain from chain id, contract, `orderId`, merchant and amount; the token consumes the nonce and the contract consumes `orderId` |
| Relayer redirects funds | A different merchant or amount changes both the recovered signer and the computed nonce, so `InvalidSignature` reverts |
| Reentrancy | Storage-based `ReentrancyGuard` on every token-moving function; state is marked before the external token call |
| Malicious or fee-on-transfer token | `SafeERC20` plus an `assert` that contract balance is unchanged; the configured tUSD8 was checked on the fork |
| Prompt injection into an AI agent | All policy is on-chain: caps, allowlist, expiry and revocation are enforced by `QuantumPayments`, not by agent text or model output |
| Compromised agent key | Owner calls `revokeMandate`; every agent and approval-resolution path checks the revoked flag immediately |
| Compromised owner key | Out of scope for Part D; the owner can create mandates, approve payments and revoke mandates, so owner key custody is an operational requirement |
| Double payment across paths | One `paid[orderId]` mapping gates direct, relayed, agent and approved payments |
| Upgrade or admin surprise in this contract | None: no owner, upgrade, pause or initializer surface on `QuantumPayments` |

### Prompt-injection model

An agent may read invoices, merchant pages or tool output that tries to instruct it to pay another address, ignore a cap or act after expiry. The contract treats that content as irrelevant: it accepts only an allowlisted merchant, an amount within caps unless a human owner approved the exact request, a live mandate and an unpaid `orderId`. Even fully compromised agent reasoning cannot exceed those on-chain rules. The remaining trust is in the owner key, the tUSD8 proxy controlled by QDay and the backend's derivation of `orderId`.

### Known limits

- `QuantumPayments` is immutable; there is no upgrade path, no admin key and no pause.
- There are no refunds. A paid `orderId` is permanently marked paid.
- `paid`, `spentToday` and approval state live in this contract, not in the Go database; Part E must reconcile `InvoicePaid` with A3 invoice state.
- The daily bucket uses `block.timestamp / 1 days`; boundary behaviour depends on validator timestamps.
- tUSD8 itself is an EIP-1967 upgradeable proxy controlled by QDay. A token upgrade could change transfer or EIP-3009 behaviour; this contract cannot prevent that.
- Approved payments bypass automatic caps by design and do not increase `spentToday`. They require owner action and are visible through `ApprovalResolved` plus `InvoicePaid`.
- No relayer service is included. Someone must submit `payWithAuthorization`, but that relayer never holds funds.

## Security review notes

- `ReentrancyGuard` uses OpenZeppelin v5's storage slot. Transient storage was avoided because Aevum targets Shanghai.
- `SafeERC20.safeTransferFrom` handles tokens with non-standard return values. The mock and real tUSD8 both return `true`.
- The strict balance assertion catches fee-on-transfer or rebasing behaviour instead of silently underpaying a merchant. tUSD8 is not fee-on-transfer.
- The relayed path checks `msg.sender == to` inside the real tUSD8 `receiveWithAuthorization`. A direct call by a relayer is rejected by the token before any transfer; this is covered by a unit test.
- The contracts intentionally use `block.timestamp` for expiries and UTC-day buckets. On this testnet that is acceptable; it remains a linter exclusion with review responsibility.
- `foundry.toml` excludes several Foundry lint rules. Each exclusion is deliberate: `block-timestamp` for expiries, `ecrecover` inside the local mock, `reentrancy-no-eth` because guard updates precede external calls, `incorrect-strict-equality` for the balance assertion, `require-revert-in-loop` for the bounded allowlist loop, `unsafe-typecast` for bounded integer conversions and `arbitrary-send-erc20` for the payer-to-merchant design.
- No test uses a live private key. Fork tests use `deal` for balances and `vm.sign` for EIP-712 signatures.

## Tests

Run from the repository root with the Foundry binary on PATH:

```powershell
cd contracts
forge fmt --check
forge build
forge test -vv
forge test --gas-report
forge test --profile fork --match-path 'test/fork/*' --fork-url aevum --evm-version shanghai -vv
```

`no_match_path = "test/fork/*"` in the default profile keeps fork tests out of `forge test`. The `fork` profile clears that filter, and `make fork-test` selects it through `FOUNDRY_PROFILE=fork`.

### Coverage

| Area | File | What is covered |
| --- | --- | --- |
| Unit | `test/QuantumPayments.t.sol` | Direct pay, argument checks, missing allowance, idempotency, all payment events, relayed payment, signature binding to order/merchant/amount, order replay, EIP-3009 token-nonce replay, expiry window, relayer-as-recipient enforcement, cross-path order reuse, mandate validation, agent/allowlist/caps/day-reset/expiry/revocation, approval lifecycle, low allowance and low balance |
| Fuzz | `test/QuantumPayments.t.sol` | Cumulative daily spend is capped and an over-cap second payment reverts without consuming spend; `spentToday` resets after a day change |
| Invariant | `test/QuantumPayments.invariants.t.sol` | A `QuantumPaymentsHandler` target calls all three payment paths; the invariant asserts the contract token balance is always zero |
| Fork | `test/fork/AevumFork.t.sol` | Real tUSD8 EIP-712 domain, deployment under Shanghai, real-token EIP-3009 relayed payment and direct approve/pay |

Observed results in this workspace:

- `forge build --force`: `Compiling 32 files with Solc 0.8.24`, compiler run successful with no warnings.
- `forge test -vv`: 21 tests passed, 0 failed.
- `forge test --gas-report`: same 21 tests passed.
- Fork suite: 3 tests passed against `aevum` with `--evm-version shanghai`.
- Deployment gas estimate: `1,441,330`; gas-report deployment size `6,569` bytes (`forge inspect` creation bytecode `6,537` bytes plus a 32-byte constructor argument); deployed runtime bytecode `6,313` bytes.

### Gas report

Production contract from the latest `forge test --gas-report`:

| Function | Min | Avg | Median | Max | Calls |
| --- | ---: | ---: | ---: | ---: | ---: |
| `pay` | 27,319 | 78,876 | 78,883 | 95,995 | 36,505 |
| `payWithAuthorization` | 31,985 | 126,130 | 126,142 | 143,254 | 36,387 |
| `authorizationNonce` | 743 | 743 | 743 | 743 | 36,387 |
| `createMandate` | 23,230 | 143,728 | 145,526 | 162,806 | 288 |
| `revokeMandate` | 24,201 | 28,076 | 28,613 | 30,877 | 4 |
| `agentPay` | 32,483 | 93,506 | 93,242 | 116,760 | 43,876 |
| `requestApproval` | 27,570 | 89,364 | 90,236 | 148,364 | 8 |
| `approveRequest` | 29,107 | 50,316 | 35,567 | 117,791 | 5 |
| `rejectRequest` | 33,315 | 33,315 | 33,315 | 33,315 | 1 |
| `token` | 303 | 303 | 303 | 303 | 1 |
| `paid` | 2,470 | 2,470 | 2,470 | 2,470 | 6 |
| `ownerNonce` | 2,586 | 2,586 | 2,586 | 2,586 | 280 |
| `mandates` | 11,274 | 11,274 | 11,274 | 11,274 | 775 |
| `allowedMerchants` | 2,670 | 2,670 | 2,670 | 2,670 | 2 |
| `approvals` | 11,090 | 11,090 | 11,090 | 11,090 | 2 |

Test-only contracts from the same report: `MockUSD8` deploys for 753,801 gas and its EIP-3009 functions cost 24,731–80,787 gas; `QuantumPaymentsHandler` deploys for 1,602,599 gas and its handler calls average 113,726–165,484 gas. Gas varies with allowlist length and call sequence; use the raw report for any budget decision.

## Deployment

No deployment was performed for this work. Dependency strategy: `contracts/lib/` is git-ignored and restored with the documented `forge install` command rather than git submodules. The intended administrator flow is:

1. Install Foundry and run `foundryup`.
2. From `contracts/`, install dependencies with `forge install --no-git --shallow foundry-rs/forge-std OpenZeppelin/openzeppelin-contracts@v5.0.2`.
3. Run `make build`, `make test` and the fork command above.
4. Import the deployer address into a Foundry keystore. Do not pass a private key on the command line.
5. Optionally export `TUSD8_ADDRESS`; otherwise [`script/Deploy.s.sol`](../../contracts/script/Deploy.s.sol) uses the recorded testnet address.
6. Run `make deploy-aevum ACCOUNT=<foundry-keystore-name>`. The target passes `--legacy --broadcast --rpc-url https://rpc-test.qday.info --account <name> --evm-version shanghai` and logs `TOKEN` plus `QUANTUM_PAYMENTS`.
7. Verify the constructor argument and bytecode on the testnet explorer, then update this document with the new address through a separate reviewed change.

Fork tests and deployment both use the `aevum` RPC alias. The fork succeeds without broadcasting; only `make deploy-aevum` sends a transaction.

The user must fill this table after deploying and verifying on the explorer:

| Field | Value |
| --- | --- |
| Deployed `QuantumPayments` address | `<fill after deploy>` |
| Deploy transaction hash | `<fill after deploy>` |
| Constructor token address | `0x456569838f1d43009A91ebA4263147Afd7233483` |
| Deployed commit | `<fill with git rev-parse HEAD>` |
| Explorer verification link | `<fill after manual verification>` |

## Part E checklist

Part E is backend/frontend integration after the human deploys the contract. It must not be mixed into Part D.

A3 fields to add in the same commit as the Go change, then mirror in `web/src/lib/quantum-api.ts`:

- `QuantumConfig.payment_contract_address: string | null` — lower-case deployed contract address; `null` until configured.
- `Invoice.order_id: string` — `0x` + 64 hex characters from the backend derivation above.
- `Invoice.payment_contract_address: string | null` — the contract whose `InvoicePaid` event is authoritative for this invoice.
- `Checkout.order_id: string` — so the wallet/relayer can call the contract without re-deriving the UUID.
- `Checkout.payment_contract_address: string | null` — same value as config/invoice, mirrored for public clients.

Worker and API changes:

- Watch `InvoicePaid` only from the configured contract address, with `orderId` matching a stored invoice.
- Treat the event as a confirmation signal but keep the existing attempt state machine, confirmations and review paths.
- Verify `payer`, `merchant` and `amount` in the log against the registered attempt and invoice before marking `PAID`.
- Store the event transaction hash and log index for idempotent claim, mirroring today's canonical `Transfer` verification.
- Add the A3 fields to Go models, SQL columns/constraints and TypeScript mirrors together.

Event ABI for the indexer:

```text
InvoicePaid(bytes32 indexed orderId, address indexed payer, address indexed merchant, uint256 amount, bytes32 mandateId)
topic0 = 0x65ec4a5912fa4e274d3e8ff427bbdf1ea781c81dad15250fba4f80d7a7b00842
```

Frontend and relayer work for later:

- Public checkout should prepare `orderId`, payer, merchant and amount from A3, then choose direct `pay` or relayed `payWithAuthorization`.
- No gasless relayer service exists yet; a relayer needs signing support for the EIP-712 domain and the exact bound nonce.
- Agent mandate UI, approval UX and mandate audit indexing remain future work.

## Open items

- The DoraHacks rules page must be read manually before any prize or eligibility claim.
- The P0 Go API and browser checkout still use direct `transfer(receiver, amount)`; this contract is not wired into them.
- Mandate approval is an owner action in the contract. Product UI, indexing, notifications and audit integration remain future work.
- Any production use needs a fresh audit against the deployed tUSD8 implementation and a reviewed decision on timestamp/day-boundary tolerance.
