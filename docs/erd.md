# Database ERD (Entity Relationship Diagram)

PayCore-এর PostgreSQL schema-র ছবি: কোন table-এ কী থাকে, আর table-গুলো কীভাবে একে অপরের সাথে যুক্ত। Diagram-গুলো **migration 026 পর্যন্ত** চালানো dev database থেকে নেওয়া হয়েছে (columns, foreign keys, unique keys, 2026-11-22)। Migration 026 কোনো table বা column বদলায়নি; শুধু role-এর তালিকা আর `users.roles`-এর নিয়ম বদলেছে ([নিচে দেখুন](#২-identity--access-user-role-permission))।

> Schema বদলানোর একমাত্র উপায় নতুন migration ([api/database/migrations/](../api/database/migrations/))। Migration যোগ করলে এই পেজও আপডেট করুন।

## App ব্যবহার করতে কোনটার পরে কী

**ERD-র রেখা table-এর সম্পর্ক বোঝায়; এগুলো command চালানোর ক্রম নয়।** Table নিজে বানানো বা row হাতে insert করা দিয়ে app ব্যবহার শুরু করবেন না। Migrations schema তৈরি করে; UI ও service প্রতিটি workflow-এর data লেখে। Command-সহ walkthrough আছে [User guide-এর setup checklist](user_guide.md#প্রথমবারের-setup-checklist)-এ।

```mermaid
flowchart TD
    A["১. Local config ও MFA key"] --> B["২. PostgreSQL ও Mailpit চালু"]
    B --> C["৩. Seed users; migrations প্রয়োগ"]
    C --> D["৪. সাধারণ ledger demo চাইলে seed-ledger"]
    D --> E["৫. API ও web চালু; sign-in ও প্রযোজ্য MFA"]
    C --> E
    E --> L["Ledger: currency ও accounts দেখুন"]
    L --> T["Balanced transfer পোস্ট"]
    T --> J["Journal ও postings দেখুন"]
    J --> V["Balance ও zero-sum audit মিলিয়ে নিন"]
    J --> R["ঐচ্ছিক: statement import ও matching"]
    L --> S["ঐচ্ছিক: sandbox payout reserve"]
    S --> SC["Capture অথবা release; linked journal যাচাই"]
    E --> Q["Quantum config enabled ও permissions আছে?"]
    Q --> M["Merchant profile ও receiver"]
    M --> I["Invoice তৈরি; checkout link শেয়ার"]
    I --> P["Payer: wallet connect ও transfer approve"]
    P --> W["Backend worker receipt ও confirmations যাচাই"]
    W --> PA["Exact valid transfer: claim ও PAID"]
    W --> RE["Mismatch: review; আবার transfer নয়"]
```

Ledger এবং Quantum **আলাদা পথ**। Ledger demo চালাতে wallet লাগে না; Quantum merchant setup করতে sample JPY ledger লাগে না। Smart contract deployment বা agent mandate এই বর্তমান direct-token checkout-এর prerequisite নয়; সেগুলোর deployment ও app integration আলাদা কাজ।

### User action থেকে database record

| আগে কী লাগবে | App-এ কী করবেন | যে data তৈরি বা ব্যবহার হবে | সফলতা কীভাবে দেখবেন | এরপর |
| --- | --- | --- | --- | --- |
| Migrations ও sample users | Email/password/CAPTCHA, প্রযোজ্য MFA দিয়ে sign-in | `users`, `roles`, `role_permissions`; `user_mfa`/challenges প্রযোজ্য হলে; শেষে `auth_sessions` | Overview ও role অনুযায়ী sidebar | Accounts অথবা Quantum |
| Sign-in, সঠিক permission | Team member invite | `users` → `user_invitations` ও `email_outbox`; admin action `audit_events`-এ | Email link ব্যবহার করে teammate নিজের password সেট করেন | Teammate sign-in |
| একই currency-র funded accounts | Balanced transfer submit | `idempotency_records`, `journal_entries`, `ledger_postings`, `outbox_events` এক transaction-এ | Posted journal; Accounts balance পরিবর্তন | Ledger explorer → Invariant audit |
| Original journal ও reverse permission | Transfer reverse | নতুন `journal_entries` ও বিপরীত `ledger_postings`; original-এর সঙ্গে reference | Original history থাকে, reversal journal আলাদা | Balance ও audit আবার যাচাই |
| Funded wallet, clearing ও bank system-role accounts | Sandbox reserve → capture অথবা release | `settlements`, প্রতিটি ধাপের journal/postings ও outbox | `RESERVED` → `CAPTURED` অথবা `RELEASED` | Linked journal যাচাই |
| Account ও আগের ledger posting | CSV preview → import → matching configure → confirm | `statement_imports` → `statement_rows` → `statement_matches`; প্রয়োজনে rejection/note | Row `MATCHED`; balance অপরিবর্তিত | Unmatched/ambiguous row review |
| Committed ledger/settlement event | Delivery Center ও Event outbox দেখুন | `outbox_events` → `notification_deliveries` → `notification_attempts` | Sandbox receipt/attempt status | Failure হলে অনুমতি অনুযায়ী retry/replay |
| Quantum enabled, signed-in merchant | Merchant profile save | `users` → `quantum_merchants` | Display name ও receiver দেখায় | Invoice তৈরি |
| Merchant profile | New invoice | `quantum_invoices`, invoice-creation idempotency record | `PENDING` ও checkout link | Payer-কে link দিন |
| Payable invoice ও testnet wallet | Pay → wallet approve → hash submit | `quantum_payment_attempts`: `CREATED` → `BROADCAST`/`VERIFYING` | Attempt, hash ও confirmations; এখনও paid নয় | Worker verification-এর অপেক্ষা |
| Exact transfer ও যথেষ্ট confirmations | Worker finalizes; user status দেখেন | Unique `quantum_transfer_claims`, confirmed attempt, `PAID` invoice ও reserved outbox event এক transaction-এ | Checkout ও merchant detail-এ `PAID` | Explorer/hash মিলিয়ে নিন |

একই transaction-এর row-গুলো **একসঙ্গে commit** হয়। Table-এর তালিকা দেখে journal আগে commit করে postings পরে লেখার মতো আলাদা operation করবেন না। Checkout hash জমা দেওয়া এবং final verification অবশ্য আলাদা ধাপ; প্রথমটি `202` ফিরিয়ে দেয়, দ্বিতীয়টি worker চালায়। Quantum events থেকে বর্তমান ledger posting বা merchant webhook delivery হয় না।

### State দেখে পরের কাজ

| State/ফল | পরের কাজ |
| --- | --- |
| Transfer posted | Journal, balances ও zero-sum audit দেখুন |
| Settlement `RESERVED` | Capture অথবা release, একই payout-এ দুটো নয় |
| Statement `SUGGESTED`/`AMBIGUOUS` | Candidate মিলিয়ে confirm; ভুল candidate reject/note করুন |
| Invoice `PENDING` | মেয়াদ থাকলে payer checkout ব্যবহার করতে পারেন |
| Attempt `BROADCAST`/`VERIFYING` | Confirmations-এর অপেক্ষা; hash alone সফলতার প্রমাণ নয় |
| Invoice `PAID` | Paid hash ও explorer মিলিয়ে নিন |
| Attempt `REVIEW_REQUIRED`, অথবা broadcast-এর পরে API error | নতুন transfer নয়; hash নিয়ে merchant-এর সঙ্গে review করুন |
| Invoice `EXPIRED`/`CANCELLED` | নতুন payment শুরু করবেন না; merchant-এর কাছে নতুন invoice চান |
| Attempt `REORGED` | আগের confirmation আর নির্ভরযোগ্য নয়; claim noncanonical হতে পারে, invoice আবার pending/expired দেখাতে পারে। নতুন transfer না পাঠিয়ে merchant review ও worker reconciliation-এর অপেক্ষা করুন |

## কীভাবে পড়বেন (How to read)

- **৩৫টা table**, domain অনুযায়ী ৬ ভাগে ভাগ করা। প্রথমে overview, তারপর প্রতিটা domain-এর বিস্তারিত (সব column সহ)।
- `PK` = primary key, `FK` = foreign key, `UK` = unique। Comment-এ `nullable` মানে column-টা `NULL` হতে পারে; বাকি সব `NOT NULL`।
- **নিরেট রেখা (solid line)** = database-এ আসল foreign key আছে।
- **ডটেড রেখা (dotted line, `..`)** = শুধু logical reference, কোনো FK নেই। বেশিরভাগ ক্ষেত্রে এটা ইচ্ছাকৃত, যেমন audit log user delete হওয়ার পরেও টিকে থাকে।
- Cardinality: `||` = ঠিক একটা, `o|` = শূন্য বা একটা, `o{` = শূন্য বা অনেক, `|{` = এক বা অনেক।
- Type: `timestamptz` = `timestamp with time zone`; টাকা সবসময় `bigint` (minor unit), কখনো float নয়।

GitHub আর VS Code (Mermaid extension) এই diagram সরাসরি render করে।

## ১. Overview: সব domain এক নজরে

```mermaid
erDiagram
    users ||--o{ auth_sessions : "signs in"
    users ||--o{ user_roles : has
    roles ||--o{ user_roles : "assigned via"
    roles ||--o{ role_permissions : grants
    permissions ||--o{ role_permissions : "granted via"
    users ||--o| user_mfa : "secures with"
    users ||--o{ attachments : owns
    users ||--o{ user_invitations : "invited as"
    users ||--o{ password_reset_tokens : resets
    users ||--o{ email_outbox : "receives mail"
    accounts ||--o{ ledger_postings : "debited / credited"
    journal_entries ||--|{ ledger_postings : contains
    accounts ||--o{ settlements : "pays out from"
    journal_entries ||--o{ settlements : "posted by"
    outbox_events ||--o{ notification_deliveries : "fans out to"
    accounts ||--o{ statement_imports : "reconciled by"
    statement_imports ||--|{ statement_rows : contains
    statement_rows ||--o| statement_matches : "matched by"
    ledger_postings ||--o| statement_matches : "matches"
    users ||--o| quantum_merchants : "acts as"
    quantum_merchants ||--o{ quantum_invoices : issues
    quantum_invoices ||--o{ quantum_payment_attempts : "paid through"
    quantum_invoices ||--o{ quantum_transfer_claims : "settled by"
```

| Domain | Tables | কাজ |
| --- | --- | --- |
| [Identity & access](#২-identity--access-user-role-permission) | `users`, `roles`, `permissions`, `user_roles`, `role_permissions`, `user_addresses`, `user_contact_numbers`, `attachments` | কে, কোন role, কী permission; profile আর file |
| [Auth security](#৩-auth-security-session-mfa-invitation-email) | `auth_sessions`, `user_mfa`, `mfa_challenges`, `mfa_recovery_codes`, `mfa_replacements`, `user_invitations`, `password_reset_tokens`, `email_outbox`, `audit_events` | Session, two-factor, invitation/reset link, email delivery, audit trail |
| [Ledger & settlement](#৪-ledger-settlement-outbox) | `accounts`, `journal_entries`, `ledger_postings`, `idempotency_records`, `settlements`, `outbox_events`, `notification_deliveries`, `notification_attempts` | Double-entry হিসাব, idempotency, payout, event delivery |
| [Reconciliation](#৫-reconciliation-bank-statement) | `statement_imports`, `statement_rows`, `statement_matches`, `statement_rejections`, `statement_row_notes` | Bank statement import করে ledger-এর সাথে মেলানো |
| [Quantum](#৬-paycore-quantum-testnet-checkout) | `quantum_merchants`, `quantum_invoices`, `quantum_payment_attempts`, `quantum_transfer_claims` | Testnet tUSD8 invoice আর on-chain পেমেন্ট যাচাই |
| System | `schema_migrations` | কোন migration চলেছে (`version`, `applied_at`); অন্য কোনো table-এর সাথে যুক্ত নয় |

## ২. Identity & access (user, role, permission)

```mermaid
erDiagram
    users {
        uuid id PK
        text first_name
        text last_name "nullable"
        text email UK
        text password_hash "nullable; bcrypt; null until invitation accepted"
        text_array roles "source of truth; 1-3 distinct of ADMIN, ACCOUNTANT, COMPLIANCE_AUDITOR; default ACCOUNTANT"
        bool is_active
        uuid photo_attachment_id FK "nullable; must be owned by this user"
        timestamptz created_at
        timestamptz updated_at
    }
    roles {
        uuid id PK
        text code UK "ADMIN, ACCOUNTANT, COMPLIANCE_AUDITOR"
        text name
        text description "nullable"
        timestamptz created_at
        timestamptz updated_at
    }
    permissions {
        uuid id PK
        text code UK "e.g. transfers:create, quantum:write"
        text name
        text description "nullable"
        timestamptz created_at
        timestamptz updated_at
    }
    user_roles {
        uuid user_id PK,FK
        uuid role_id PK,FK
        timestamptz assigned_at
    }
    role_permissions {
        uuid role_id PK,FK
        uuid permission_id PK,FK
        timestamptz assigned_at
    }
    user_addresses {
        uuid id PK
        uuid user_id FK
        text label
        text line1
        text line2 "nullable"
        text city
        text state_region "nullable"
        text postal_code "nullable"
        char country_code
        bool is_primary "at most one primary per user"
        timestamptz created_at
        timestamptz updated_at
    }
    user_contact_numbers {
        uuid id PK
        uuid user_id FK "UK with phone_number"
        text label
        text phone_number
        bool is_primary "at most one primary per user"
        timestamptz verified_at "nullable"
        timestamptz created_at
        timestamptz updated_at
    }
    attachments {
        uuid id PK
        uuid owner_user_id FK
        text file_name
        text content_type
        bigint size_bytes
        text sha256 "nullable"
        text storage_kind
        text storage_key UK "nullable"
        text external_url "nullable"
        timestamptz created_at
        timestamptz deleted_at "nullable; soft delete"
    }

    users ||--o{ user_roles : has
    roles ||--o{ user_roles : "assigned via"
    roles ||--o{ role_permissions : grants
    permissions ||--o{ role_permissions : "granted via"
    users ||--o{ user_addresses : has
    users ||--o{ user_contact_numbers : has
    users ||--o{ attachments : owns
    users }o--o| attachments : "profile photo"
```

- একজন user-এর effective role আসে `users.roles` থেকে। `user_roles` হলো তার mirror; mirror পুরনো হলেও কেউ বাড়তি role পায় না ([rbac.md](rbac.md))।
- **Role এখন ৩টা** (migration [026_accountant_roles.sql](../api/database/migrations/026_accountant_roles.sql)): `ADMIN`, `ACCOUNTANT`, `COMPLIANCE_AUDITOR`। পুরনো role-গুলো মুছে ফেলা হয়েছে, আর তাদের permission নতুন role-এ চলে গেছে:

  | পুরনো role | এখন | Permission |
  | --- | --- | --- |
  | `OPERATOR` | `ACCOUNTANT` | OPERATOR-এর সব grant ACCOUNTANT-এ কপি হয়েছে |
  | `LEAD_OPERATOR` | Explicit review | Pending 026-এর আগে উপযুক্ত role বেছে নিতে হবে; স্বয়ংক্রিয় ADMIN promotion runner আটকায় |
  | `STAFF_ENGINEER` | Explicit review | Pending 026-এর আগে উপযুক্ত role বেছে নিতে হবে; স্বয়ংক্রিয় ADMIN promotion runner আটকায় |

  026 ইতিমধ্যে applied হলে আগের SQL mapping-এর ADMIN assignment অপরিবর্তিত থাকে। সেগুলো review করতে deployment history লাগবে; runner অনুমান করে demote করে না। দেখুন [legacy upgrade review](rbac.md#legacy-role-upgrade-review)।

- `users.roles`-এ `CHECK (valid_user_roles(roles))`: ১ থেকে ৩টা role, কোনো duplicate বা `NULL` নয়, আর শুধু ওই ৩টার মধ্য থেকে। নতুন user-এর ডিফল্ট role `ACCOUNTANT`।
- Profile photo-র FK composite: `(photo_attachment_id, id)` → `attachments(id, owner_user_id)`। তাই অন্যের file কেউ নিজের photo বানাতে পারে না।

## ৩. Auth security (session, MFA, invitation, email)

```mermaid
erDiagram
    users {
        uuid id PK
        text email UK
    }
    auth_sessions {
        bytea token_hash PK "only the hash of the bearer token is stored"
        uuid id UK
        uuid user_id FK
        bool mfa_verified
        timestamptz created_at
        timestamptz expires_at
    }
    user_mfa {
        uuid user_id PK,FK
        bytea encrypted_secret "nullable; TOTP secret, encrypted"
        timestamptz enabled_at "nullable"
        bigint last_counter "blocks TOTP code reuse"
        int failed_attempts
        timestamptz locked_until "nullable"
    }
    mfa_challenges {
        bytea token_hash PK
        uuid user_id FK,UK
        text password_hash_snapshot
        bytea pending_secret "nullable"
        timestamptz expires_at
    }
    mfa_recovery_codes {
        uuid user_id PK,FK
        bytea code_hash PK
    }
    mfa_replacements {
        uuid user_id PK,FK
        uuid session_id FK
        text password_hash_snapshot
        bytea encrypted_secret
        timestamptz expires_at
    }
    user_invitations {
        bytea token_hash PK
        uuid user_id FK
        uuid created_by FK "nullable"
        timestamptz created_at
        timestamptz expires_at
        timestamptz used_at "nullable; single use"
        text email_status
        int email_attempts
        timestamptz email_sent_at "nullable"
        text email_error "nullable"
    }
    password_reset_tokens {
        bytea token_hash PK
        uuid user_id FK
        timestamptz created_at
        timestamptz expires_at
        timestamptz used_at "nullable; single use"
    }
    email_outbox {
        uuid id PK
        text kind "UK with token_hash"
        uuid user_id FK
        bytea token_hash
        text recipient
        text subject
        text text_body
        text html_body
        text status
        int attempts
        timestamptz available_at
        timestamptz expires_at
        uuid lease_id "nullable"
        timestamptz lease_until "nullable"
        text last_error "nullable"
        timestamptz sent_at "nullable"
        timestamptz created_at
        timestamptz updated_at
    }
    audit_events {
        bigint id PK
        timestamptz occurred_at
        text action "CHECK: closed list of actions"
        uuid actor_user_id "nullable; no FK on purpose"
        text actor_email "nullable"
        uuid target_user_id "nullable; no FK on purpose"
        text target_email "nullable"
        jsonb details
        text ip "nullable"
        text user_agent "nullable"
    }

    users ||--o{ auth_sessions : "signs in"
    users ||--o| user_mfa : "secures with"
    users ||--o| mfa_challenges : "pending login"
    user_mfa ||--o{ mfa_recovery_codes : "backup codes"
    user_mfa ||--o| mfa_replacements : "replacing authenticator"
    auth_sessions ||--o{ mfa_replacements : "started in"
    users ||--o{ user_invitations : "invited as"
    users |o--o{ user_invitations : "invited by"
    users ||--o{ password_reset_tokens : resets
    users ||--o{ email_outbox : "receives mail"
    users |o..o{ audit_events : "actor / target"
```

- Token (session, invitation, reset, MFA challenge) কখনো সরাসরি রাখা হয় না, শুধু তার **hash** থাকে। Database চুরি হলেও token কাজে লাগবে না।
- `audit_events` **append-only**: database নিজেই update আর delete আটকায়। User delete হওয়ার পরেও ইতিহাস রাখার জন্য এখানে ইচ্ছা করে FK নেই; email-টাও কপি করে রাখা হয়।

## ৪. Ledger, settlement, outbox

```mermaid
erDiagram
    accounts {
        uuid id PK "UK with currency"
        text account_number UK
        text name
        text type "ASSET, LIABILITY, EQUITY, REVENUE, EXPENSE"
        text currency "JPY, USD, EUR"
        bool overdraft_allowed
        bool is_active
        text system_role "nullable; PAYOUT_CLEARING or OPERATING_BANK; one per currency"
        uuid created_by FK "nullable"
        timestamptz created_at
        timestamptz updated_at
    }
    journal_entries {
        uuid id PK
        text reference_id
        text description
        text currency
        timestamptz posted_at
        uuid reversal_for_id FK,UK "nullable; a journal can be reversed once"
        uuid created_by "nullable; no FK"
        text created_by_email "nullable"
    }
    ledger_postings {
        uuid id PK
        uuid journal_entry_id FK
        uuid account_id FK
        bigint amount "minor units, > 0"
        text direction "DEBIT or CREDIT"
        timestamptz created_at
    }
    idempotency_records {
        text client_id PK "caller user ID; no FK"
        text key PK "Idempotency-Key header"
        text request_hash "sha256 of the request"
        int status_code "nullable until completed"
        jsonb response_body "nullable until completed; replayed on retry"
        timestamptz created_at
    }
    settlements {
        uuid id PK
        text payout_reference UK
        uuid source_account_id FK "customer wallet"
        uuid clearing_account_id FK "PAYOUT_CLEARING"
        uuid destination_account_id FK "nullable; OPERATING_BANK after capture"
        bigint amount "minor units"
        text currency
        text status "RESERVED, CAPTURED, RELEASED"
        text provider_mode "SANDBOX"
        text provider_reference
        text beneficiary_name
        text beneficiary_bank
        text beneficiary_account
        uuid reserve_journal_id FK
        uuid capture_journal_id FK "nullable"
        uuid release_journal_id FK "nullable"
        uuid created_by FK "nullable"
        timestamptz created_at
        timestamptz updated_at
    }
    outbox_events {
        uuid id PK
        text aggregate_type "e.g. JOURNAL, QUANTUM_INVOICE"
        text aggregate_id "no FK; points at the aggregate"
        text event_type
        jsonb payload
        text status "PENDING, PUBLISHED, FAILED"
        int retry_count
        timestamptz available_at
        text lease_id "nullable"
        timestamptz lease_until "nullable"
        text last_error "nullable"
        timestamptz created_at
        timestamptz published_at "nullable"
    }
    notification_deliveries {
        uuid id PK
        uuid event_id FK "UK with channel"
        uuid journal_id FK
        text channel "EMAIL or WEBHOOK"
        text destination
        text status "PENDING, DELIVERED, FAILED"
        int attempts
        text last_error "nullable"
        text receipt "nullable"
        timestamptz delivered_at "nullable"
        timestamptz created_at
    }
    notification_attempts {
        uuid delivery_id PK,FK "UK with attempt"
        text lease_id PK
        int attempt
        text status
        text receipt "nullable"
        text error "nullable"
        timestamptz occurred_at
    }
    users {
        uuid id PK
    }

    journal_entries ||--|{ ledger_postings : "contains (debits = credits)"
    accounts ||--o{ ledger_postings : "debited / credited"
    journal_entries |o--o| journal_entries : "reversed by"
    users |o--o{ accounts : "created by"
    users |o..o{ journal_entries : "posted by"
    users ||..o{ idempotency_records : "client_id"
    accounts ||--o{ settlements : "source / clearing / destination"
    journal_entries ||--o{ settlements : "reserve / capture / release"
    users |o--o{ settlements : "created by"
    journal_entries ||..o{ outbox_events : "event for (aggregate_id)"
    outbox_events ||--o{ notification_deliveries : "fans out to"
    journal_entries ||--o{ notification_deliveries : about
    notification_deliveries ||--o{ notification_attempts : "tried in"
```

- **Balance কোথাও সংরক্ষিত নেই।** Account-এর balance সবসময় তার `ledger_postings` যোগ করে বের করা হয়, তাই balance আর history কখনো আলাদা হতে পারে না।
- `journal_entries` আর `ledger_postings` **append-only**: database unbalanced journal, edit, delete আর দেরিতে আসা posting প্রত্যাখ্যান করে ([ledger.md](ledger.md))।
- একটা settlement তিনটা account আর সর্বোচ্চ তিনটা journal-এর সাথে যুক্ত: reserve (source → clearing), তারপর capture (clearing → bank) অথবা release (clearing → source)।
- টাকা সরানোর প্রতিটা write একই transaction-এ একটা `outbox_events` row লেখে (transactional outbox)। Relay সেটা পরে deliver করে।

## ৫. Reconciliation (bank statement)

```mermaid
erDiagram
    accounts {
        uuid id PK
        text currency
    }
    attachments {
        uuid id PK
    }
    users {
        uuid id PK
    }
    ledger_postings {
        uuid id PK
    }
    statement_imports {
        uuid id PK
        uuid owner_user_id "no FK; UK with account_id, content_hash"
        uuid attachment_id FK "nullable; the uploaded CSV"
        uuid account_id FK "FK with currency to accounts(id, currency)"
        text currency
        text file_name
        text content_hash "duplicate-import guard"
        text source_sha256
        int row_count
        numeric total_debits
        numeric total_credits
        text direction_mode "nullable; how bank DEBIT/CREDIT maps to the ledger"
        int matching_version
        timestamptz created_at
    }
    statement_rows {
        uuid import_id PK,FK
        int row_number PK
        date booked_date
        text reference
        bigint amount "minor units"
        text direction "bank perspective: DEBIT or CREDIT"
        text description
    }
    statement_matches {
        uuid import_id PK,FK
        int row_number PK,FK
        uuid posting_id FK,UK "a posting matches at most one row"
        uuid matched_by "no FK"
        timestamptz matched_at
    }
    statement_rejections {
        uuid import_id PK,FK
        int row_number PK,FK
        uuid posting_id PK,FK
        uuid rejected_by "no FK"
        text reason
        timestamptz rejected_at
    }
    statement_row_notes {
        uuid import_id PK,FK
        int row_number PK,FK
        text note
        uuid updated_by "no FK"
        timestamptz updated_at
    }

    accounts ||--o{ statement_imports : "reconciled by"
    attachments |o--o{ statement_imports : "source file"
    users ||..o{ statement_imports : owns
    statement_imports ||--|{ statement_rows : contains
    statement_rows ||--o| statement_matches : "matched by"
    ledger_postings ||--o| statement_matches : matches
    statement_rows ||--o{ statement_rejections : "rejected candidates"
    ledger_postings ||--o{ statement_rejections : "rejected for"
    statement_rows ||--o| statement_row_notes : "exception note"
```

- Statement import **টাকা সরায় না**। এটা শুধু bank-এর row রাখে আর `ledger_postings`-এর সাথে মেলায় ([reconciliation.md](reconciliation.md))।
- `(account_id, currency)` → `accounts(id, currency)` composite FK নিশ্চিত করে যে statement-এর currency account-এর currency-র সাথে মেলে।
- Row-এর key `(import_id, row_number)`; match, rejection আর note সবাই এই composite key দিয়ে row-এর সাথে যুক্ত।

## ৬. PayCore Quantum (testnet checkout)

```mermaid
erDiagram
    users {
        uuid id PK
    }
    quantum_merchants {
        uuid id PK
        uuid owner_user_id FK,UK "one merchant per user"
        text display_name
        text receiver_address "0x + 40 hex, lower-case, not zero"
        timestamptz created_at
    }
    quantum_invoices {
        uuid id PK
        uuid merchant_id FK
        text checkout_token UK "43-char random; public link"
        text description
        bigint amount_base_units "> 0; tUSD8 has 6 decimals"
        bigint chain_id "mainnet IDs rejected by CHECK"
        text token_address "snapshot at creation"
        text receiver_address "snapshot at creation"
        int token_decimals
        text explorer_url
        text checkout_url
        text status "PENDING, PAID, EXPIRED, CANCELLED"
        timestamptz expires_at "at most 24h after created_at"
        timestamptz paid_at "nullable; set only when PAID"
        text paid_tx_hash "nullable; set only when PAID"
        timestamptz created_at
        timestamptz updated_at
        bigint version
    }
    quantum_payment_attempts {
        uuid id PK
        uuid invoice_id FK
        text payer_address "one open attempt per invoice and payer"
        text tx_hash "nullable until broadcast"
        text status "CREATED, BROADCAST, VERIFYING, CONFIRMED, FAILED, REORGED, REVIEW_REQUIRED"
        text failure_code "nullable; set only for FAILED or REVIEW_REQUIRED"
        bigint confirmations
        bigint observed_block "nullable"
        text observed_hash "nullable"
        timestamptz next_verify_at
        text lease_id "nullable; worker lease"
        timestamptz lease_until "nullable"
        int rpc_failures
        timestamptz created_at
        timestamptz updated_at
    }
    quantum_transfer_claims {
        uuid id PK
        uuid invoice_id FK "one canonical claim per invoice"
        uuid attempt_id FK
        bigint chain_id "UK with tx_hash, log_index"
        text tx_hash
        bigint log_index
        text token_address
        text from_address
        text to_address
        bigint amount_base_units
        bigint block_number
        text block_hash
        bool canonical "false after a reorg"
        timestamptz created_at
    }

    users ||--o| quantum_merchants : "acts as"
    quantum_merchants ||--o{ quantum_invoices : issues
    quantum_invoices ||--o{ quantum_payment_attempts : "paid through"
    quantum_invoices ||--o{ quantum_transfer_claims : "settled by"
    quantum_payment_attempts ||--o{ quantum_transfer_claims : proves
```

- **একই on-chain transfer দুইবার কাজে লাগানো যায় না।** `(chain_id, tx_hash, log_index)` unique, তাই একটা transfer event দিয়ে দুটো invoice শোধ হয় না।
- **একটা invoice-এ একসঙ্গে মাত্র একটা canonical payment থাকে।** Partial unique index `quantum_one_canonical_payment` এটি রক্ষা করে। Reorg-এ claim noncanonical হতে পারে; একই payment পরে canonical হলে invoice reconfirm হতে পারে।
- Invoice তৈরির সময় token, receiver, chain আর decimals **snapshot** করে রাখা হয়। পরে config বদলালেও পুরনো invoice-এর শর্ত বদলায় না।
- Quantum module **accounting ledger থেকে আলাদা**: এখানে কোনো `ledger_postings` লেখা হয় না। শুধু `quantum.invoice.paid` আর `quantum.invoice.reorged` event `outbox_events`-এ যায় ([quantum/ARCHITECTURE.md](quantum/ARCHITECTURE.md))।

## Database-level guarantees (সারাংশ)

| Guarantee | কোথায় প্রয়োগ হয় |
| --- | --- |
| Email unique | `users.email` UK |
| Journal balanced, immutable | `journal_entries`, `ledger_postings` triggers ([014_ledger_core.sql](../api/database/migrations/014_ledger_core.sql)) |
| একটা journal একবারই reverse হয় | `journal_entries.reversal_for_id` UK |
| Retry-তে double posting হয় না | `idempotency_records` PK `(client_id, key)` |
| প্রতি currency-তে একটা clearing আর একটা bank account | `accounts_currency_system_role_key` partial unique index |
| Audit log বদলানো যায় না | `audit_events` triggers |
| Statement দুইবার import হয় না | `statement_imports` UK `(owner_user_id, account_id, content_hash)` |
| একটা posting একটাই statement row-এর সাথে মেলে | `statement_matches.posting_id` UK |
| একটা on-chain transfer একটাই invoice শোধ করে | `quantum_transfer_claims` UK `(chain_id, tx_hash, log_index)` |
| প্রতি invoice-এ একটাই বৈধ পেমেন্ট | `quantum_one_canonical_payment` partial unique index |
