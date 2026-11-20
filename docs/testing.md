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
│   └── responses/
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
