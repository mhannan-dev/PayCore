# Contributing to PayCore

Thanks for helping improve PayCore. This guide explains how to report problems and how to get a change merged. By taking part you agree to keep discussions respectful and constructive.

## Reporting a security vulnerability

**Do not open a public issue for security problems.** Use GitHub's private reporting instead: open the repository's **Security** tab and choose **Report a vulnerability**. See [SECURITY.md](SECURITY.md) for what to include and what to expect.

## Reporting a bug

1. Search [existing issues](../../issues) first; add details to a matching one instead of opening a duplicate.
2. Open a new issue with the **Bug report** template and include:
   - what you did (exact steps, request or page),
   - what you expected and what happened instead (error code and message from the API envelope, browser console output),
   - your environment (OS, Go, Node.js and browser versions; how you started PayCore, e.g. `run.ps1`),
   - logs with **secrets removed**: never paste tokens, passwords, `.env` contents or invitation/reset links.

## Suggesting a feature

Open an issue describing the problem you want solved before writing code. Check [ProjectFeature.md](ProjectFeature.md) and [docs/feature_list_bn.md](docs/feature_list_bn.md) first: the feature may already be planned.

## Making a change

### 1. Set up

Follow [Getting started](README.md#getting-started). Copy `api/.env.example` and `web/.env.example`; the defaults work for local development.

### 2. Branch

Fork the repository and create a branch from `main` named after the change, e.g. `feat/password-reset`, `fix/users-pagination`, `docs/readme-setup`.

### 3. Follow the project rules

- **Type safety:** all Go and TypeScript code follows [docs/coding_rules.md](docs/coding_rules.md). In short: money is never a float, no `any` or `x!` in TypeScript, no unchecked type assertions in Go, closed sets are typed, and API contracts change in Go and TypeScript together.
- **Database changes** go in a new numbered migration in `api/migrations/` (never edit an applied one) and are backed by constraints (`NOT NULL`, `CHECK`, foreign keys).
- **API responses** use `pkg/response`, so errors keep the standard envelope.
- **Security-sensitive code** (auth, permissions, tokens, email links, uploads) needs tests for the failure paths too, not only the happy path.
- **Documentation:** update the relevant file in `docs/` and the README when behaviour, endpoints or configuration change.
- Keep pull requests focused: one feature or fix per PR. Unrelated refactors go in separate PRs.

### 4. Check

Run every check before pushing and make sure all of them pass:

```powershell
cd web; npm run check                    # tsc --noEmit + type-aware ESLint
cd api; go vet ./...; go test ./...      # set DATABASE_TEST_URL to include PostgreSQL tests
golangci-lint run                        # in api/
```

Add or update tests for the behaviour you change. Database code should have an integration test that runs when `DATABASE_TEST_URL` is set.

### 5. Commit

Write commit messages in the imperative, with a short summary line (about 72 characters) and an optional body explaining *why*:

```text
Add password reset request endpoint

Reset requests answer the same for unknown emails so the endpoint
cannot be used to discover accounts.
```

[Conventional Commits](https://www.conventionalcommits.org/) prefixes (`feat:`, `fix:`, `docs:`) are welcome but not required.

### 6. Open a pull request

- Fill in the pull request template: what changed, why, how you tested it, and any screenshots for UI changes.
- Link the issue it resolves (`Closes #123`).
- Keep the PR up to date with `main` and respond to review comments; a maintainer merges once checks pass and the review is approved.

## License

By contributing, you agree that your contributions are licensed under the [MIT License](LICENSE).
