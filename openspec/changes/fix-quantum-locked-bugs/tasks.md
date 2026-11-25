# Tasks

## 1. Readiness
- [x] 1.1 Verify F1-F4 scope, current source, migration number, API examples, schema and validation matrix agree; verify strict OpenSpec validation before dependent implementation.

## 2. F2 and F4 verification safety
- [ ] 2.1 Add regression tests for isolated/sustained receipt absence, positive conflict, lower head and clock boundaries; observe baseline failures.
- [ ] 2.2 Implement persisted suspicion/finalization and configured skew on both payment paths; verify focused service/config/repository tests and update timing/reorg docs.

## 3. F3 admission and proxy safety
- [ ] 3.1 Add capacity/IPv6/proxy regression tests, observe failures, implement bounded eviction/sweeping and validated explicit proxy configuration; verify middleware/config/server tests and document proxy deployment settings.

## 4. F1 API recovery
- [ ] 4.1 Add failing retention/same-hash/recovery regression tests; implement migration, hash serialization, recovery service/controller/route and retained abandoned attempts; verify PostgreSQL tests including duplicate claims and admission limits and update A3/API/types.

## 5. F1 checkout recovery
- [ ] 5.1 Add failing storage/resume/retry regression tests; implement immediate persistence, reload recovery, manual recovery and Pay suppression, with storage-failure handling; verify frontend tests, types and lint and update recovery documentation.

## 6. Integration verification
- [ ] 6.1 Run required API vet/all tests with PostgreSQL/lint and frontend checks/all tests; validate OpenSpec strictly and record actual results and limitations in the change verification record.
