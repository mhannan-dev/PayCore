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
4. **API contracts are mirrored by hand, in one place.** Go JSON structs live in `api/internal/model`; their TypeScript mirrors live in `web/src/lib/auth-api.ts` and `web/src/lib/ledger-api.ts`. Changing a field means changing both in the same change, with the same name (snake_case JSON) and nullability (`*string` ⇔ `string | null`). *(review)*
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

16. **No unchecked type assertions.** Use `v, ok := x.(T)`. Values shared through `gin.Context` use the typed `contextKey[T]` in `internal/handler/context_keys.go`, never `c.MustGet("…").(T)`. *(golangci `forcetypeassert`)*
17. **Errors are compared with `errors.Is` / `errors.As`,** never `==` or a type assertion, and wrapped with `%w`. Return sentinel errors (`ErrEmailTaken`) rather than `nil, nil`. *(golangci `errorlint`, `nilnil`)*
18. **Request bodies bind into typed structs** with size limits (`http.MaxBytesReader`) and validation (`binding` tags or a `Normalize()` method); never decode into `map[string]any`. *(review)*
19. **Every JSON struct field has a `json` tag;** secrets are `json:"-"` (e.g. `PasswordHash`). *(golangci `musttag`)*
20. **SQL is parameterised** (`$1`), never built with `fmt.Sprintf` from input, and scanned into typed fields; nullable columns scan into pointers. Close rows and check `rows.Err()`. *(golangci `gosec`, `sqlclosecheck`, `rowserrcheck`)*
21. **Database constraints back the types:** every enum column has a `CHECK`, every "required" field is `NOT NULL`, every reference a foreign key. The database is the last line of defence. *(review; migrations)*
22. **Interfaces are declared by the consumer** (e.g. `handler.RBACStore`, `service.InvitationStore`) and kept small, so fakes in tests are type-checked against exactly what is used. *(review)*

## Architecture (Go API)

The API is layered. Each layer depends only on the one below it, through interfaces the caller declares:

```text
HTTP request
   │
   ▼
server/      router, global middleware (CORS, request metadata, recovery, errors)
   │
   ▼
handler/     transport only: bind + validate input, call a service, write the response
   │  consumer-side interfaces (e.g. handler.RBACStore)
   ▼
service/     business rules and orchestration; no HTTP, no SQL
   │  consumer-side interfaces (e.g. service.InvitationStore)
   ▼
repository/  SQL and transactions; returns model types and sentinel errors
   │
   ▼
PostgreSQL
```

23. **Handlers stay thin ("no fat handlers").** A handler does four things only: bind and validate the request, take the caller and request values it needs, call **one** service method, and write the response with `pkg/response`. Business rules (permissions beyond the route's, invariants, multi-step workflows, email, audit decisions) belong in a service. A handler that grows branches of business logic or exceeds ~40 lines per endpoint must be split. *(review)*
24. **`*gin.Context` never leaves the transport layer.** Only `server/`, `handler/` and `middleware/` (plus `pkg/response`) import Gin. Services and repositories take a `context.Context` and plain typed arguments, so they can be called from tests, CLIs (`cmd/`) and future workers. Request-scoped facts they need (actor, IP) travel in `context.Context` via typed helpers such as `audit.WithActor`, never as `*gin.Context`. *(golangci `depguard`, see api/.golangci.yml)*
25. **Dependencies are injected, never constructed inside.** Every handler and service receives its collaborators through its constructor (`NewXHandler(svc XService, …)`) as **interfaces declared by the consumer** (rule 22), never concrete types such as `*service.AuthService`. Only the composition root, `cmd/server/main.go`, creates concrete repositories, services and handlers and wires them together. No package-level mutable state and no `init()` wiring. *(review)*
26. **Errors are mapped to HTTP in one place: centralized error handling middleware.** Services and repositories return typed errors (sentinels like `repository.ErrEmailTaken`, or a domain error carrying a code); they never choose HTTP status codes. Handlers do not `switch` on errors: they call `c.Error(err)` and return, and a single error middleware in `server/` translates the error to the standard envelope (`{"error", "message", "details"}`) using one mapping table, logs unexpected errors with the request ID, and answers 500 without leaking internals. Adding an error means adding one row to that table. *(review)*
27. **Validation happens at the edge, rules in the core.** Handlers validate *shape* (binding tags, size limits, UUID format); services validate *meaning* (e.g. last admin, role changes) and return typed errors. A service must not trust that a handler validated anything business-critical. *(review)*
28. **Tests follow the layers.** Services are tested with in-memory fakes of their consumer-side interfaces; repositories with PostgreSQL in an isolated schema (`isolatedPool`); handlers with `httptest` against fakes of their services. No test needs a running server. *(review)*

### Folder structure

Each folder has one responsibility; put new code where a reader would look for it.

```text
api/
├── cmd/                 entry points only (server = composition root; CLI tools); no business logic
├── internal/
│   ├── server/          router, global middleware wiring, error-mapping table (errors.go)
│   ├── middleware/      cross-cutting HTTP middleware (errors, request ID, rate limit)
│   ├── apperr/          HTTP-ready error type and the mapping-table types
│   ├── handler/         one file per resource: <resource>_handler.go (+ _test.go)
│   ├── service/         one file per use-case area: auth.go, invitation.go, …
│   ├── repository/      one file per aggregate/table group: <name>_repository.go
│   ├── model/           request/response DTOs, domain types, normalization
│   ├── audit/, mailer/, storage/   self-contained infrastructure packages
│   └── config/          environment loading only
├── migrations/          NNN_description.sql, never edited once applied
└── pkg/response/        the JSON envelope (the only package other projects may import)

web/src/
├── app/                 routes only: page.tsx renders a component, plus metadata
├── components/<area>/   UI by feature (admin/, login/, user-profile/, …); ui/ holds shared primitives
└── lib/                 API client (auth-api.ts), session, rbac, pure helpers; no JSX
```

Rules of thumb: `internal/` packages never import `cmd/`; `model/` imports nothing else from `internal/`; a package name says what it *is* (`audit`, `mailer`), not `utils` or `common`.

### How the pieces fit (reference implementation)

- **Errors:** handlers call `fail(c, err)` (`internal/handler/guard.go`); `middleware.Errors` (`internal/middleware/errors.go`) resolves it with `server.ErrorTable` (`internal/server/errors.go`); each route group sets its fallback with `middleware.ErrorFallback`. Request-shape errors use `apperr.BadRequest(...)` directly in the handler.
- **Not found:** repositories return named sentinels (`repository.ErrUserNotFound`, `ErrAttachmentNotFound`) that wrap `pgx.ErrNoRows`; a bare `pgx.ErrNoRows` is never mapped to HTTP.
- **Authentication:** one shared `handler.Guard` (`Session()`, `Require(permission)`) sets the caller and the audit actor.
- **Wiring:** `cmd/server/main.go` builds `server.Handlers` and calls `server.New`; handlers depend only on interfaces such as `handler.UserAdmin`, `handler.AuthService`, `handler.PasswordResets`.
- **Request IDs:** every response carries `X-Request-ID`, which also prefixes logged server errors.

## Adding an exception

If a rule genuinely cannot be followed, disable the specific rule on the specific line with a reason, e.g.
`// eslint-disable-next-line @typescript-eslint/no-unsafe-assignment -- third-party callback is untyped`
or `//nolint:forcetypeassert // value set two lines above`. File-wide or project-wide disables need a team decision recorded here.
