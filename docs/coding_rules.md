# Coding rules

PayCore moves money, so a value that is "probably" a string, a number or present is a bug waiting to happen, and business logic tangled into HTTP code cannot be tested or reused. These rules keep the compiler and linters in a position to catch mistakes before they run, and keep each layer small and replaceable. Each rule names what enforces it; **review** means a reviewer must check it because no tool can.

Run the checks before every commit:

```powershell
cd web; npm run check          # tsc --noEmit + eslint (type-aware)
cd api; go vet ./...; go test ./...; golangci-lint run   # see api/.golangci.yml
```

## Shared rules (Go and TypeScript)

1. **Money is never a float.** Amounts are integers in the currency's minor unit: Go `int64`, PostgreSQL `BIGINT`, TypeScript `bigint` for arithmetic and a digit string in JSON (`"25000"`). Parse with a validator (`validAmount`), never `Number()`/`parseFloat`. *(review)*
2. **Closed sets are typed, not free strings.** Roles, permissions, statuses and directions are a Go `const` block plus a database `CHECK`, and a TypeScript string-literal union (`"pending" | "sent" | …`). Never compare against an ad-hoc string literal that is not part of the union. *(tsc, exhaustive lint)*
3. **Switches over a union cover every case.** Either list every member or keep a `default` that handles the unknown value deliberately. *(eslint `switch-exhaustiveness-check`, golangci `exhaustive`)*
4. **API contracts are mirrored by hand, in one place.** Go JSON structs live in `api/internal/models`; their TypeScript mirrors live in `web/src/lib/auth-api.ts` and `web/src/lib/ledger-api.ts`. Changing a field means changing both in the same change, with the same name (snake_case JSON) and nullability (`*string` ⇔ `string | null`). *(review)*
5. **Absence is explicit.** Use `*T` / `T | null` for "may be missing" and say what missing means in a comment. Do not use zero values (`""`, `0`) as a silent "none" in API responses.

## TypeScript (web/)

Compiler (`web/tsconfig.json`): `strict`, `noUncheckedIndexedAccess`, `noImplicitReturns`, `noImplicitOverride`, `noFallthroughCasesInSwitch`, `allowJs: false`.

6. **No `any`.** Use `unknown` and narrow it. *(eslint `no-explicit-any`)*
7. **Untrusted data is `unknown` until narrowed.** `JSON.parse`, `response.json()`, `localStorage`, `catch (e)` values and URL parameters must be checked (type guard, `instanceof`, or a yup schema) before their fields are used. The single exception is the typed `request<T>()` helper in `lib/auth-api.ts`, which is where the API contract (rule 4) is asserted. *(eslint `no-unsafe-*`)*
8. **No non-null assertions (`x!`).** Handle the `undefined` case: `?.`, `??`, an early return, or a lookup that cannot fail (`balanceOf(id)` instead of `accounts[3]!`). *(eslint `no-non-null-assertion`)*
9. **Array and record lookups may be `undefined`.** `noUncheckedIndexedAccess` makes `arr[i]` and `record[key]` include `undefined`; prefer `find`, `at()` with a fallback, or destructuring with a default (`const [area = ""] = code.split(":")`). Do not index fixtures by position. *(tsc)*
10. **Type assertions are a last resort.** `x as T` only at a proven boundary (rule 7), never on object literals (`{…} as T` hides missing fields; annotate the variable instead). `@ts-ignore`/`@ts-nocheck` are banned; `@ts-expect-error` needs a reason. *(eslint `consistent-type-assertions`, `ban-ts-comment`)*
11. **Promises are awaited or explicitly ignored.** Write `void doThing()` for fire-and-forget, and never pass an async function where a void callback is expected except JSX event handlers. *(eslint `no-floating-promises`, `no-misused-promises`)*
12. **Throw `Error` objects** (`AuthApiError` for API failures), never strings or plain objects. *(eslint `only-throw-error`)*
13. **Type-only imports use `type`.** `import { type ManagedUser } from …` keeps types erased from client bundles. *(eslint `consistent-type-imports`, auto-fixable)*
14. **Environment variables:** read `process.env.NEXT_PUBLIC_*` with dot access (Next.js inlines only that form) and give a typed fallback at the read site.
15. **Forms validate with yup schemas** that mirror the API's limits, and infer form types from the schema (`InferType`), so the type and the validation cannot drift.

## Go (api/)

16. **No unchecked type assertions.** Use `v, ok := x.(T)`. Values shared through `gin.Context` use the typed `contextKey[T]` in `internal/http/controllers/context_keys.go`, never `c.MustGet("…").(T)`. *(golangci `forcetypeassert`)*
17. **Errors are compared with `errors.Is` / `errors.As`,** never `==` or a type assertion, and wrapped with `%w`. Return sentinel errors (`ErrEmailTaken`) rather than `nil, nil`. *(golangci `errorlint`, `nilnil`)*
18. **Request bodies bind into typed structs** with size limits (`http.MaxBytesReader`) and validation (`binding` tags or a `Normalize()` method); never decode into `map[string]any`. *(review)*
19. **Every JSON struct field has a `json` tag;** secrets are `json:"-"` (e.g. `PasswordHash`). *(golangci `musttag`)*
20. **SQL is parameterised** (`$1`), never built with `fmt.Sprintf` from input, and scanned into typed fields; nullable columns scan into pointers. Close rows and check `rows.Err()`. *(golangci `gosec`, `sqlclosecheck`, `rowserrcheck`)*
21. **Database constraints back the types:** every enum column has a `CHECK`, every "required" field is `NOT NULL`, every reference a foreign key. The database is the last line of defence. *(review; migrations)*
22. **Interfaces are declared by the consumer** (e.g. `controllers.RBACStore`, `services.InvitationStore`) and kept small, so fakes in tests are type-checked against exactly what is used. *(review)*

## Architecture (Go API)

The API is layered. Each layer depends only on the one below it, through interfaces the caller declares:

```text
HTTP request
   │
   ▼
server/      HTTP engine, global middleware (CORS, request metadata, recovery, errors)
   │
   ▼
routes/      endpoint mapping, route middleware and permission requirements
   │
   ▼
http/controllers/  transport only: bind + validate input, call a service, write the response
   │  consumer-side interfaces (e.g. controllers.RBACStore)
   ▼
services/     business rules and orchestration; no HTTP, no SQL
   │  consumer-side interfaces (e.g. services.InvitationStore)
   ▼
repositories/  SQL and transactions; returns model types and sentinel errors
   │
   ▼
PostgreSQL
```

23. **Controllers stay thin ("no fat controllers").** A controller does four things only: bind and validate the request, take the caller and request values it needs, call **one** service method, and write the response with `internal/http/responses`. Business rules (permissions beyond the route's, invariants, multi-step workflows, email, audit decisions) belong in a service. A controller that grows branches of business logic or exceeds ~40 lines per endpoint must be split. *(review)*
24. **`*gin.Context` never leaves the transport layer.** Only `internal/server/`, `routes/` and `internal/http/` import Gin. Services and repositories take a `context.Context` and plain typed arguments, so they can be called from tests, CLIs (`cmd/`) and future workers. Request-scoped facts they need (actor, IP) travel in `context.Context` via typed helpers such as `audit.WithActor`, never as `*gin.Context`. *(golangci `depguard`, see api/.golangci.yml)*
25. **Dependencies are injected, never constructed inside.** Every controller and service receives its collaborators through its constructor (`NewXController(svc XService, …)`) as **interfaces declared by the consumer** (rule 22), never concrete types such as `*services.AuthService`. Only the composition root, `cmd/server/main.go`, creates concrete repositories, services and controllers and wires them together. No package-level mutable state and no `init()` wiring. *(review)*
26. **Errors are mapped to HTTP in one place: centralized error handling middleware.** Services and repositories return typed errors (sentinels like `repositories.ErrEmailTaken`, or a domain error carrying a code); they never choose HTTP status codes. Controllers do not `switch` on errors: they call `c.Error(err)` and return, and a single error middleware in `server/` translates the error to the standard envelope (`{"error", "message", "details"}`) using one mapping table, logs unexpected errors with the request ID, and answers 500 without leaking internals. Adding an error means adding one row to that table. *(review)*
27. **Validation happens at the edge, rules in the core.** Controllers validate *shape* (binding tags, size limits, UUID format); services validate *meaning* (e.g. last admin, role changes) and return typed errors. A service must not trust that a controller validated anything business-critical. *(review)*
28. **Tests follow the layers.** Public Go tests live under `api/tests/<area>/`; frontend tests live under `web/tests/`. Go unit tests that access private package internals stay beside their source. Services are tested with in-memory fakes of their consumer-side interfaces; repositories with PostgreSQL in an isolated schema (`isolatedPool`); controllers with `httptest` against fakes of their services. No test needs a running server. Run all tests and required checks with `./test.ps1`; see [testing.md](testing.md). *(review)*

### Folder structure

Each folder has one responsibility; put new code where a reader would look for it.

```text
api/
├── cmd/                         CLI entry points; server wires dependencies
├── routes/                      per-resource endpoint, middleware and permission mapping
├── database/
│   ├── migrations/              embedded versioned SQL; never edit applied migrations
│   └── seeders/                 explicit sample users, RBAC and administrator setup
└── internal/
    ├── http/
    │   ├── controllers/         request validation, service calls and responses
    │   ├── middleware/          errors, request IDs, rate limiting and logging
    │   ├── requests/            strict JSON binding and transport validation helpers
    │   └── responses/           JSON response helpers and error envelopes
    ├── server/                  HTTP engine, global middleware and error mapping
    ├── services/                business rules and use-case orchestration
    ├── repositories/            SQL queries and transactions
    ├── models/                  shared request/response DTOs and domain types
    ├── apperr/                  application error types and mapping support
    ├── config/, database/       environment configuration and PostgreSQL connection
    └── audit/, mailer/, mfa/, storage/, helpers/  supporting infrastructure

web/src/
├── app/                         routes only; page.tsx renders a component
├── components/<area>/           UI by feature; ui/ holds shared primitives
└── lib/                         API clients, sessions and RBAC; no JSX
```

Rules of thumb: `internal/` packages never import `cmd/`; `models/` imports nothing else from `internal/`; a package name says what it *is* (`audit`, `mailer`), not `utils` or `common`.

Routes belong in `api/routes/<resource>.go`, not in controllers. `server.New` mounts these registration functions under `/api/v1`. Controllers expose endpoint methods; routes apply their shared Guard, readiness checks and route middleware without constructing services or repositories.

Shared request/response DTOs stay in `internal/models` so services never depend on `internal/http/requests`. Controllers use `requests.BindJSON` for HTTP parsing; service validation remains transport-independent.

### How the pieces fit (reference implementation)

- **Errors:** controllers call `fail(c, err)` (`internal/http/controllers/guard.go`); `middleware.Errors` (`internal/http/middleware/errors.go`) resolves it with `server.ErrorTable` (`internal/server/errors.go`); each route group sets its fallback with `middleware.ErrorFallback`. Request-shape errors use `apperr.BadRequest(...)` directly in the controllers.
- **Not found:** repositories return named sentinels (`repositories.ErrUserNotFound`, `ErrAttachmentNotFound`) that wrap `pgx.ErrNoRows`; a bare `pgx.ErrNoRows` is never mapped to HTTP.
- **Authentication:** one shared `controllers.Guard` (`Session()`, `Require(permission)`) sets the caller and the audit actor.
- **Wiring:** `cmd/server/main.go` builds `server.Controllers` and calls `server.New`; controllers depend only on interfaces such as `controllers.UserAdmin`, `controllers.AuthService`, `controllers.PasswordResets`.
- **Request IDs:** every response carries `X-Request-ID`, which also prefixes logged server errors.

## Adding an exception

If a rule genuinely cannot be followed, disable the specific rule on the specific line with a reason, e.g.
`// eslint-disable-next-line @typescript-eslint/no-unsafe-assignment -- third-party callback is untyped`
or `//nolint:forcetypeassert // value set two lines above`. File-wide or project-wide disables need a team decision recorded here.
