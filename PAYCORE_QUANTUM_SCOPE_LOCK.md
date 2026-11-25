# PayCore Quantum: Scope Lock (feature freeze)

**কার্যকর:** 2026-11-25, শেষ হবে submission-এর পর (**2026-12-11 08:00 UTC**)।
**সবার জন্য বাধ্যতামূলক:** মানুষ, Codex, opencode, Claude, OpenSpec change। এই ফাইল `PAYCORE_QUANTUM_CODEX_PROMPT.md` আর `PAYCORE_QUANTUM_EXTENSION_SPEC.md`-এর ওপরে প্রাধান্য পায়।

> **নিয়ম এক লাইনে:** নতুন feature যোগ হবে না। শুধু নিচের তালিকার কাজ শেষ করা, bug fix, আর demo, docs ও submission-এর প্রস্তুতি চলবে।

Scope ঠিক করা হয়েছে challenge-এর প্রত্যাশা থেকে: [qday.io/builder-challenge](https://qday.io/builder-challenge)-এর deliverables, `PAYCORE_QUANTUM_JUDGING_CRITETIA.md`-এর judging criteria, আর `PAYCORE_QUANTUM_QDAY_VERIFICATION.md`-এ যাচাই করা নিয়ম। প্রত্যাশার বাইরে কিছু নেই, আর প্রত্যাশার ভেতরের কিছু বাদও পড়েনি।

---

## ১. Challenge যা চায় → আমাদের locked উত্তর

| প্রত্যাশা (source) | Locked deliverable | এখনকার অবস্থা |
| --- | --- | --- |
| One end-to-end payment flow on Aevum testnet (qday.io) | Merchant invoice → public checkout → wallet payment → backend on-chain verification → `PAID` | ✅ direct transfer আর contract `pay()` দুটোই implement করা; ❌ live testnet-এ এখনো চালানো হয়নি |
| Smart contract on Aevum, settlement on Aevum (qday.io, starter kit) | `contracts/src/QuantumPayments.sol` deploy করা, address আর commit docs-এ | ✅ code ও tests আছে; ❌ deploy হয়নি |
| Verifiable on-chain activity: tx hash, explorer link (criteria ২) | README/DEMO-তে deploy tx, কয়েকটা payment tx আর একটা agent payment tx | ❌ |
| Agentic payments, spend caps, revocation (criteria ৪, ৫, ৭) | **আগে থেকে থাকা** contract mandate (`createMandate`, `agentPay`, caps, expiry, `revokeMandate`, approvals), backend `agentPay`-কে মেনে নেয়, demo **script** দিয়ে | ✅ contract; ⚠️ verifier `agentPay` reject করে (tx sender ≠ payer) |
| Safety: no custody, least privilege, audit (criteria ৭) | Noncustodial checkout, RBAC defaults (migration 028), exact-match verification, review state | ✅ |
| Clean, tested code that handles retries and async confirmations (criteria ৫) | Existing tests সব green, আর নিচের locked bug fix | ⚠️ Part F fix বাকি |
| UX: clear status, plain-language errors, mobile (criteria ১) | Existing checkout ও merchant console; lost-hash recovery | ⚠️ reload করলে hash হারায় |
| Documentation: README, flowchart, env vars, testnet addresses, steps (criteria ৩) | README Quantum section, `docs/quantum/*`, user guide `/docs`, ERD | ✅ লেখা; ❌ deployed address, tx hash, screenshot বাকি |
| Live deployment: demo, prototype বা Docker image (qday.io) | Deployed demo URL **অথবা** `compose.yaml` Docker package | ❌ |
| Short demo + live demo on Dec 15 06:00 UTC (qday.io) | `docs/quantum/DEMO.md` অনুযায়ী ৩–৫ মিনিটের ভিডিও; একজন সদস্য live উপস্থিত | ❌ |
| Eligibility: core building inside the window (qday.io FAQ) | Organizer-এর লিখিত উত্তর; `docs/quantum/ELIGIBILITY.md` মানুষ লিখবেন | ❌ **blocker** |

---

## ২. LOCKED IN: যা শেষ করতেই হবে (এর বাইরে কিছু নয়)

### A. Blocker (মানুষের কাজ)
1. Organizer-এর কাছ থেকে eligibility-র লিখিত উত্তর। উত্তর "না" হলে শুধু **repo বদলাবে, scope বদলাবে না**।
2. Discord faucet থেকে tUSD8 আর tQDAY: অন্তত ২টা wallet (payer, agent) আর merchant receiver-এর জন্য।
3. `make deploy-aevum` দিয়ে `QuantumPayments` deploy করা, তারপর `QUANTUM_PAYMENT_CONTRACT_ADDRESS` সেট করা।

### B. Bug fix: আগে থেকে থাকা feature ঠিকমতো কাজ করানো (`PAYCORE_QUANTUM_CODEX_PROMPT.md` Part F)
| ID | কাজ | কেন locked |
| --- | --- | --- |
| F1 | Broadcast-এর পরে tx hash sessionStorage-এ রাখা; reload করলে resume; same-hash submit idempotent; recover endpoint | Double payment ঠেকানো (criteria ১, ৫) |
| F2 | একবার RPC miss হলে reorg না ধরা (positive evidence অথবা পরপর ৫টা miss লাগবে) | Demo চলার সময় PAID → PENDING উল্টে যাওয়া ঠেকানো |
| F3 | Rate limiter: পূর্ণ table-এ নতুন client আটকানো যাবে না; `TRUSTED_PROXIES` | Deployed demo proxy-র পেছনে চলবে |
| F4 | Clock-skew tolerance | বৈধ পেমেন্ট যেন "late" না হয় |

### C. Existing feature সম্পূর্ণ করা (নতুন feature নয়)
| ID | কাজ | সীমা |
| --- | --- | --- |
| C1 | Verifier `agentPay` আর `payWithAuthorization`-এর `InvoicePaid` মেনে নেবে: payer যাচাই হবে event থেকে; tx sender = mandate agent বা relayer | Contract-এ এই path আগেই আছে; শুধু backend-এর reject তুলে দেওয়া। Tests: agent path settle হবে, revoked বা অন্য order reject হবে |
| C2 | `contracts/script/` বা `api/load/`-এর মতো জায়গায় **agent demo script**: mandate create → `agentPay` → over-cap `requestApproval` → `revokeMandate` → revoke-এর পরের pay fail | কোনো নতুন UI নয়। Foundry `cast`/`forge script` ব্যবহার হবে, key থাকবে keystore-এ |

### D. Demo ও submission
1. Deploy address, deploy tx, exact commit: `docs/quantum/CONTRACTS.md` টেবিলে।
2. Live tx: direct payment, contract payment, agent payment আর revoke। প্রতিটা explorer link সহ README আর DEMO-তে।
3. Screenshot: merchant console, checkout (mobile), PAID, review state।
4. Live demo URL অথবা Docker package নির্দেশনা।
5. ৩–৫ মিনিটের demo video, DEMO.md-এর script ধরে।
6. `docs/quantum/ELIGIBILITY.md` (মানুষ লিখবেন) আর NOTICE/license credit।
7. DoraHacks submission; Dec 15-এর live demo-র জন্য একজন নির্ধারিত সদস্য।

---

## ৩. FROZEN: যেমন আছে তেমন থাকবে (শুধু critical bug fix)

Ledger, transfers, reversals, reconciliation, settlements, notifications, outbox, simulator, reports, users/RBAC/MFA, user guide `/docs`, ERD।

- নতুন endpoint, page, column, permission বা role যোগ হবে না।
- Bug fix করা যাবে শুধু যদি demo flow ভাঙে, নিরাপত্তার ঝুঁকি থাকে, বা টেস্ট ভাঙে।
- Pitch-এ এগুলো "PayCore-এর ভিত্তি" হিসেবে উল্লেখ হবে, নতুন কাজ হিসেবে নয়।

## ৪. OUT OF SCOPE: submission-এর আগে করা হবে না

- Mandate বা agent-এর **web UI** (create, revoke বা approval screen)। Demo চলবে script আর explorer দিয়ে।
- Gasless checkout-এর web UI ও relayer service (contract path থাকবে; C1 শুধু backend-এর acceptance)।
- Webhook endpoint UI বা delivery (P1), merchant dashboard-এর বাড়তি অংশ, analytics।
- ERC-4337, session key, smart account, paymaster। QDay-এ এগুলো supported নয় (verification §2.10)।
- Mainnet, real money, fiat, refund, multi-token, multi-chain।
- নতুন role বা permission, ledger-এ Quantum posting।
- Refactor বা rename, যদি না তা কোনো locked কাজের জন্য সরাসরি দরকার হয়।
- নতুন dependency, যদি না locked কাজের জন্য অপরিহার্য হয় (তখন exact version আর license লিখতে হবে)।

---

## ৫. Change control

1. নতুন কিছু করার আগে প্রশ্ন: **এটা কি §২-এর কোনো ID?** না হলে করবেন না।
2. Scope বদলাতে হলে মানুষের লিখিত অনুমতি লাগবে, আর এই ফাইলের §৬-এ একটা লাইন যোগ করতে হবে (তারিখ, কী, কেন, কী বাদ গেল)। একটা জিনিস ঢুকলে সমান আকারের একটা জিনিস বের হবে।
3. প্রতিটা commit message-এ locked ID থাকবে, যেমন `fix(quantum): F2 reorg needs positive evidence`। ID ছাড়া কোনো feature commit হবে না।
4. Migration নম্বর নেওয়ার আগে `ls api/database/migrations` দেখুন। একসাথে একাধিক agent কাজ করছে।
5. প্রতিটা কাজের শেষে চেক: `cd api; go vet ./...; go test ./...; golangci-lint run` আর `cd web; npm run check; npm test`। Contract বদলালে `cd contracts; forge test`।

## ৬. Scope change log

| তারিখ | পরিবর্তন | কারণ | বদলে কী বাদ গেল | অনুমোদন |
| --- | --- | --- | --- | --- |
| 2026-11-25 | Scope lock তৈরি | Challenge-এর প্রত্যাশার সাথে মেলানো; নতুন feature বন্ধ | – | User |

## ৭. Timeline (deadline: Dec 11 08:00 UTC = বাংলাদেশ সময় দুপুর ২টা)

| তারিখ | লক্ষ্য |
| --- | --- |
| Nov 25–26 | A1 eligibility উত্তর; A2 faucet; F1–F4 |
| Nov 27–29 | A3 deploy; C1; C2 script |
| Nov 30–Dec 3 | Live testnet run: সব tx hash সংগ্রহ; bug fix |
| Dec 4–6 | Demo deploy বা Docker; screenshot; README ও DEMO আপডেট |
| Dec 7–8 | Video রেকর্ড; ELIGIBILITY.md; NOTICE |
| Dec 9 | Full regression; submission draft |
| Dec 10 | Submit (একদিন আগে) |
| Dec 11 | শুধু buffer; কোনো code change নয় |
| Dec 15 06:00 UTC | Live demo (বাংলাদেশ সময় দুপুর ১২টা) |
