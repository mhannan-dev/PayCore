# Statement import and preview

Open **Reconciliation** in the web sidebar (`/reconciliation`). Download the CSV
template, choose an ASSET or LIABILITY account, and upload the statement. The
account's currency must match every row. Historic statements may target an
inactive account because this workflow does not post money.

CSV files must be UTF-8, at most 2 MiB and contain 1–2,000 rows:

```csv
booked_date,reference,currency,amount,direction,description
2026-11-20,BANK-001,USD,1250.00,CREDIT,Customer receipt
2026-11-20,BANK-002,USD,15.50,DEBIT,Bank fee
```

Header order may vary; all six columns are required and extra columns are
rejected. UTF-8 BOM, CRLF, quoted commas and quoted multiline descriptions work.
Dates use YYYY-MM-DD, from 1900 onward. Reference is required (128 characters
maximum); description may be empty (500 maximum). Amount is a positive decimal
in major units, with no signs, thousands separators, exponents or excess decimal
places. JPY has zero decimal places; USD/EUR have two. The API converts amounts
to int64 minor units using string arithmetic and returns them as JSON strings.
Totals use arbitrary-precision integers to avoid overflow across rows.

**DEBIT/CREDIT follows the bank statement's perspective.** Import does not infer
ledger posting directions, create accounts, post journals or change balances.
Open an imported statement and use **Match & review** to configure the direction
mapping, inspect automatic suggestions and confirm or reject candidates.

Preview shows invalid rows. Fix the CSV and upload it again; import is only
enabled when every row validates. Repeated identical normalized rows in one file
are errors. The normalized content hash ignores filename, header/row order,
whitespace around fields, currency/direction case and equivalent decimal
formatting. Description and reference remain case-sensitive. An identical set
of rows cannot be imported twice by the same user for the same account, even
under concurrent requests. Overlapping but non-identical files are not yet
detected as duplicates.

Files use the existing single attachments table and private local storage.
Preview and import verify attachment ownership, byte size and SHA-256. Import
reparses the source and checks `expected_hash` from preview; client-supplied rows
and totals are never trusted. Metadata, rows and the audit event commit in one
transaction. The source attachment and account are rechecked under locks.

Imported statement rows are financial history: they survive deletion of the
uploader or source attachment. The immutable owner UUID scopes list/detail
access; administrators do not bypass this ownership rule. The source attachment
reference becomes null after hard deletion. Soft deletion does not change
already imported rows, but prevents a new import from that attachment.

## API

All URLs start with `/api/v1` and require a bearer session:

| Method / path | Required permissions | Body / query |
| --- | --- | --- |
| POST `/attachments` | `attachments:write` | Existing multipart upload, field `file` |
| POST `/reconciliation/preview` | `reconciliation:import`, `attachments:read`, `ledger:read` | `attachment_id`, `account_id` |
| POST `/reconciliation/imports` | Same as preview | Same plus `expected_hash` from preview |
| GET `/reconciliation/imports` | `reconciliation:read`, `ledger:read` | `limit` 1–100 (default 20), `offset` |
| GET `/reconciliation/imports/:id` | Same as list | `limit` 1–100 (default 50), `offset` for rows |

ADMIN, OPERATOR and LEAD_OPERATOR receive import/read permissions. The
COMPLIANCE_AUDITOR receives read permission. STAFF_ENGINEER receives neither.
The web form also requires attachment upload/read and ledger read permissions.
List returns an array; detail includes import metadata and the requested rows.
Duplicate imports return HTTP 409; refreshing preview provides the existing
import ID. Invalid CSV structure returns 400; invalid row values appear in a
successful preview and block import. Reads are owner-scoped 404s for other users.

Migration `019_statement_imports.sql` adds the tables, constraints, audit action
and default grants. The API applies pending migrations during startup. Restart
the API with `./run.ps1` after updating; refresh the session/page to load the new
permission grants.

## Exact matching and exception review

Migration `020_statement_matching.sql` adds matching, rejected-candidate and
review-note tables. Matching is one statement row to one existing ledger posting.
It never creates transfers, reversals, adjustment postings or balance changes.
Matching applies to the uploader's own imports; the ADMIN role does not bypass
ownership. ADMIN, OPERATOR and LEAD_OPERATOR receive `reconciliation:match`;
COMPLIANCE_AUDITOR can view their own imported statements and reviews only.

Choose the direction mapping explicitly for each import:

| Mapping | Statement CREDIT | Statement DEBIT |
| --- | --- | --- |
| OPPOSITE | Ledger DEBIT | Ledger CREDIT |
| SAME | Ledger CREDIT | Ledger DEBIT |

Confirm the bank's convention; account type alone does not determine it. No
suggestions appear before configuration. Automatic suggestions require exact
account, currency, minor-unit amount, case-sensitive reference, mapped direction,
and booking date. The ledger posting's journal timestamp is converted to **UTC**
before comparing its date. No fuzzy reference matching, date tolerance, grouped
matches or amount tolerance is applied. A journal with several identical account
legs can produce ambiguous candidates. Original and reversal postings are each
eligible when they independently satisfy the exact criteria.

The report classifies each row as `PENDING`, `UNMATCHED`, `SUGGESTED`, `AMBIGUOUS`,
`MATCHED` or `REJECTED`. Candidate responses contain up to five postings per row
and the full candidate count. Suggestions remain unconfirmed until a reviewer
chooses **Confirm match**. Confirming one of several candidates resolves that
ambiguity for the row. Rejecting a candidate requires a 3–500 character reason
and removes that candidate from subsequent suggestions for this row. Other
candidates remain available. If none remain and at least one was rejected, the
status is `REJECTED`.

Unmatched rows can have follow-up notes without creating synthetic matches.
Notes are editable; audit history retains previous note events. **Reset row
review** requires a reason, releases any confirmed posting, clears this row's
candidate rejections, and preserves its note. The released posting becomes
available for reconciliation again. To change the direction mapping, first
reset every confirmed match in the import. Changing mapping clears old candidate
rejections for that import and preserves notes.

A posting can be confirmed only once **across all imports and uploaders**. The
database's unique posting constraint prevents concurrent double reconciliation.
Reports hide postings already claimed by other rows without exposing those
imports or owners. Reports are computed live in a consistent database snapshot,
so newly posted ledger entries may become suggestions after refreshing. Statuses
and counts shown in the UI cover the current page, not an aggregate report of
the entire import or ledger-only discrepancies.

Configuration and every review use the report's integer `version` as
`expected_version`. A locked import row serializes changes; stale requests return
HTTP 409. Candidate validity is rechecked inside the write transaction. The
decision, new version and audit event commit together; audit failure rolls them
all back. Refresh matching after conflicts before making another decision.

| Method / path (under `/api/v1`) | Required permissions | Body / query |
| --- | --- | --- |
| GET `/reconciliation/imports/:id/matches` | `reconciliation:read`, `ledger:read` | `limit` 1–100 (default 50), `offset` |
| POST `/reconciliation/imports/:id/matching` | Above plus `reconciliation:match` | `direction_mode`, `expected_version` |
| POST `/reconciliation/imports/:id/rows/:row/match` | Above plus `reconciliation:match` | `action`, `posting_id` (nullable), `reason`, `expected_version` |

`:row` is the original CSV row number returned in the matching report, including
the header and physical line offsets for quoted multiline records. Actions are
`CONFIRM`, `REJECT`, `RESET` and `NOTE`. CONFIRM/REJECT require a posting UUID;
RESET/NOTE require a null or omitted posting ID. All except CONFIRM require a
reason of 3–500 characters. Unknown fields, malformed IDs, invalid enum values
and oversized requests are rejected. Successful writes return HTTP 204; fetch
the matching report again for the updated version and candidates.
