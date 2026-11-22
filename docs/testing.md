# Tests

Public Go tests are centralized under `api/tests/`, grouped by the layer they exercise:

```text
api/tests/
├── audit/
├── database/
│   └── seeders/
├── helpers/
├── http/
│   ├── controllers/
│   ├── middleware/
│   ├── requests/
│   ├── responses/
│   └── routing/
├── models/
├── repositories/
├── server/
└── storage/

web/tests/                 frontend tests
```

Go tests that access private functions, fields or embedded migration files remain alongside their package. Do not export production internals merely to relocate a test. Keep helpers used only by a test suite in that suite's directory, such as `api/tests/repositories/testdb_test.go`.

From the repository root, run all tests and required static checks:

```powershell
.\test.ps1
```

Run one application with `-Suite api` or `-Suite web`. If PowerShell script execution is restricted, use `powershell -NoProfile -ExecutionPolicy Bypass -File .\test.ps1` for this invocation.

The runner checks the API with `go vet ./...`, `go test ./...`, and `golangci-lint run` when installed. It checks the frontend with `npm run check` and `npm test`. It stops on failures and restores the working directory.

Individual suites can also run directly:

```powershell
# API: all tests, including package-local private unit tests
cd api
go test ./...

# API: only centralized public tests
go test ./tests/...

# Frontend, starting from the repository root
cd web
npm test
```

Database integration tests require `DATABASE_TEST_URL`; otherwise they explicitly skip. With the local PostgreSQL container running, set it before invoking the runner:

```powershell
$env:DATABASE_TEST_URL = 'postgres://paycore:paycore_local_dev@127.0.0.1:5434/paycore?sslmode=disable'
.\test.ps1
```

Repository, controller and seeder integration suites use isolated schemas and clean them up after testing. Migration tests remain in `api/database/migrations` because they inspect embedded SQL and private migration helpers.

## Concurrency load tests

The k6 suite drives concurrent idempotent JPY transfers against a running API and verifies the ledger invariant before and after the run. It is opt-in and is not part of `.\test.ps1`. It uses a seeded accountant for transfers and a separate auditor for the zero-sum report, so the admin MFA flow is not involved. Each VU submits a transfer twice with the same `Idempotency-Key` and checks that both responses return the same journal ID; counters track duplicate postings, overdrafts, `ledger_busy` responses, replay matches and invariant failures.

Start the API with seeded users and ledger accounts, then run:

```powershell
$env:PAYCORE_LOAD_PASSWORD = '<seeded password>'
.\api\load\run-load-test.ps1 -BaseUrl 'http://127.0.0.1:8080' -Vus 20 -Duration '30s'
```

The wrapper uses a local `k6` binary when present, otherwise it runs `grafana/k6:latest` through Docker. It prompts for the password when `PAYCORE_LOAD_PASSWORD` is unset. Override `-Email`, `-AuditorEmail`, `-Currency`, `-DebitAccount`, `-CreditAccount`, `-Amount`, `-MaxBusy`, `-MaxP95Ms` and `-MaxFailureRate` for other scenarios. Default accounts are `1001` (debit) and `2003` (credit) from `seed-ledger`; use small amounts so the debit account is not drained during long runs.

## Opt-in local PostgreSQL tests

Run `powershell -NoProfile -ExecutionPolicy Bypass -File .\test.ps1 -Suite api -Integration` with the local database in `compose.yaml` running. This uses its public localhost development connection unless `DATABASE_TEST_URL` is already set. The runner restores that environment variable afterward and serializes Go packages to avoid competing migration advisory locks. All repository tests use isolated schemas that are cleaned up at completion.
