# QDay Aevum and challenge verification (2026-11-22)

Each fact below is labelled with how it was checked:

- **[RPC]**: queried directly from `https://rpc-test.qday.info` on 2026-11-22.
- **[explorer]**: from the explorer's Blockscout API.
- **[wiki]**: from wiki.qday.info.
- **[kit]**: from `github.com/qday-io/aevum-starter-kit`.
- **[qday.io]**: from `qday.io/builder-challenge`.

The DoraHacks page could not be fetched (HTTP 405), and the starter kit says the DoraHacks rules override everything else. **Read the DoraHacks page yourself before relying on any rule below.**

## 1. Network values in the spec

| Value | Spec | Verified | Source |
| --- | --- | --- | --- |
| Chain ID | 44005 | ✅ `0xabe5` = 44005 | RPC, wiki |
| RPC | `https://rpc-test.qday.info` | ✅ responds; node is `cdk-erigon/2.0-qday-v2.61.19-validium` (a Polygon CDK zk validium L2) | RPC |
| Explorer | `https://explorer-test.qday.info` | ✅ HTTP 200; it is Blockscout (`/api/v2/...`) | explorer |
| tUSD8 address | `0x4565…3483` | ✅ an ERC-20 contract with 543 holders, symbol `tUSD8` | RPC, explorer, wiki, kit |
| tUSD8 decimals | 6 | ✅ `decimals()` = 6 | RPC |
| Gas token | – | tQDAY, 18 decimals, gas price ≈ 1 gwei | wiki, RPC |
| Mainnet chain ID | – | 44002 ("in development"). Add it to the deny-list. | wiki |

## 2. Technical constraints Codex and opencode must know

1. **The tUSD8 token is an upgradeable EIP-1967 proxy** [RPC, explorer].
   - Implementation: `0xD6bE708F41b07b2F4A7B48889b8e16626e317597`.
   - Proxy admin: `0x6032e6e660d5c05846b7f305f8a186fa59c6fb88`.
   - The implementation can change, so re-run the startup check (`decimals()`, code exists) on every boot.
2. **The token is USDC-style**: `permit` (EIP-2612), `transferWithAuthorization` and `receiveWithAuthorization` (EIP-3009) all exist in the implementation bytecode [RPC]. The EIP-712 domain is name `tUSD8`, version `2` [kit]. This allows gasless payments where the payer only signs.
3. **`eth_getLogs` accepts at most 1000 blocks** per call (`block range too large, max range: 1000`) [RPC]. Scan in chunks of 1000 blocks or fewer.
4. **Blocks arrive about every second**: 1000 blocks took ≈1013 s [RPC]. A 1000-block window covers only ~17 minutes, so look up a transaction by its hash rather than scanning logs.
5. **No WebSocket RPC** [kit; a direct test confirms the server does not upgrade the connection]. The wiki lists a `wss://` endpoint, but it doesn't work, so poll over HTTPS only. viem's `watchContractEvent` must run in polling mode.
6. **Finality:**
   - There is no official confirmation count [wiki, kit]. The wiki only says "L2 soft confirmation".
   - The node supports the zkEVM RPCs `zkevm_batchNumber`, `zkevm_virtualBatchNumber` and `zkevm_verifiedBatchNumber` [RPC]. These expose the trusted, virtual and verified stages, which can serve as a stronger finality signal.
   - `QUANTUM_MIN_CONFIRMATIONS=2` remains a demo policy, not a guarantee.
7. **The EVM is Shanghai** [kit]. Cancun opcodes (`MCOPY`, `TSTORE`) revert, so compile with `evm_version = shanghai`.
8. **The kit's scripts use legacy (type 0) transactions** [kit].
9. **Explorer source verification is broken** [kit]. In place of verification, put each contract address and a link to the exact commit of its source in the README.
10. **ERC-4337:**
    - EntryPoint v0.7 is deployed at its canonical address `0x0000000071727De22E5E9d8BAf0edAc6f37da032` [RPC].
    - **No bundler or paymaster is reachable.** `bundler.qday.io` does not resolve [RPC test], and the kit says: "no officially supported bundler or paymaster… the team won't support or debug it".
    - The wiki's `@qday-io/sdk` (session keys, smart accounts) is **not published; it arrives in Q1 2027** [wiki].
    - **Do not depend on native session keys or smart accounts.** The kit's advice: "For budgets, limits, expiry and revoke, write them into your own contract."
11. **Multicall3** is at `0xcA11bde05977b3631167028862bE2a173976CA11` [kit].

## 3. Getting test tokens

- **tQDAY (gas):** from https://faucet.qday.info. qday.io says 5 tQDAY per address every 24 hours.
- **tUSD8:** only through Discord. Verified members run `/faucet <address>` in `#faucet` and receive **1 tQDAY + 5 tUSD8, once per account and once per address** [kit, qday.io].
- **Plan around this:** the whole demo has only about 5 tUSD8 per wallet. Keep demo invoices at 0.1–1 tUSD8, and get several team wallets funded.
- `mint` exists in the token, but it is a restricted minter function; you cannot mint it yourself.

## 4. Challenge rules (from qday.io and the kit; confirm on DoraHacks)

- **Deadline:** **Dec 11, 2026, 08:00 UTC** (14:00 Bangladesh time). The deadline is firm.
- **Judging:** Dec 12–14.
- **Demo Day:** **Dec 15, 06:00 UTC** (12:00 Bangladesh time). At least one team member **must attend live**; a pre-recorded video cannot replace attendance.
- **Prizes:** 1st $800, 2nd $500, 3rd $200 (USDT), plus a fast track into the QDay mainnet builder program.
- **Tracks:** Invoice Payments, Payroll/Vesting, Crowdfunding, Confirmation of Payee, Decentralized Payments, Digital Ownership, Native AI Assistants, or another business payment tool.
- **Scoring (out of 100):**

  | Criterion | Points |
  | --- | --- |
  | Innovation | **20** |
  | UX | 15 |
  | Technical quality and platform integration | 15 |
  | Real-world utility | 15 |
  | Safety and controls | 15 |
  | Demo | 10 |
  | Documentation | 10 |

  "Ecosystem contribution", which appears in the local criteria file, is **not** on qday.io.
- **Deliverables:** a public repo, evidence of Aevum integration ("such as smart contracts on Aevum Testnet"), a demo, prototype or Docker image, and documentation. The kit also recommends listing contract addresses, the exact commit and a few transaction hashes.
- **"Judging counts activity on Aevum"**: all settlement must happen on Aevum.

### ⚠️ Eligibility blocker [qday.io FAQ]

> "All core building must happen during the build period, Nov 16 to Dec 11."
> "Adding tUSD8 to an existing product, or submitting an existing product as it is, doesn't count as a submission."

**The current plan ("PayCore Quantum": tUSD8 checkout added to the existing PayCore) matches the disallowed case almost word for word.** Boilerplate and templates are allowed if their license permits, with credit.

Before building further, choose one:

- **A. Ask the organizers in writing (Discord or DoraHacks)** whether a new product built in a new repo during the build window may reuse your own MIT-licensed PayCore code as boilerplate. Credit it, and keep the reused code clearly separated.
- **B. Build a new, standalone product in a new repo.** Write the core (contracts, payment flow, agent mandates) new during Nov 16–Dec 11. Copy only generic pieces (auth, UI shell) from PayCore, with credit headers and a NOTICE file.

## 5. Gaps in the current plan revealed by this check

1. **There is no smart contract.** The plan only verifies plain ERC-20 transfers, but judging rewards "smart contracts on Aevum Testnet" and Aevum activity.
   - Add an invoice contract in the style of the kit's `Checkout.sol`: it pays an `orderId` once and emits `Paid(orderId, payer, amount)`.
   - This also fixes the plan's weakest point: the backend matches on the `Paid` event's `orderId` instead of guessing from amount and payer.
2. **Originality is low.** Public entries already include PayLink, AevumPay, Aevum Pay, RelayPay and StablePay, all doing invoices or checkout. Innovation is the biggest criterion at 20 points.
   - The differentiator should be an **on-chain Agent Spend Mandate contract**: per-transaction and daily caps, a merchant allowlist, expiry, instant revoke, and human approval above the cap.
   - Pair it with gasless EIP-3009 payments, which the token supports.
   - Note that some rivals (paysure-agent, ImmuneVault, zambo-receipt-rail) also do agent budgets, so the edge must come from PayCore's strengths: audit chain, idempotency and RBAC.
3. **Session keys and smart accounts are not usable yet.** Spending limits and expiry must live in our own contract, not in QDay-native primitives.
4. **Codex and opencode prompt updates needed:** no WebSocket, 1000-block log chunks, Shanghai EVM, add 44002 to the mainnet deny-list, small demo amounts, and token-proxy awareness.

## Sources

- https://wiki.qday.info/ (chains, tokens, faucet, network-configuration, erc-4337-development, payment-development, sdk-code-examples, wallet-integration, getting-started, smart-contract-development)
- https://qday.io/builder-challenge
- https://github.com/qday-io/aevum-starter-kit
- https://dorahacks.io/hackathon/qday (not fetchable here; read it manually)
- https://x.com/QDayOfficial/status/2119647054918217955
- Public competitor repos found by search: github.com/ZhangHaozhi001/aevum-paylink, github.com/Tiannuryadins/aevum-pay, github.com/ZQ051031/aevum-builder-challenge, github.com/Ajengayukusu/paysure-agent, github.com/LeDenti/antitout-aevum, github.com/zambodotdev/zambo-receipt-rail
