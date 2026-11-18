# Local PostgreSQL

PayCore uses PostgreSQL 16 with a pgx connection pool. The local Compose project is `paycore`; it has its own `postgres_data` volume and binds only to `127.0.0.1:5434` so other projects on port 5432 remain untouched.

## Start

```powershell
.\run.ps1
```

With the default configuration, the launcher runs `docker compose -f compose.yaml up -d --wait postgres` before starting the API/web. Docker Desktop must be running. The API verifies a database connection before listening and closes its pool during normal shutdown. Database startup failure prevents the API from starting.

Development-only connection settings (also in `api/.env.example`):

```dotenv
DATABASE_URL=postgres://paycore:paycore_local_dev@127.0.0.1:5434/paycore?sslmode=disable
```

These credentials are for loopback-only development. For another database, set `DATABASE_URL` in the API process environment or `api/.env`; environment variables take precedence. When an explicit URL exists, the launcher skips its local database startup automatically. `-SkipDatabase` also skips startup but the API still requires a working connection.

```powershell
docker compose -f compose.yaml up -d --wait postgres
.\run.ps1 -SkipDatabase
```

## Check

The API startup log includes `PostgreSQL connection verified; connection pool ready`. Use the API port printed by the launcher:

```powershell
Invoke-RestMethod http://localhost:8080/health/live
Invoke-RestMethod http://localhost:8080/health/ready
.\run.ps1 -ListRoutes
```

`GET /health/live` checks HTTP liveness. `GET /health/ready` pings PostgreSQL with a two-second deadline and returns `200` with `database: connected`, or `503` without exposing connection details. Route listing does not start a database or require connectivity; its URLs use the requested ports, not running-service discovery.

## Data lifecycle

Stopping `run.ps1` stops its API/web jobs and leaves PostgreSQL running. To stop PostgreSQL without deleting its persistent volume:

```powershell
docker compose -f compose.yaml stop postgres
```

The API also applies the embedded user/session migration during startup. [Email login](email_login.md) documents user provisioning, CAPTCHA, password verification, and revocable sessions. Account/ledger migrations remain pending; sample dashboard records and the browser's demo session are not connected to these backend tables.

## Tests

From `api/`:

```powershell
go test ./...
$env:DATABASE_TEST_URL = 'postgres://paycore:paycore_local_dev@127.0.0.1:5434/paycore?sslmode=disable'
go test ./internal/database -run TestPostgresIntegration -v
```

The integration test connects through the pool, writes and reads a temporary table inside a rolled-back transaction, and leaves no application table behind.
