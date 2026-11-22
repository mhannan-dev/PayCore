# PayCore User Guide: কে, কেন, কীভাবে

প্রথমবার app চালানো থেকে transfer বা testnet payment যাচাই করা পর্যন্ত **কোন কাজের পরে কোন কাজ করবেন**, এই guide সেই ক্রমে সাজানো। আগে নিচের checklist অনুসরণ করুন, তারপর আপনার কাজের workflow বেছে নিন। Table-এর সম্পর্ক বুঝতে [ERD ও data flow](erd.md) দেখুন; আরও setup বিকল্প আছে [README](../README.md#getting-started)-তে।

> **PayCore একটা alpha software, শুধু learning, demo আর testnet-এর জন্য।** আসল টাকা লেনদেনে এটা ব্যবহার করবেন না।

## সূচিপত্র (Contents)

- [শুরু করুন এখান থেকে](#শুরু-করুন-এখান-থেকে)
- [কাজ অনুযায়ী সঠিক ক্রম](#কাজ-অনুযায়ী-সঠিক-ক্রম)
- [কে ব্যবহার করবেন (Who)](#কে-ব্যবহার-করবেন-who)
- [কেন PayCore (Why)](#কেন-paycore-why)
- [কীভাবে ব্যবহার করবেন (How)](#কীভাবে-ব্যবহার-করবেন-how)
  - [১. চালু করা ও sign in](#১-চালু-করা-ও-sign-in)
  - [২. Console-এ পথ চেনা](#২-console-এ-পথ-চেনা)
  - [৩. Role অনুযায়ী রোজকার কাজ](#৩-role-অনুযায়ী-রোজকার-কাজ)
  - [৪. PayCore Quantum: testnet-এ tUSD8 পেমেন্ট](#৪-paycore-quantum-testnet-এ-tusd8-পেমেন্ট)
- [যে নিয়মগুলো আপনাকে সুরক্ষা দেয়](#যে-নিয়মগুলো-আপনাকে-সুরক্ষা-দেয়)
- [সমস্যা ও সমাধান (Troubleshooting)](#সমস্যা-ও-সমাধান-troubleshooting)
- [আরও জানতে](#আরও-জানতে)

## শুরু করুন এখান থেকে

**সাধারণ ledger demo-এর পথ:** setup → database → sample users ও JPY ledger → app চালু → admin sign-in → currency JPY → Accounts → Transfer studio → Ledger explorer → Invariant audit। Quantum চালু করা এই পথের জন্য দরকার নেই।

**শুধু merchant-এর checkout link থেকে pay করবেন?** Local setup, seed ও PayCore sign-in বাদ দিয়ে সরাসরি [payer-এর ধাপ](#payer-এর-ক্রম) অনুসরণ করুন। Merchant-এর server আগে থেকেই চালু থাকতে হবে।

### প্রথমবারের setup checklist

Windows PowerShell-এ project root থেকে শুরু করুন। Go, Node.js/npm এবং Docker Desktop আগে install করুন; Docker Desktop চালু রাখুন। প্রতিটি ধাপের ফল মিলিয়ে তারপর পরের ধাপে যান।

| ধাপ | যা করবেন | সফল হলে যা দেখবেন | এরপর |
| --- | --- | --- | --- |
| ১ | নিচের command দিয়ে অনুপস্থিত local config কপি করুন | Root `.env`, `api/.env` ও `web/.env.local` আছে; আগের config মুছে যায়নি | MFA key তৈরি |
| ২ | `api/` থেকে `go run ./cmd/mfa-key`; editor-এ root `.env`-এ একই key বসান | Key saved/preserved; দুই config-এ `MFA_ENCRYPTION_KEY` পূরণ করা আছে | Database চালু |
| ৩ | Root থেকে `docker compose up -d --wait postgres mailpit` | PostgreSQL healthy; local inbox চালু | Sample users তৈরি |
| ৪ | `api/` থেকে `go run ./cmd/seed-users` | চারটি sample email-এর inserted/skipped ফল; password আপনি prompt-এ দিয়েছেন | Demo ledger তৈরি |
| ৫ | একই `api/` terminal-এ `go run ./cmd/seed-ledger` | JPY account ও journal created/exists/replayed | App চালু |
| ৬ | Root থেকে `.\run.ps1` চালিয়ে terminal খোলা রাখুন | API ও web-এর নির্বাচিত port/URL terminal-এ দেখাবে | Sign-in |
| ৭ | Web URL-এ `admin@yopmail.com`, seed password ও CAPTCHA দিয়ে sign-in করুন | MFA চালু থাকলে enrollment/verification, তারপর Overview | JPY বেছে Accounts খুলুন |
| ৮ | Header currency **JPY** করুন, Accounts খুলুন | `1001`, `2001`, `2002`, `2003`, `4001` sample account ও balance | নিচের transfer workflow |

**ধাপ ১ — শুধু অনুপস্থিত config কপি করুন:**

```powershell
# Project root
if (-not (Test-Path api/.env)) { Copy-Item api/.env.example api/.env }
if (-not (Test-Path web/.env.local)) { Copy-Item web/.env.example web/.env.local }
if (-not (Test-Path .env)) { Copy-Item .env.example .env }

# ধাপ ২: key তৈরি; existing key অপরিবর্তিত থাকবে
Push-Location api
go run ./cmd/mfa-key
Pop-Location
```

এখানে একবার থামুন: editor-এ `api/.env` খুলে `MFA_ENCRYPTION_KEY`-এর value root `.env`-এর একই setting-এ বসান। Existing key থাকলে সেটিই রাখুন। কোনো key chat, screenshot বা Git-এ দেবেন না। Compose পুরো config পড়ে বলে শুধু PostgreSQL/Mailpit চালাতেও root key খালি রাখা যাবে না। তারপর:

```powershell
# Project root
# ধাপ ৩: database ও development inbox
docker compose up -d --wait postgres mailpit

# ধাপ ৪–৫: migrations, users ও sample ledger
Push-Location api
go run ./cmd/seed-users
go run ./cmd/seed-ledger
Pop-Location

# ধাপ ৬: এই terminal চালু রাখুন
.\run.ps1
```

**Sign-in-এর সময়:** `MFA_ENABLED=true` ডিফল্ট। Admin হলে authenticator app-এর setup/verification শেষ করুন এবং recovery code সংরক্ষণ করুন। আগে MFA pause করা local config থাকলে শুধু password/CAPTCHA দেখা যেতে পারে; বিস্তারিত [admin_mfa.md](admin_mfa.md)। নতুন setup-এ MFA key বাদ দেবেন না।

**আগে থেকেই app setup করা আছে?** আবার config কপি, key বদল বা user reset করবেন না। Docker Desktop চালু করুন → root থেকে `.\run.ps1` → terminal-এর web URL খুলুন → নিজের account দিয়ে sign-in করুন। সাধারণ seed rerun পুরনো user-এর password বদলায় না; `-refresh` একটি আলাদা reset operation, এই walkthrough-তে সেটি দরকার নেই।

Default web URL `http://localhost:3000`, API `http://localhost:8080`, Mailpit `http://localhost:8025`। Port ব্যস্ত থাকলে launcher অন্য port নিতে পারে; terminal-এর URL-ই ব্যবহার করুন। Quantum ব্যবহার করলে তার checkout base URL-ও ওই web URL-এর সঙ্গে মিলতে হবে।

## কাজ অনুযায়ী সঠিক ক্রম

### Ledger transfer-এর ক্রম

আগে setup checklist-এর ধাপ ৮ পর্যন্ত শেষ করুন। প্রথম demo-তে seeded **admin** account ব্যবহার করুন, যাতে প্রয়োজনীয় পেজগুলোতে access থাকে।

| ক্রম | পেজ ও কাজ | সফলতার প্রমাণ | এরপর |
| --- | --- | --- | --- |
| ১ | **Accounts**: JPY, `2001` Akira wallet ও `2002` Hana wallet-এর balance দেখুন | Source wallet-এ demo transfer-এর জন্য যথেষ্ট balance আছে | Transfer studio |
| ২ | **Transfer studio**: `2001` debit, `2002` credit, amount `100`, description দিন | Preview-তে debit = credit এবং currency JPY | একবার submit |
| ৩ | Transfer submit করুন | Posted journal/reference দেখায়; সেটি সংরক্ষণ করুন | Ledger explorer |
| ৪ | **Ledger explorer**: ওই journal খুলুন | একই journal-এ debit ও credit posting দুটোই আছে | Accounts ও Invariant audit |
| ৫ | **Accounts** ও **Invariant audit** দেখুন | `2001` কমেছে, `2002` বেড়েছে; JPY audit-এ difference `0` | Reports অথবা নিচের optional workflow |

JPY-তে `100` মানে ১০০ yen; USD/EUR-এর মতো দুই ঘর দশমিক নয়। অন্য currency বেছে নিলে seed করা JPY account দেখা যাবে না। Transfer submit-এর পর network error হলে একই form retry করুন; নতুন logical transfer বানানোর আগে আগের journal হয়েছে কি না মিলিয়ে নিন।

**ভুল transfer হলে:** original journal খুঁজুন → অনুমতি থাকলে **Reverse** করুন → নতুন reversal journal দেখুন → Accounts ও Invariant audit আবার মিলিয়ে নিন। Original journal edit/delete করবেন না। পরবর্তী transfer-এর কারণে balance কমে গেলে reversal insufficient funds-এ ব্যর্থ হতে পারে; ফল যাচাই না করে reversal সফল ধরে নেবেন না।

### Sandbox settlement-এর ক্রম

শুরুতে একই currency-তে funded customer wallet, `PAYOUT_CLEARING`-সহ active LIABILITY account এবং `OPERATING_BANK`-সহ active ASSET account দরকার। JPY demo seed এগুলো তৈরি করে।

1. **Settlements** খুলুন → currency JPY → source wallet, amount ও beneficiary তথ্য দিয়ে payout reserve করুন।
2. `RESERVED` দেখুন: funds wallet থেকে clearing-এ গেছে। এখন **একটি** পরবর্তী কাজ বেছে নিন।
3. Payout সম্পন্ন করার demo হলে **Capture** → `CAPTURED`; বাতিলের demo হলে **Release** → `RELEASED`, funds wallet-এ ফিরে যাবে।
4. Linked journal খুলে postings ও balance মিলিয়ে নিন। শেষ হওয়া payout-এ আবার capture/release করবেন না।

এটি sandbox accounting workflow; কোনো external bank-এ টাকা পাঠানো হয় না।

### Reconciliation-এর ক্রম

Ledger-এ posting তৈরি হওয়ার **পরে** statement মিলাবেন।

1. **Reconciliation** → সঠিক account ও currency বেছে CSV template/format দেখুন।
2. CSV upload → preview-এর validation error ঠিক করুন → import করুন।
3. Import খুলুন → matching-এর direction/date settings মিলিয়ে suggestions দেখুন।
4. Statement row ও ledger posting মিললে **Confirm**; না মিললে reject/note দিয়ে manual review রাখুন।
5. Matched/unmatched row আবার দেখুন। Match confirm করলে balance বদলায় না; এটি আগের posting-এর সঙ্গে সম্পর্ক তৈরি করে।

CSV columns, sign convention ও permission: [reconciliation.md](reconciliation.md)।

### নতুন teammate-এর ক্রম

Admin sign-in → **Users & access** → user invite ও role নির্বাচন → development-এ Mailpit থেকে invitation email → teammate link খুলে password সেট করেন → নিজের email/password/CAPTCHA দিয়ে sign-in → role অনুযায়ী কাজ। Admin-এর session বা password teammate-কে দেবেন না।

### Quantum merchant-এর ক্রম

এটি ledger transfer থেকে আলাদা পথ। JPY seed account বা settlement সম্পন্ন করা Quantum invoice তৈরির শর্ত নয়।

| ক্রম | যা আগে লাগবে | কাজ | সফল হলে পরের ধাপ |
| --- | --- | --- | --- |
| ১ | PayCore stack ও local config | `QUANTUM_ENABLED=true` এবং বাকি Quantum config মিলিয়ে API restart | `/api/v1/quantum/config`-এ `enabled:true` |
| ২ | `quantum:read` ও `quantum:write`; ডিফল্টে ADMIN আর ACCOUNTANT-এ থাকে | **Quantum Payments** খুলুন | Setup form বা existing merchant profile |
| ৩ | Payment গ্রহণের testnet address | Merchant display name ও receiver address সংরক্ষণ | New invoice; receiver পরে পরিবর্তন করা যায় না |
| ৪ | Positive tUSD8 amount, description, expiry ৫–১৪৪০ মিনিট | **New invoice** dialog submit | `PENDING` invoice ও checkout link |
| ৫ | Invoice এখনও payable | Checkout link copy করে payer-কে দিন | নিচের payer workflow |
| ৬ | Payer transfer করেছেন | Invoice detail ও attempts দেখুন; প্রতি ১০ সেকেন্ডে refresh হয় | Backend `PAID` ও confirmed transaction দেখালে payment যাচাই সম্পন্ন |

Config-এর network/token মান ও faucet নির্দেশনা [Quantum API](quantum/API.md#configuration-and-readiness) এবং [verification notes](../PAYCORE_QUANTUM_QDAY_VERIFICATION.md) অনুযায়ী মিলিয়ে নিন। API config unavailable হলে merchant বা payer ধাপে যাবেন না। শুধু সাধারণ ledger demo চাইলে Quantum বন্ধ রেখেই app ব্যবহার করা যায়।

### Payer-এর ক্রম

1. Merchant-এর checkout link খুলুন; PayCore sign-in দরকার নেই। Receiver, amount, token, chain ও expiry দেখুন।
2. নিজের testnet wallet-এ ওই token এবং gas-এর test funds আছে নিশ্চিত করুন; **Connect wallet** চাপুন।
3. ভুল network দেখালে **Switch network** করুন; তারপর সঠিক chain নিশ্চিত করুন।
4. **Pay** চাপুন → app payment attempt register করে → wallet-এ exact transfer approve করুন।
5. Wallet থেকে hash পাওয়া মানে broadcast; backend verification ও confirmation শেষ না হওয়া পর্যন্ত **PAID** ধরে নেবেন না।
6. Checkout-এ `PAID` এবং explorer link দেখুন → merchant invoice detail-এও একই paid transaction মিলিয়ে দেখুন।

Unused attempt ১৫ মিনিটে expire হয়। Wallet prompt দীর্ঘক্ষণ খোলা থাকলে sign করার আগে prompt cancel করে নতুন attempt শুরু করুন। **একবার broadcast হয়ে গেলে** API error, `attempt_expired`, `REVIEW_REQUIRED` বা reorg দেখে আবার transfer পাঠাবেন না; hash/explorer link নিয়ে merchant-এর সঙ্গে যোগাযোগ করুন। Automatic refund নেই।

### কোনটা এখন ব্যবহার করবেন, কোনটা অপেক্ষায়

| অংশ | এই guide-এ অবস্থান |
| --- | --- |
| Accounts, transfer, reversal, audit, statement matching | বর্তমান ledger workflow |
| Settlements ও notification receipts | বর্তমান sandbox workflow; external bank/notification delivery ধরে নেবেন না |
| Quantum invoice ও direct token checkout | বর্তমান testnet workflow; accounting ledger-এ posting তৈরি করে না |
| Smart contract deployment, agent mandates, gasless relayer ও contract-based app integration | এই walkthrough-এর prerequisite নয়; contract docs/deployment ও পরবর্তী integration আলাদা milestone। শুধু code/doc আছে দেখে deployed বা app-এ connected ধরে নেবেন না |

## কে ব্যবহার করবেন (Who)

PayCore একটা ছোট payment বা wallet ব্যবসার **operations console**, সাথে একটা open-source codebase, যারা এমন সিস্টেম বানান সেই engineer-দের জন্য। প্রত্যেকে এক বা একাধিক **role** নিয়ে sign in করেন। Role ঠিক করে কে কোন পেজ আর বাটন দেখবেন, আর API প্রতিটা request-এ একই permission আবার যাচাই করে।

| আপনি যদি… | Role | PayCore দিয়ে যা করবেন |
| --- | --- | --- |
| **Administrator**, টিম ও প্ল্যাটফর্ম পরিচালনা করেন | `ADMIN` | মানুষ invite করা, role দেওয়া, account deactivate করা, account খোলা, transfer reverse করা, settlement চালানো, report দেখা, event outbox দেখা, failed event replay করা, idempotency simulator চালানো, audit log পড়া, আর নিচের সব কাজ |
| **Accountant**, রোজকার লেনদেন সামলান | `ACCOUNTANT` | Account ও journal দেখা, balanced transfer পোস্ট করা, statement import ও matching, report দেখা, settlement reserve, Quantum invoice |
| **Compliance auditor**, হিসাব যাচাই করেন | `COMPLIANCE_AUDITOR` | Ledger পড়া, zero-sum audit চালানো, report দেখা ও export করা, audit trail আর event outbox দেখা। শুধু read-only |
| **Merchant**, testnet-এ stablecoin নেন | `quantum:read` আর `quantum:write` আছে এমন যেকোনো role (ডিফল্টে ADMIN আর ACCOUNTANT) | tUSD8 invoice বানানো, checkout link শেয়ার করা ([অংশ ৪](#৪-paycore-quantum-testnet-এ-tusd8-পেমেন্ট)) |
| **Payer**, merchant-কে টাকা দেবেন | কোনো account লাগবে না | Checkout link খুলে নিজের wallet থেকে পেমেন্ট করা |
| **Developer বা student** | – | Production-মানের ledger শেখা: double-entry, idempotency, locking, RBAC, outbox ([ledger.md](ledger.md), [coding_rules.md](coding_rules.md)) |

একজনের একাধিক role থাকতে পারে, তখন সব role-এর permission যোগ হয়। পুরো তালিকা: [rbac.md](rbac.md#default-role-matrix)।

## কেন PayCore (Why)

টাকার software সাধারণত কয়েকটা পরিচিত কারণে ভুল করে: floating-point সংখ্যায় টাকা রাখা, retry করলে দুইবার charge হওয়া, হাতে balance বদলানো, আর admin কী করেছে তার কোনো রেকর্ড না থাকা। PayCore এমনভাবে বানানো যে এই ভুলগুলো শুধু নিরুৎসাহিত করা হয় না, **সরাসরি refuse করা হয়**:

- **টাকা সবসময় integer।** মুদ্রার সবচেয়ে ছোট একক (yen, cent, tUSD8 base unit) হিসেবে রাখা হয়, তাই rounding-এর কোনো চমক নেই।
- **প্রতিটা transfer double-entry।** Debit আর credit সমান না হলে database নিজেই journal প্রত্যাখ্যান করে। পোস্ট করা journal কখনো edit বা delete করা যায় না। ভুল শোধরানো হয় **reversal** দিয়ে, তাই history সৎ থাকে।
- **Retry নিরাপদ।** টাকা সরানোর প্রতিটা request-এ একটা `Idempotency-Key` থাকে। একই request আবার পাঠালে দ্বিতীয়বার পোস্ট না করে আগের ফলাফলটাই ফেরত দেয়। [Request simulator](#administrator) এটা লাইভ প্রমাণ করে।
- **প্রতিটা সংবেদনশীল কাজের রেকর্ড থাকে।** Sign-in, role বদল, replay সব যায় একটা append-only audit log-এ, যেটা database নিজেই রক্ষা করে।
- **Least privilege।** User শুধু তার role-এর অনুমতি অনুযায়ী দেখতে ও করতে পারেন। বাটন লুকানো শুধু সুবিধার জন্য, আসল নিয়ম API প্রয়োগ করে।
- **পেমেন্ট যাচাই করা হয়, বিশ্বাস করা হয় না।** PayCore Quantum-এ browser শুধু transaction hash জানায়। Invoice-কে paid দেখানোর আগে server নিজে blockchain থেকে মিলিয়ে দেখে।

টিমের জন্য এর মানে কম incident, আর "কে কখন এটা করল?" প্রশ্নের নিশ্চিত উত্তর। শিক্ষার্থীর জন্য, এই guarantee-গুলো কীভাবে বানাতে হয় তার একটা পড়ার মতো উদাহরণ।

## কীভাবে ব্যবহার করবেন (How)

### ১. চালু করা ও sign in

1. নতুন local setup হলে আগে [setup checklist](#প্রথমবারের-setup-checklist) শেষ করুন। Windows-এ `.\run.ps1`, অন্য OS-এ [Manual setup](../README.md#manual-setup-any-os) দেখুন। Web URL terminal থেকে নিন; default `http://localhost:3000`।
2. **শুধু প্রথমবার:** sample user আর sample ledger data বানান।
   ```powershell
   cd api
   go run ./cmd/seed-users      # একটা password চাইবে; সব sample user একই password পাবে
   go run ./cmd/seed-ledger     # sample chart of accounts ও journal (JPY)
   ```
   এতে তৈরি হবে `admin@yopmail.com`, `accountant@yopmail.com` আর `auditor@yopmail.com` ([বিস্তারিত](user_seeder.md))।
3. Email, password আর ছোট একটা অঙ্কের CAPTCHA-র উত্তর দিয়ে sign in করুন।
4. **Administrator**-রা প্রথম sign-in-এ two-factor authentication (authenticator app) সেট করবেন। `MFA_ENABLED=true` (ডিফল্ট) থাকলে এটা বাধ্যতামূলক। Recovery code নিরাপদ জায়গায় রাখুন ([admin_mfa.md](admin_mfa.md))।
5. নতুন সদস্যকে আপনি কখনো password দেবেন না। Admin তাঁকে invite করবেন, আর তিনি email-এর লিংক থেকে নিজের password ঠিক করবেন। Password ঠিক ১০ অক্ষরের হতে হবে। Development-এ email আসে Mailpit-এ: `http://localhost:8025`।

Password ভুলে গেছেন? Sign-in পেজে **Forgot password** ব্যবহার করুন। লিংক একবারই কাজ করে, আর নির্দিষ্ট সময় পর মেয়াদ শেষ হয়।

### ২. Console-এ পথ চেনা

Sidebar-এ শুধু সেই পেজগুলো দেখায়, যেগুলোর অনুমতি আপনার role-এ আছে। অনুমতি না থাকলে পেজে data-র বদলে *access denied* দেখায়।

| পেজ | কী কাজে লাগে |
| --- | --- |
| **Overview** | Balance, সাম্প্রতিক লেনদেন আর system health এক নজরে |
| **Accounts** | Chart of accounts ও লাইভ balance; নতুন account খোলা |
| **Transfer studio** | Balanced transfer পোস্ট করা বা reverse করা |
| **Reconciliation** | Bank statement (CSV) import করে ledger-এর সাথে মেলানো |
| **Ledger explorer** | Journal আর তার debit/credit posting দেখা |
| **Invariant audit** | প্রতিটা currency-তে মোট debit = মোট credit প্রমাণ করা |
| **Reports** | Summary আর export |
| **Notifications** | Delivery Center: sandbox notification receipt আর retry |
| **Settlements** | Sandbox payout: reserve, capture, release |
| **Quantum Payments** | Testnet tUSD8 invoice আর checkout link |
| **Request simulator** | লাইভ প্রমাণ যে একই key দিয়ে retry করলে দুইবার পোস্ট হয় না |
| **Account security** | নিজের session আর two-factor সেটিং |
| **Users & access**, **Roles & permissions**, **Audit log**, **Event outbox** | Administration (admin অংশ) |

উপরের **workspace currency** selector (JPY, USD, EUR) টাকার পেজগুলোকে এক currency-তে সীমিত রাখে। PayCore কখনো ভিন্ন currency-র অঙ্ক যোগ করে না।

### ৩. Role অনুযায়ী রোজকার কাজ

#### Administrator

- **টিমে কাউকে আনা:** Users & access → *Invite user* → নাম, email আর role দিন। তিনি password ঠিক করার email পাবেন। একই তালিকা থেকে invitation আবার পাঠানো বা বাতিল করা যায়।
- **Role বদলানো, deactivate বা delete করা:** user-এর row খুলুন। PayCore শেষ administrator-কে সরাতে দেয় না, আর আপনি নিজেকে deactivate করতে পারবেন না।
- **"কে এটা করল?" জানতে:** Audit log-এ actor, target, আগের ও পরের role আর IP থাকে। Entry edit বা delete করা যায় না।

পূর্বের Lead operator ও Staff engineer-এর দায়িত্ব এখন Administrator-এরই অংশ:

- **Account খোলা:** Accounts → *New account* → number, name, type (ASSET, LIABILITY, REVENUE, …) আর currency দিন। Settlement চালু করতে প্রতি currency-তে একটা LIABILITY account-কে **Payout clearing** আর একটা ASSET account-কে **Operating bank** হিসেবে চিহ্নিত করুন ([ledger.md](ledger.md))।
- **ভুল শোধরানো:** Transfer studio-তে journal-টা reverse করুন। Reversal একটা নতুন, উল্টো journal; আসলটা দেখা যেতেই থাকে।
- **Beneficiary-কে payout (sandbox):** Settlements → *Reserve* করলে customer wallet থেকে টাকা clearing-এ যায়। *Capture* করলে payout operating bank-এ সম্পন্ন হয়; *Release* করলে টাকা wallet-এ ফেরত যায়।
- **Bank statement মেলানো:** Reconciliation → CSV template download → upload → statement খুলুন → *Match & review* ([reconciliation.md](reconciliation.md))।
- **Event delivery নজরে রাখা:** Event outbox-এ pending, delivered আর failed event, সাথে retry metrics দেখা যায়। *Replay* দিলে failed event আবার queue-তে যায়, আর প্রতিটা replay audit-এ রেকর্ড হয়।
- **Idempotency দেখানো:** Request simulator একই key দিয়ে একসাথে ৫টা request পাঠায়। ফল হওয়া উচিত: একটা commit, আর চারটা stored response-এর replay। একই key দিয়ে বদলানো payload পাঠালে `422` দিয়ে reject হয়।

#### Accountant

- **টাকার হিসাব দেখা:** Accounts-এ balance, আর Ledger explorer-এ তার পেছনের প্রতিটা journal।
- **Transfer পোস্ট করা:** Transfer studio → account, amount আর description বেছে submit করুন। Form শুধু balanced, একই currency-র transfer নেয়। Connection কেটে গিয়ে আবার submit করলেও idempotency key-র কারণে পোস্ট হবে **একবারই**।

#### Compliance auditor

- **হিসাব মিলছে কি না দেখা:** Invariant audit নির্বাচিত currency-র zero-sum check চালায়, আর snapshot কতটা সাম্প্রতিক তা দেখায়।
- **Review ও export:** summary-র জন্য Reports, user ও access-এর ইতিহাসের জন্য Audit log, আর যেকোনো journal-এর বিস্তারিত দেখতে Ledger explorer। এই role দিয়ে কোনো data বদলানো যায় না।

### ৪. PayCore Quantum: testnet-এ tUSD8 পেমেন্ট

PayCore Quantum দিয়ে merchant **QDay Aevum testnet**-এ **tUSD8** (একটা test stablecoin) নিতে পারেন। PayCore কখনো key বা টাকা নিজের কাছে রাখে না। Payer নিজের wallet থেকে সরাসরি merchant-এর address-এ টাকা পাঠান, আর PayCore শুধু সেটা যাচাই করে।

> **শুধু TESTNET · tUSD8-এর কোনো আসল মূল্য নেই।** কখনো mainnet wallet বা আসল টাকা ব্যবহার করবেন না।

**শুরুর আগে (একবার):**

1. একজন administrator `api/.env`-এ Quantum চালু করবেন (`QUANTUM_ENABLED=true` আর বাকি `QUANTUM_*` মান; দেখুন [README](../README.md#paycore-quantum-testnet)), তারপর API restart করবেন।
2. MetaMask-এর মতো একটা browser wallet install করুন, আর QDay Aevum network যোগ করুন ([official guide](https://wiki.qday.info/guide/start/add-network))।
3. বর্তমান [network ও faucet verification notes](../PAYCORE_QUANTUM_QDAY_VERIFICATION.md) অনুসরণ করে gas ও tUSD8 test fund নিন। Wallet-এ কত test token আছে দেখে ছোট demo invoice বেছে নিন; faucet-এর পরিমাণ বা availability স্থায়ী ধরে নেবেন না।

**Merchant হিসেবে:**

1. **Quantum Payments** খুলুন। প্রথমবার একটা display name আর যে wallet address-এ পেমেন্ট **পাবেন** সেটা দিন।
2. *New invoice* চাপুন: amount (যেমন `0.5`), description, আর কতক্ষণ খোলা থাকবে (৫ মিনিট থেকে ২৪ ঘণ্টা)।
3. **Checkout link** কপি করে payer-কে পাঠান।
4. Invoice-এর অবস্থা দেখুন: **Pending** থেকে **Paid** হবে, সাথে explorer-এ transaction-এর লিংক থাকবে। Unpaid invoice cancel করা যায়।

**Payer হিসেবে** (PayCore account লাগবে না):

1. Checkout link খুলুন। কাকে কত টাকা, কোন token-এ, কোন সময়ের মধ্যে দিতে হবে, সব দেখাবে।
2. *Connect wallet* চাপুন। Wallet ভুল network-এ থাকলে *Switch network* চাপুন।
3. *Pay* চাপুন, তারপর wallet-এ transfer approve করুন।
4. কয়েক সেকেন্ড অপেক্ষা করুন। PayCore নিজে blockchain-এ পেমেন্ট যাচাই করার পরেই পেজে **Paid** দেখাবে। Wallet "sent" বললেই সেটা যথেষ্ট নয়।

**কিছু ভুল হলে:**

| যা দেখছেন | মানে | যা করবেন |
| --- | --- | --- |
| *Wrong network* | Wallet QDay Aevum-এ নেই | *Switch network* চাপুন |
| *Rejected in wallet* | আপনি request বাতিল করেছেন | প্রস্তুত হলে আবার *Pay* চাপুন |
| *Needs review* | পেমেন্ট এসেছে কিন্তু হুবহু মেলেনি: amount আলাদা, অন্য wallet থেকে পাঠানো, বা সময় পেরিয়ে গেছে | **আবার টাকা পাঠাবেন না।** Explorer লিংকসহ merchant-এর সাথে যোগাযোগ করুন |
| *Expired* বা *Cancelled* | Invoice বন্ধ হয়ে গেছে | Merchant-এর কাছে নতুন link চান |

Invoice paid হয় শুধু **হুবহু** মিললে: সঠিক token, receiver, payer wallet আর amount, সময়সীমার মধ্যে। অন্য যেকোনো ক্ষেত্রে পেমেন্ট চুপচাপ মেনে নেওয়া বা হারিয়ে যেতে দেওয়ার বদলে মানুষের review-র জন্য রাখা হয়।

## যে নিয়মগুলো আপনাকে সুরক্ষা দেয়

| নিয়ম | কেন দরকার |
| --- | --- |
| Transfer balanced আর এক currency-তে হতে হবে | ভুল করে টাকা তৈরি হয় না, হারায়ও না |
| পোস্ট করা journal edit বা delete করা যায় না; reversal দিয়ে শোধরাতে হয় | যে history audit করছেন, সেটাই আসলে যা ঘটেছে |
| একই key দিয়ে বারবার request দিলে পোস্ট হয় একবার | Network retry বা double-click-এ কখনো double-charge হয় না |
| যথেষ্ট টাকা না থাকলে account negative হয় না | Overdraft পরে ঠিক করা হয় না, শুরুতেই আটকানো হয় |
| শেষ administrator-কে সরানো যায় না | টিম নিজেই নিজেকে lock out করতে পারে না |
| Admin-দের two-factor authentication লাগে | শুধু password চুরি হলেই যথেষ্ট নয় |
| প্রতিটা permission API যাচাই করে | বাটন লুকানো security boundary নয় |
| Quantum পেমেন্ট server নিজে on-chain যাচাই করে | Browser থেকে আসা ভুয়া "আমি পেমেন্ট করেছি"-র কোনো দাম নেই |

## সমস্যা ও সমাধান (Troubleshooting)

| সমস্যা | যা চেষ্টা করবেন |
| --- | --- |
| Compose-এ `set MFA_ENCRYPTION_KEY in .env` দেখাচ্ছে | [Setup-এর ধাপ ২](#প্রথমবারের-setup-checklist)-এ ফিরুন: root `.env`-এর key খালি কি না দেখুন। Existing API key বদলাবেন না |
| Seed চালানোর নতুন password দিয়ে পুরনো user sign-in করতে পারছেন না | Seeder existing user skip করলে আগের password থাকে। আগের password বা **Forgot password** ব্যবহার করুন |
| পেজে *access denied* দেখাচ্ছে | আপনার role-এ ওই permission নেই। Admin-কে বলুন ([rbac.md](rbac.md)) |
| Invitation বা reset email আসছে না | Development-এ Mailpit খুলুন (`http://localhost:8025`)। নইলে SMTP সেটিং দেখুন ([email_outbox.md](email_outbox.md)) |
| Settlements-এ *Reserve* fail করছে | ওই currency-তে **Payout clearing** আর **Operating bank** account লাগবে ([ledger.md](ledger.md)) |
| Quantum Payments-এ *unavailable* দেখাচ্ছে | `QUANTUM_ENABLED` বন্ধ, অথবা startup-এ API chain বা token যাচাই করতে পারেনি। API log-এ `quantum_unavailable` খুঁজুন |
| Paid invoice কয়েক মিনিট ধরে *Pending* | Testnet RPC ধীর হতে পারে। PayCore নিজেই retry করতে থাকে; দ্বিতীয়বার টাকা পাঠাবেন **না** |
| Overview/Accounts-এ sample data নেই | আগে currency **JPY** করুন। এরপরও খালি হলে `api/` থেকে `go run ./cmd/seed-ledger` চালান; এটি USD/EUR sample তৈরি করে না |
| Transfer-এ `insufficient_funds` | Source/debit account-এর balance ও currency দেখুন, amount কমান; প্রথম demo-তে funded `2001` wallet ব্যবহার করুন |

## আরও জানতে

- Database-এর table আর সম্পর্ক (ERD): [erd.md](erd.md)
- Ledger-এর guarantee কীভাবে কাজ করে: [ledger.md](ledger.md)
- Role আর permission-এর পুরো তালিকা: [rbac.md](rbac.md)
- Bank statement reconciliation: [reconciliation.md](reconciliation.md)
- Notification আর outbox: [notifications.md](notifications.md), [system_admin.md](system_admin.md)
- PayCore Quantum: [architecture](quantum/ARCHITECTURE.md), [API](quantum/API.md), [security](quantum/SECURITY.md), [demo script](quantum/DEMO.md)
- Code-এ অবদান রাখতে: [coding_rules.md](coding_rules.md), [testing.md](testing.md)
