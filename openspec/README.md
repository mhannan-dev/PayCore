# Planning a PayCore change

Use `config.yaml` for shared context and artifact rules. Keep feature-specific decisions in `changes/<change-name>/`, not in the shared configuration. `specs/` describes delivered behavior after synchronization; a change's delta specs describe planned behavior until implemented and synchronized.

Before planning, read `AGENTS.md`, `docs/coding_rules.md`, affected directory instructions, and the relevant source, migrations, DTOs and tests. For Quantum, also read the assignment and its applicable ownership/contract sections. Link to existing contracts instead of maintaining competing copies. Resolve conflicting documents explicitly using current user instructions and the documented precedence.

## Artifact responsibilities

| Artifact | Required content before dependent implementation |
| --- | --- |
| `proposal.md` | Problem, observable outcome, scope/non-goals, affected capabilities, compatibility, assumptions and unresolved decisions |
| `specs/<capability>/spec.md` | Requirements and named scenarios covering successful, boundary and rejected behavior |
| `design.md` | Decisions, edge cases, database schema, API contract, validation, migration/compatibility and verification |
| `tasks.md` | Readiness check first, then dependency-ordered implementation tasks with their own tests/docs and verification |

For areas that are unaffected, write **Not applicable** and explain why. A UI-only change does not need a new table or endpoint. Reuse the existing schema's headings and scenario syntax; add the sections below to the design where needed.

## Edge cases

Document each applicable case with a link to its spec scenario:

| Case / scenario | Preconditions | Input or action | Expected result / error | Persistence and side effects |
| --- | --- | --- | --- | --- |
| `<named boundary or failure>` | `<actor, ownership, current state, time>` | `<exact trigger>` | `<status, stable code, observable outcome>` | `<writes/events, or invariant preserved>` |

Consider missing vs null vs empty input; malformed and unknown values; lower/upper bounds and overflow; unauthorized access and cross-owner access; duplicate submissions; retry after timeout; concurrent state changes; expiry boundaries; unavailable dependencies; and partial completion. Explain excluded categories. For affected Quantum settlement, include wrong chain/token/payer/recipient/amount, failed or absent receipts, confirmation thresholds, duplicate claims and reorgs.

Do not invent numeric limits or time windows to fill the table. Derive them from the existing contract or record a deliberate product decision with its rationale.

## Database schema

| Table / column | SQL type | Null / default | Keys and constraints | Index / ownership scope |
| --- | --- | --- | --- | --- |
| `<name>` | `<exact type>` | `<explicit semantics>` | `<PK, FK, UNIQUE, CHECK>` | `<query or invariant served>` |

Describe relations, transaction boundaries and race handling. Identify the next migration, compatibility with existing rows/readers/writers, backfill steps and deployment order. Explain rollback or forward recovery, including data that cannot safely be reversed. Never edit an applied migration to disguise a schema change.

## API contract

For each changed endpoint, specify:

- Method/path, authentication, permission and ownership checks; required headers.
- Request and response fields: JSON name, type, required/optional, null/omitted meaning, units, bounds and normalization.
- Success status and a valid request/response example. For bodyless endpoints, document that explicitly.
- Rejection statuses/codes using PayCore's existing `{error,message,details}` envelope, with representative invalid examples.
- Applicable idempotency, retries, state transitions, pagination, rate limits and compatibility behavior.
- Exact Go DTO and TypeScript mirror paths. Read those files before proposing changes; preserve existing clients deliberately.

For money, document currency/token, minor/base units, precision, rounding if relevant and overflow bounds. Examples must use valid digit strings consistent with the contract, without secrets or real checkout capabilities.

## Validation

| Field / business rule | Normalize | Accepted values / bounds | Enforcement layer | HTTP status / error code | Spec scenario / verification |
| --- | --- | --- | --- | --- | --- |
| `<field or invariant>` | `<none or exact operation>` | `<explicit rule>` | `<controller, service, DB; frontend if relevant>` | `<existing or proposed stable mapping>` | `<named scenario and check>` |

Controllers validate request shape; services enforce business meaning; database constraints protect persisted invariants. Frontend validation gives feedback but cannot be the only enforcement. Specify the mapping of typed domain errors to the centralized HTTP error handler. Rejected requests must have explicit persistence and event behavior.

## Readiness and verification

The first tracked task checks that the four artifacts agree and all applicable sections above are concrete. Resolve questions that change behavior, schema, API, permissions, money handling or compatibility before dependent implementation. Use repository evidence and existing authorization for routine decisions; ask the user when a consequential decision cannot otherwise be resolved. Record deferrable questions with their impact and why they do not block the work.

Use `- [ ] 1.1 ...` tasks with observable verification, keeping relevant tests and docs in the group that implements the behavior. Run the checks required by `AGENTS.md`, plus affected contract/integration checks. Record commands, source revision, results and coverage limits. A dated passing result is evidence for that revision, not proof of future changes.

Useful commands from the repository root:

```powershell
openspec list --json
openspec new change <change-name>
openspec instructions proposal --change <change-name>
openspec instructions specs --change <change-name>
openspec instructions design --change <change-name>
openspec instructions tasks --change <change-name>
openspec status --change <change-name>
openspec validate <change-name> --strict
```

Artifact rules and operation guidance are instructions for agents and reviewers. `openspec validate` checks supported structural/spec rules; it does not prove that all edge cases are covered, API examples match the implementation, or a migration is safe. The readiness review and the relevant tests provide that evidence. This configuration does not retroactively create baseline specs for existing features.
