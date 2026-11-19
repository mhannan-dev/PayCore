## What and why

<!-- What does this PR change, and what problem does it solve? -->

Closes #

## Type of change

- [ ] Bug fix
- [ ] New feature
- [ ] Refactor (no behaviour change)
- [ ] Documentation
- [ ] Database migration
- [ ] Breaking change (API contract, configuration or migration that needs action from users)

## How was this tested?

<!-- Commands run, scenarios checked by hand, screenshots for UI changes. -->

## Checklist

- [ ] `cd web; npm run check` passes (type check + type-aware lint)
- [ ] `cd api; go vet ./...` and `go test ./...` pass (with `DATABASE_TEST_URL` for database changes)
- [ ] `golangci-lint run` passes in `api/`
- [ ] Follows the [coding rules](../docs/coding_rules.md): no `any`, `x!`, unchecked assertions, or float money
- [ ] Handlers stay thin; no `*gin.Context` in services/repositories; dependencies injected as consumer-side interfaces; errors returned (not mapped to HTTP) below the handler
- [ ] API contract changes are mirrored in Go (`api/internal/model`) and TypeScript (`web/src/lib/auth-api.ts`)
- [ ] Schema changes are in a new migration with constraints; no applied migration was edited
- [ ] Tests cover the new behaviour, including failure paths for auth, permission or token code
- [ ] Docs and README updated for changed behaviour, endpoints or configuration
- [ ] No secrets, tokens, real credentials or personal data are included

## Screenshots

<!-- For UI changes: before / after. Delete if not applicable. -->
