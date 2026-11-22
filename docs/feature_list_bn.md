# PayCore — সম্পূর্ণ Feature List

এই একটি ফাইলে PayCore-এর সব planned feature ও implementation checklist রাখা হয়েছে। `[x]` মানে code/scaffold আছে; `[ ]` মানে তৈরি বা সম্পূর্ণ যাচাই বাকি। Scaffold থাকা মানে business feature সম্পূর্ণ হওয়া নয়।

লক্ষ্য: **Ledger Core, Reporting & Audit, Notification এবং Settlement**—চারটি business microservice এবং একটি Next.js dashboard। বর্তমানে শুধু API ও web scaffold আছে।

## ১. বর্তমানে তৈরি আছে

- [x] Go + Gin backend scaffold (`api/`)।
- [x] `GET /` endpoint: API-এর নাম ও `running` status দেখায়; database readiness যাচাই করে না।
- [x] Gin request logger ও panic recovery middleware।
- [x] Environment variable / `.env` থেকে backend configuration পড়া।
- [x] Backend graceful shutdown handling।
- [x] Next.js + TypeScript + Tailwind frontend scaffold (`web/`)।
- [x] `run.ps1` দিয়ে API ও web একসঙ্গে চালানোর ব্যবস্থা।
- [x] Launcher-এ Go/Node/npm check এবং Next.js dependency অসম্পূর্ণ হলে npm install করার ব্যবস্থা।
- [x] Launcher-এ ব্যস্ত port এড়িয়ে পরের available port নির্বাচন এবং API/web URL দেখানো।
- [x] Docker-এ PayCore-এর নিজস্ব PostgreSQL database, persistent volume এবং API connection pool।
- [x] `GET /health/live` ও PostgreSQL ping-সহ `GET /health/ready`।
- [x] `run.ps1 -ListRoutes` / `-Routes` দিয়ে server বা database চালু না করে registered API ও web page list দেখা।

Database চালানো ও connection settings: [Local PostgreSQL setup](database_setup.md)। Ledger schema/migrations ও business repository queries এখনো তৈরি হয়নি।

## ২. Ledger Core Service — Account Management

Account এখানে ledger/chart-of-accounts record; login user account নয়।

- [ ] নতুন ledger account তৈরি: `POST /api/v1/accounts`।
- [ ] Account detail ও authoritative derived balance দেখা: `GET /api/v1/accounts/:id`।
- [ ] Account postings cursor pagination-সহ দেখা: `GET /api/v1/accounts/:id/postings`।
- [ ] Account types: `ASSET`, `LIABILITY`, `EQUITY`, `REVENUE`, `EXPENSE`।
- [ ] Unique account number, currency এবং active/inactive status সংরক্ষণ ও validation।
- [ ] Inactive, unauthorized বা অনুপস্থিত account দিয়ে financial posting প্রত্যাখ্যান।

## ৩. Ledger Core Service — Transfer ও Double-Entry Ledger

Account, Transfer, Ledger ও Reversal একই service এবং একই database transaction boundary-তে থাকবে।

- [ ] Double-entry transfer: `POST /api/v1/transfers`।
- [ ] প্রতি journal-এ অন্তত দুটি positive posting এবং মোট debit = মোট credit নিশ্চিত করা।
- [ ] এক journal-এ transfer, platform fee ও tax-এর মতো multiple posting legs সমর্থন।
- [ ] Journal-এর সব account ও posting একই currency-এর কি না যাচাই করা।
- [ ] Append-only journal/postings; পুরোনো financial record update/delete প্রতিরোধ।
- [ ] Ledger থেকে balance হিসাব করা; verified snapshot ব্যবহার করলে তার consistency বজায় রাখা।
- [ ] Account-specific overdraft rule ও insufficient-funds validation।
- [ ] সব affected account ID অনুযায়ী একই ক্রমে lock করে concurrent overspending ও deadlock risk কমানো।
- [ ] Deadlock/serialization failure-এ bounded whole-transaction retry।
- [x] Idempotency response, journal, postings ও outbox event একই ACID transaction-এ commit/rollback।

## ৪. Ledger Core Service — Reversal

- [ ] Compensating reversal: `POST /api/v1/transfers/:id/reverse`।
- [ ] Original journal-এর প্রতিটি posting-এর direction উল্টে সমপরিমাণ নতুন posting তৈরি।
- [ ] Reversal journal থেকে original journal-এর reference সংরক্ষণ।
- [ ] একই original journal-এর একাধিক full reversal আটকানো এবং reversal eligibility যাচাই।
- [ ] Original record না বদলে linked reversal থেকে reversal status নির্ধারণ।

প্রথম scope-এ full reversal থাকবে; partial reversal অন্তর্ভুক্ত নয়।

## ৫. Ledger Core Service — Idempotency ও Money Precision

- [ ] Financial request-এ বাধ্যতামূলক `Idempotency-Key`।
- [ ] Authenticated client ও key অনুযায়ী PostgreSQL unique constraint।
- [ ] একই key ও একই request retry হলে stored status/response ফেরত দেওয়া; নতুন posting নয়।
- [ ] একই key দিয়ে পরিবর্তিত request পাঠালে `422` response।
- [ ] In-flight duplicate-এর bounded wait শেষে `409` ও `Retry-After`।
- [ ] Key অনুপস্থিত হলে `400`; request hash-এ method, target route ও validated body অন্তর্ভুক্ত করা।
- [ ] Optional Redis response cache; Redis unavailable হলেও PostgreSQL দিয়ে correctness বজায় রাখা।
- [ ] Idempotency retention/reuse policy নির্ধারণ।
- [ ] JPY, USD ও EUR-এর integer minor-unit amount; currency অনুযায়ী scale validation।
- [ ] JSON amount decimal string হিসেবে পাঠানো, floating point এড়ানো এবং overflow check।

Multi-currency মানে একাধিক currency-এর account/journal রাখা। একই journal-এ currency মেশানো যাবে না; FX transfer যোগ করতে হলে separately balanced journals ও clearing rules আলাদাভাবে implement করতে হবে।

## ৬. Ledger Core Service — Audit ও Reliable Events

- [x] Authoritative zero-sum audit: `GET /api/v1/audit/zero-sum?currency=JPY`।
- [x] Consistent database snapshot-এ per-journal ও per-currency debit/credit validation।
- [x] Core-owned transactional outbox ও publisher acknowledgment-এর পরে published marker।
- [x] Outbox relay retry/backoff ও failed-event replay।
- [x] Versioned events: `ledger.journal_posted.v1`, `ledger.journal_reversed.v1`।
- [x] Outbox lag/event health: `GET /api/v1/outbox/metrics`।
- [ ] Broker outage-এ durable backlog, capacity monitoring ও recovery ব্যবস্থা।

## ৭. Reporting & Audit Service

- [ ] Core events থেকে নিজস্ব database-এ reporting projection তৈরি।
- [ ] Dashboard analytics ও currency অনুযায়ী financial summary।
- [ ] Account ID, currency, date range ও transaction type দিয়ে search/filter।
- [ ] Original transaction ও reversal-এর linked event timeline।
- [ ] Account history ও নির্ধারিত ordering contract অনুযায়ী running-balance projection।
- [ ] Statement/export তৈরি এবং export job status দেখা।
- [ ] Report-এর last-processed time / `as_of` ও consumer lag দেখানো।
- [ ] Duplicate/out-of-order events সামলানো এবং historical projection rebuild।

Reporting data eventually consistent। Transfer authorization, overdraft decision ও authoritative zero-sum audit Ledger Core-এই হবে।

## ৮. Notification Service

- [ ] Transfer ও settlement event অনুযায়ী email/webhook notification।
- [ ] Subscription, notification preference ও template management।
- [ ] Durable delivery job, stable delivery ID ও delivery-attempt history।
- [ ] Provider failure-এ bounded retry/backoff ও failed delivery replay।
- [ ] Authorized delivery status দেখা।
- [ ] Duplicate event থেকে duplicate local delivery job তৈরি প্রতিরোধ।

Provider deduplication না দিলে ambiguous timeout-এর পরে duplicate external delivery সম্ভব। Notification failure committed transfer rollback করবে না।

## ৯. Settlement Service — Sandbox Phase

- [ ] Caller idempotency-সহ payout request তৈরি ও status দেখা।
- [ ] Settlement batch ও persisted payout state machine/saga।
- [ ] Core-এ idempotent reserve/capture/release command এবং তার posting rules তৈরি।
- [ ] Sandbox bank/payment provider adapter।
- [ ] Stable provider reference/idempotency key দিয়ে payout submit।
- [ ] Verified callback গ্রহণ ও duplicate/out-of-order callback handling।
- [ ] Success-এ capture, definitive failure-এ release, timeout-এ unknown/reconciliation state।
- [ ] Provider statement-এর সঙ্গে reconciliation ও প্রয়োজন হলে manual review।
- [ ] Crash-এর পরে persisted workflow থেকে recovery ও append-only financial compensation।
- [ ] `settlement.completed.v1` / `settlement.failed.v1` outbox events।

Core reserve/capture/release তৈরি হওয়ার আগে payout চালু হবে না। সব wallet transfer bank payout নয়। বাস্তব bank integration প্রথম scope-এর বাইরে।

## ১০. Web Dashboard

**UI preview তৈরি আছে:** `/`, `/accounts`, `/transfers`, `/ledger`, `/audit`, `/reports`, `/notifications`, `/settlements`, `/simulator`। Shared sample data দিয়ে account/ledger filtering, posting details, transfer validation/preview, sample audit, CSV export এবং local idempotency illustration কাজ করে। Authentication ও business API integration এখনো হয়নি; নিচের checkbox-গুলো সম্পূর্ণ integrated feature-এর জন্য।

- [x] Authoritative zero-sum status, currency ও last successful audit time দেখানো।
- [ ] Account detail, posting history ও ledger explorer।
- [ ] Multiple legs দিয়ে visual transfer builder।
- [ ] Debit/credit অসমান হলে submit বন্ধ করা; backend-এও একই validation।
- [ ] Pending transfer দেখানো; acknowledgment-এর পরে confirmed balance refresh।
- [ ] Active idempotency key দেখানো এবং একই logical request-এর retry-এ key বজায় রাখা।
- [ ] TanStack Query দিয়ে caching, polling ও mutation-এর পরে query invalidation।
- [ ] Reporting analytics, statement/export এবং reversal timeline।
- [ ] Loading, error, unavailable এবং stale-report states স্পষ্টভাবে দেখানো।
- [ ] API base URL ও CORS নির্বাচিত local API port অনুযায়ী configure করা।
- [ ] Responsive, high-contrast dashboard UI।

## ১১. Authentication ও Service Security

**Backend তৈরি আছে:** `User` model, users/session migration, bcrypt password verification, one-use image CAPTCHA, `/auth/me`, database session logout এবং interactive user-creation command। বিস্তারিত: [Email login](email_login.md)। Web login/profile এখনো demo state ব্যবহার করে; backend auth-এর সঙ্গে যুক্ত হয়নি।

**UI preview:** `/profile`-এ demo name/email edit ও local save, এবং `/login`-এ demo workspace entry আছে। Logout saved demo profile reset করে local session বন্ধ করে; এটি server authentication বা token invalidation নয়।

- [ ] নির্বাচিত external OIDC provider দিয়ে authentication integration।
- [ ] প্রতিটি business endpoint-এ resource/account-level authorization।
- [x] Audit, outbox metrics, replay ও administrator action-এর restricted permissions।
- [ ] Service-to-service token-এর audience ও permission validation।
- [ ] Sensitive information বাদ দিয়ে minimal event payload ও নিরাপদ logging।
- [ ] প্রয়োজনীয় security/administrator action-এর durable audit record।

Custom Auth service বর্তমান চার-service roadmap-এর বাধ্যতামূলক অংশ নয়।

## ১২. Microservice Infrastructure ও Operations

### File ও attachment management

- [x] সব file এবং user photo-এর metadata একটি `attachments` table-এ রাখা।
- [x] Local `api/uploads/` folder-এ random key দিয়ে file রাখা; প্রতি file সর্বোচ্চ 10 MiB; image WebP-তে রূপান্তর ও সর্বোচ্চ 3 MiB।
- [x] Authenticated upload, metadata/list, private download ও soft-delete API।
- [x] `users.photo_attachment_id` দিয়ে নিজের image attachment photo হিসেবে link/clear করা।
- [x] পুরোনো photo URL attachment metadata-তে migrate করা এবং ownership/database constraints যাচাই।
- [ ] Web file-upload/gallery ও profile-photo upload controls।
- [ ] Physical file purge, orphan reconciliation ও storage quota।

Endpoint ও storage ব্যবহারের বিস্তারিত: [Unified attachments](attachments.md)।

- [ ] প্রতিটি service-এর নিজস্ব database, role/credentials ও migrations।
- [ ] Cross-service direct table access বন্ধ রেখে REST/event contract ব্যবহার।
- [ ] Versioned OpenAPI ও event schemas।
- [ ] Kafka অথবা RabbitMQ থেকে একটি broker নির্বাচন ও setup।
- [ ] Consumer inbox deduplication এবং side effect একই local transaction-এ লেখা।
- [ ] Dead-letter queue, permission-checked replay ও event retention/rebuild policy।
- [ ] Independent service image, configuration, CI checks ও deployment।
- [ ] Docker Compose-এ infrastructure এবং optional service profiles।
- [ ] Liveness ও dependency-aware readiness checks।
- [ ] Correlation/trace IDs, latency, outbox age, consumer lag ও failure metrics।

`run.ps1` বর্তমানে API + web এবং default local configuration-এ PostgreSQL চালায়। Gateway optional; Redis, broker, database ও relay আলাদা business microservice নয়।

## ১৩. Demo ও Verification

- [x] একই idempotency key দিয়ে পাঁচটি parallel request: একটি financial commit এবং matching replay।
- [x] একই key দিয়ে tampered payload পাঠিয়ে `422` দেখানো।
- [ ] Isolated demo environment-এ fault injection; production-এ ওই control বন্ধ রাখা।
- [x] PostgreSQL integration tests: zero-sum, append-only, currency ও positive amount constraints।
- [ ] Concurrency tests: overspending, duplicate posting ও simultaneous full reversal প্রতিরোধ।
- [ ] Redis outage, rollback এবং broker acknowledgment-এর পরে relay crash পরীক্ষা।
- [ ] Consumer বন্ধ থাকা অবস্থায় transfer commit এবং recovery-এ backlog processing পরীক্ষা।
- [ ] Settlement ambiguous timeout, duplicate callback ও compensation পরীক্ষা।
- [x] k6/wrk load test-এ throughput, latency, deadlock ও retry পরিমাপ।
- [x] Realistic JPY seed data ও repeatable demonstration setup।

## ১৪. কাজের অগ্রাধিকার

1. **Account API:** PostgreSQL setup/migration → account models/repositories/services/controllers → `POST /api/v1/accounts` → account detail/history।
2. **Ledger Core:** money validation → double-entry posting → locking/overdraft → idempotency → reversal → zero-sum audit।
3. **Core events ও dashboard:** outbox/broker → transfer studio → ledger explorer → replay demo।
4. **Reporting + Notification:** independent databases → event consumers → inbox/retry/replay → frontend integration।
5. **Settlement:** Core reservation contract → sandbox provider → payout workflow → reconciliation।

Technical specification: [ProjectFeature.md](../ProjectFeature.md)। Service boundaries ও event flow: [Microservices Architecture](microservices_architecture_bn.md)। Feature সম্পূর্ণ করে যাচাই হলে সংশ্লিষ্ট checkbox update করতে হবে।
