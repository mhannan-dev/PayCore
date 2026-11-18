# Coding rules: type safety

PayCore moves money, so a value that is "probably" a string, a number or present is a bug waiting to happen. These rules keep the compiler and linters in a position to catch mistakes before they run. Each rule names what enforces it; **review** means a reviewer must check it because no tool can.

Run the checks before every commit:

```powershell
cd web; npm run check          # tsc --noEmit + eslint (type-aware)
cd api; go vet ./...; go test ./...; golangci-lint run   # see api/.golangci.yml
```

## Shared rules (Go and TypeScript)

1. **Money is never a float.** Amounts are integers in the currency's minor unit: Go `int64`, PostgreSQL `BIGINT`, TypeScript `bigint` for arithmetic and a digit string in JSON (`"25000"`). Parse with a validator (`validAmount`), never `Number()`/`parseFloat`. *(review)*
2. **Closed sets are typed, not free strings.** Roles, permissions, statuses and directions are a Go `const` block plus a database `CHECK`, and a TypeScript string-literal union (`"pending" | "sent" | …`). Never compare against an ad-hoc string literal that is not part of the union. *(tsc, exhaustive lint)*
3. **Switches over a union cover every case.** Either list every member or keep a `default` that handles the unknown value deliberately. *(eslint `switch-exhaustiveness-check`, golangci `exhaustive`)*
4. **API contracts are mirrored by hand, in one place.** Go JSON structs live in `api/internal/model`; their TypeScript mirrors live in `web/src/lib/auth-api.ts`. Changing a field means changing both in the same change, with the same name (snake_case JSON) and nullability (`*string` ⇔ `string | null`). *(review)*
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

## Adding an exception

If a rule genuinely cannot be followed, disable the specific rule on the specific line with a reason, e.g.
`// eslint-disable-next-line @typescript-eslint/no-unsafe-assignment -- third-party callback is untyped`
or `//nolint:forcetypeassert // value set two lines above`. File-wide or project-wide disables need a team decision recorded here.
