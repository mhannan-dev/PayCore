<div align="center">

# PayCore

**An open-source double-entry payment and ledger platform: a Go API and a Next.js operations console, built around correctness first.**

[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](api/go.mod)
[![Gin](https://img.shields.io/badge/Gin-1.12-008ECF)](https://gin-gonic.com/)
[![Next.js](https://img.shields.io/badge/Next.js-16-000000?logo=nextdotjs&logoColor=white)](web/package.json)
[![React](https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black)](web/package.json)
[![TypeScript](https://img.shields.io/badge/TypeScript-strict-3178C6?logo=typescript&logoColor=white)](web/tsconfig.json)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-4169E1?logo=postgresql&logoColor=white)](compose.yaml)
[![Tailwind CSS](https://img.shields.io/badge/Tailwind_CSS-4-06B6D4?logo=tailwindcss&logoColor=white)](web/package.json)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Status](https://img.shields.io/badge/status-alpha-orange)](#project-status)

</div>

PayCore is a learning-grade but production-minded fintech core: an append-only double-entry ledger with strict idempotency and deterministic locking, plus the identity, access control and operator tooling around it. Money is always an integer in minor units, every rule that matters is enforced by the database, and both codebases are held to [strict type-safety and layered-architecture rules](docs/coding_rules.md).

![PayCore operations console: overview](docs/previews/web-overview.png)

## Contents

- [Project status](#project-status)
- [Features](#features)
- [Architecture](#architecture)
- [Getting started](#getting-started)
- [Configuration](#configuration)
- [Usage](#usage)
- [Development](#development)
- [Documentation](#documentation)
- [Contributing](#contributing)
- [License](#license)

## Project status

PayCore is in **alpha**. The platform layer (authentication, access control, user management, email, file storage) is implemented and tested against PostgreSQL. The **ledger core** (accounts, transfers, reversals, zero-sum audit) is implemented and drives the Accounts, Transfer studio, Ledger explorer and Invariant audit pages; Overview, Reports, Notifications, Settlements and Simulator still show *sample data*. See the [roadmap](ProjectFeature.md#6-implementation--portfolio-roadmap) and the [feature checklist](docs/feature_list_bn.md).

**Do not use PayCore to move real money.**

## Features

**Implemented**

- **Email and password sign-in** with a server-generated math CAPTCHA, bcrypt hashing, and revocable bearer sessions (only token hashes are stored).
- **Role-based access control:** 5 roles and 12 fine-grained permissions, enforced by the API on every request and reflected in the UI (navigation, page guards, controls).
- **User management:** paginated, searchable user list; invite users by email; edit roles; deactivate, reactivate or delete users, with "last administrator" and "not yourself" safeguards.
- **Audit log:** append-only record of sign-ins and account administration (actor, target, before/after roles, IP); the database refuses edits and deletes, and admin changes are recorded in the same transaction.
- **Email invitations and forgot password:** single-use, hashed, expiring links; background delivery with retries and a per-invitation delivery record; enumeration-safe reset requests.
- **SMTP mailer:** enforced STARTTLS or implicit TLS, AUTH PLAIN or LOGIN (works with Microsoft 365), local Mailpit inbox for development.
- **Profiles and attachments:** editable names, profile photos, validated uploads stored outside the web root.
- **Operations console:** responsive Next.js dashboard with an animated preloader, toast notifications and sticky navigation.
- **Engineering hygiene:** versioned SQL migrations, standard JSON error envelope, hot reload, and Postgres integration tests.

- **Double-entry ledger:** chart of accounts with derived balances, balanced transfers, full reversals and a live zero-sum audit. The database itself rejects unbalanced journals, edits, deletes and late postings; accounts are locked in a fixed order with overdraft checks; every write is idempotent per `Idempotency-Key` and records an outbox event in the same transaction ([docs/ledger.md](docs/ledger.md)).

**Planned** (see [ProjectFeature.md](ProjectFeature.md))

- Outbox relay to a message broker, optional Redis idempotency cache.
- Reporting, Notification and Settlement services.

## Architecture

```text
┌────────────────────────┐   HTTPS / JSON   ┌──────────────────────────┐        ┌──────────────┐
│  web/  Next.js 16      │ ───────────────▶ │  api/  Go 1.27 + Gin     │ ─────▶ │ PostgreSQL 16│
│  React 19, TypeScript  │   Bearer token   │  handler → service →     │  pgx   │ (migrations) │
│  Tailwind 4, yup       │ ◀─────────────── │  repository              │        └──────────────┘
└────────────────────────┘                  │  mail outbox ──▶ SMTP    │ ─────▶  Mailpit (dev) /
                                            └──────────────────────────┘         SMTP provider
```

| Layer | Technology |
| --- | --- |
| API | Go 1.27, Gin, pgx/v5, bcrypt, embedded SQL migrations |
| Web | Next.js 16 (App Router, Turbopack), React 19, TypeScript (strict), Tailwind CSS 4, yup, react-toastify |
| Data | PostgreSQL 16 (Docker Compose for local development) |
| Email | SMTP (any provider) and Mailpit for local development |
| Tooling | Air (Go hot reload), ESLint with type-aware rules, golangci-lint, PowerShell launcher (`run.ps1`) |

```text
.
├── api/                 Go API
│   ├── cmd/             server, seed-users, create-user, grant-system-admin, mail-check
│   ├── internal/        config, handler, service, repository, model, mailer, storage, …
│   ├── migrations/      versioned SQL migrations (embedded and applied at startup)
│   └── pkg/response/    standard JSON responses and error envelope
├── web/                 Next.js console (src/app routes, src/components, src/lib)
├── docs/                feature, setup and design documentation
├── compose.yaml         PostgreSQL and Mailpit for local development
└── run.ps1              starts the database, API and web together
```

## Getting started

### Prerequisites

- [Go](https://go.dev/dl/) 1.27 or later
- [Node.js](https://nodejs.org/) 20.9 or later, with npm
- [Docker Desktop](https://www.docker.com/products/docker-desktop/) (for PostgreSQL and Mailpit)
- Windows PowerShell 5.1+ or PowerShell 7 for `run.ps1`; on macOS or Linux, use the manual steps below

### Quick start (Windows)

```powershell
git clone https://github.com/<your-account>/paycore.git
cd paycore

# 1. Configure (copy the examples; the defaults work for local development)
Copy-Item api/.env.example api/.env
Copy-Item web/.env.example web/.env.local

# 2. Start everything: PostgreSQL, Mailpit, the API (with hot reload) and the web console
.\run.ps1
```

`run.ps1` starts the Docker services, picks free ports (the API defaults to 8080, the web to 3000), passes the right API URL to the web app, and stops both servers on Ctrl+C. Run `.\run.ps1 -ListRoutes` to print every API endpoint and web page without starting anything.

### Manual setup (any OS)

```bash
# Database and local inbox
docker compose up -d --wait postgres mailpit

# API (applies migrations on startup)
cd api
cp .env.example .env
go run ./cmd/server            # or `air` for hot reload

# Web (in a second terminal)
cd web
cp .env.example .env.local
npm install
npm run dev
```

### Create users

Seed the four development accounts (admin, operator, auditor, engineer). The command prompts for a password and never prints it:

```powershell
cd api
go run ./cmd/seed-users
```

Or create a single user: `go run ./cmd/create-user -email you@example.com -first-name You -roles ADMIN`. Admins can then invite others from **Users & access** in the console. See [docs/user_seeder.md](docs/user_seeder.md) and [docs/system_admin.md](docs/system_admin.md).

Open http://localhost:3000 and sign in. Invitation and password-reset emails sent during development appear in Mailpit at http://localhost:8025.

## Configuration

Copy [`api/.env.example`](api/.env.example) to `api/.env` and [`web/.env.example`](web/.env.example) to `web/.env.local`. Both real files are git-ignored. **Never commit real credentials.**

### API (`api/.env`)

| Variable | Default | Description |
| --- | --- | --- |
| `PORT` | `8080` | HTTP port. `run.ps1` overrides it with the port it selects. |
| `ENV` | `development` | `development` or `production`. Development allows any loopback CORS origin and logs emails when no SMTP server is set. |
| `DATABASE_URL` | local Compose database | PostgreSQL connection string. |
| `WEB_ORIGINS` | `http://localhost:3000,http://127.0.0.1:3000` | Comma-separated origins allowed by CORS. When unset in development, any loopback port is also allowed. Set it to your console's URL in production. |
| `APP_URL` | `http://localhost:3000` | Public URL of the web console, used in emailed links. |
| `UPLOAD_DIR` | `uploads` | Directory for uploaded files (outside the web root). |
| `REDIS_URL` | `localhost:6379` | Reserved for the planned idempotency cache; not required yet. |
| `SMTP_HOST` | *(empty)* | SMTP server. Empty: development writes emails to the API log; `run.ps1` uses Mailpit. |
| `SMTP_PORT` | `587` / `465` / `1025` | Defaults from the encryption mode. |
| `SMTP_ENCRYPTION` | `starttls` | `starttls` (required, never opportunistic), `tls` (port 465), or `none` (loopback only). |
| `SMTP_USERNAME`, `SMTP_PASSWORD` | *(empty)* | SMTP credentials; sent only over TLS. |
| `SMTP_FROM` | the username, or `PayCore <no-reply@paycore.local>` | Sender address. |
| `MAIL_*` | | Laravel-style alternative: `MAIL_HOST`, `MAIL_PORT`, `MAIL_USERNAME`, `MAIL_PASSWORD`, `MAIL_ENCRYPTION`, `MAIL_FROM_ADDRESS`, `MAIL_FROM_NAME`. Used only when `SMTP_HOST` is unset; the two sets are never mixed. |

Check the email settings without starting the API: `cd api; go run ./cmd/mail-check` (add `-to you@example.com` to send a test message).

### Web (`web/.env.local`)

| Variable | Default | Description |
| --- | --- | --- |
| `NEXT_PUBLIC_API_URL` | `http://localhost:8080` | API base URL. `run.ps1` sets it automatically. |
| `NEXT_PUBLIC_DEMO_PASSWORD` | *(empty)* | Development only: fills the seed password when a demo identity is picked on the login page. Visible in the browser bundle, so never set a real password. Ignored in production builds. |

### Tooling and tests (shell only)

| Variable | Used by |
| --- | --- |
| `PAYCORE_SEED_PASSWORD` | `seed-users`, to avoid the interactive prompt in scripts |
| `PAYCORE_API_URL` | `run.ps1`, to point the web app at a different API |
| `DATABASE_TEST_URL` | Go integration tests (PostgreSQL); skipped when unset |
| `SMTP_TEST_HOST`, `SMTP_TEST_PORT` | Go SMTP delivery tests (e.g. Mailpit); skipped when unset |

## Usage

### API examples

Every response is JSON; errors use one envelope: `{"error": "code", "message": "…", "details": …}` ([docs/api_responses.md](docs/api_responses.md)).

```bash
# 1. Get a CAPTCHA challenge (an SVG image of a small sum)
curl -s http://localhost:8080/api/v1/auth/captcha

# 2. Sign in with the answer
curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@example.com","password":"<password>","captcha_id":"<id>","captcha_answer":"12"}'
# → {"token":"<64 hex>","token_type":"Bearer","expires_in":86400,"user":{…,"permissions":[…]}}

# 3. Call an authenticated endpoint
curl -s http://localhost:8080/api/v1/users?limit=20 -H 'Authorization: Bearer <token>'

# 4. Invite a user (requires users:write); they receive a set-password email
curl -s -X POST http://localhost:8080/api/v1/users -H 'Authorization: Bearer <token>' \
  -H 'Content-Type: application/json' \
  -d '{"first_name":"Mina","email":"mina@example.com","roles":["OPERATOR"]}'
```

Full endpoint references: [authentication](docs/email_login.md), [RBAC, users, invitations and password reset](docs/rbac.md), [attachments](docs/attachments.md).

### Screenshots

| Overview | Users & access | Sign-in |
| --- | --- | --- |
| ![Overview](docs/previews/web-overview.png) | *Screenshot coming soon* | *Screenshot coming soon* |

> **Live demo:** *coming soon.*

## Development

```powershell
# Web: type check + type-aware lint
cd web; npm run check

# API: vet, unit tests, and integration tests against the local database
cd api
go vet ./...
$env:DATABASE_TEST_URL = 'postgres://paycore:paycore_local_dev@127.0.0.1:5434/paycore?sslmode=disable'
go test ./...
golangci-lint run          # config in api/.golangci.yml
```

All changes follow the [coding rules](docs/coding_rules.md): type safety, thin handlers, dependency injection with consumer-side interfaces, and centralized error handling. The local database password above is a development-only default from `compose.yaml`; never reuse it anywhere else.

## Documentation

| Topic | Document |
| --- | --- |
| Product specification and roadmap | [ProjectFeature.md](ProjectFeature.md) |
| Coding rules (type safety) | [docs/coding_rules.md](docs/coding_rules.md) |
| Database setup | [docs/database_setup.md](docs/database_setup.md) |
| Ledger core (accounts, transfers, reversals, idempotency) | [docs/ledger.md](docs/ledger.md) |
| Authentication | [docs/email_login.md](docs/email_login.md) |
| RBAC, users, invitations, password reset, email | [docs/rbac.md](docs/rbac.md) |
| Attachments | [docs/attachments.md](docs/attachments.md) |
| API responses and errors | [docs/api_responses.md](docs/api_responses.md) |
| Seeding and administrators | [docs/user_seeder.md](docs/user_seeder.md), [docs/system_admin.md](docs/system_admin.md) |
| Microservices architecture (Bengali) | [docs/microservices_architecture_bn.md](docs/microservices_architecture_bn.md) |

## Contributing

Contributions are welcome. Please read [CONTRIBUTING.md](CONTRIBUTING.md) before opening an issue or pull request. Report security vulnerabilities privately as described there, not in public issues.

## License

PayCore is released under the [MIT License](LICENSE).
