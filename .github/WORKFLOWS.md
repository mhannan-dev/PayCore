# Automated checks and PR review

The workflows run on pull requests. CI and CodeQL also run on pushes to `main` and `PayCore-Quantum` and can be dispatched manually. CodeQL runs weekly on Monday at 03:23 UTC (09:23 Asia/Dhaka); scheduled runs use the default branch. All external actions are pinned to verified upstream commit hashes. Review changes to these pins before updating them.

## CI

`workflows/ci.yml` runs three independent jobs:

- **API checks:** Go from `api/go.mod`, `go vet`, all tests with `-count=1`, and golangci-lint 2.15.0. PostgreSQL 16 uses disposable CI credentials, and `DATABASE_TEST_URL` is set so database tests execute. Packages run serially to avoid migration-lock starvation. Existing `depguard` and `gosec` rules enforce supported architecture and security checks.
- **Web checks:** Node 22, `npm ci`, `npm run check`, and `npm test` using the committed lockfile.
- **Contract checks:** Foundry 1.10.0, Solidity from `foundry.toml`, OpenZeppelin 5.0.2 and forge-std 1.17.0 at immutable commits. It builds contracts, runs unit/fuzz/invariant tests, and runs the offline deployment-tooling PowerShell suite. Fork tests and live deployment/payment commands are excluded.

Test jobs have repository read permission and receive no application or wallet secrets. The PostgreSQL password in the workflow is public disposable test data. These jobs execute PR code; do not add privileged credentials to them.

## CodeQL

`workflows/codeql.yml` scans Go and JavaScript/TypeScript separately with `security-extended` queries and no-build extraction. It grants `security-events: write` only to the analysis job. Check GitHub Code Security availability for this repository before enabling it in a private repository. Use this committed advanced setup instead of enabling default setup simultaneously. Neither CodeQL nor the architecture linter proves business-rule correctness or audits Solidity.

## Enable Claude review

The Claude job is opt-in to avoid unexpected provider usage or missing-secret failures:

1. In repository **Settings → Secrets and variables → Actions**, add the secret `ANTHROPIC_API_KEY` yourself. Never put it in a commit, PR, issue or chat.
2. Add repository variable `ENABLE_CLAUDE_REVIEW` with the exact value `true`.
3. Ensure repository/organization policy allows the job's `GITHUB_TOKEN` to write PR comments. This workflow supplies that short-lived token directly; it does not require installing the Claude GitHub App or granting content-write/OIDC permissions.
4. Open or update a non-draft PR from a branch in this repository, authored by an owner/member/collaborator and triggered by a user. Fork PRs and bot events are excluded. The upstream action also checks repository write access.

Without the enable variable the job is visibly skipped. With the variable enabled but no API key, it fails with a setup message. Disable the variable to stop provider calls. A 15-minute timeout and 20-turn limit bound each run; these are not a precise monetary spending cap. Configure provider-side usage limits separately.

The review checks out the exact base revision and obtains a bounded PR diff as data. It does not install dependencies or execute PR scripts. Claude's built-in tools are limited to Read/Grep/Glob; shell, edit, agent and web tools are denied. The prompt uses trusted base-branch rules; added PR instructions are untrusted review material. A separate fixed script validates the structured result and publishes/updates one advisory comment, without shell interpolation of model output. Stale revisions are rejected before publication. Diffs above 500 KB fail with a manual-review message; unavailable/truncated context must be disclosed by the review. The first PR adding these files may lack the new base-branch planning documents until they are merged.

This reduces exposure but is not a general prompt-injection guarantee. The action and provider still process repository content. The model never receives permission to push code or merge. Review the report and verify consequential findings with tests.

## Required checks and activation

After pushing these files and observing successful GitHub runs, configure branch rules for the intended protected branches. Require **API checks**, **Web checks**, **Contract checks** and the two **CodeQL** analyses as appropriate. Keep Claude advisory; a completed AI job does not mean no vulnerabilities were found. Restrict changes to workflows and security settings through your normal maintainer review.

Creating these files locally does not install secrets, change repository settings, enable paid security features, or prove the hosted workflows ran. No workflow has been dispatched or PR comment posted by this local setup.

References: [Claude Action setup](https://github.com/anthropics/claude-code-action/blob/main/docs/setup.md), [structured outputs](https://github.com/anthropics/claude-code-action/blob/main/docs/usage.md#structured-outputs), [action security](https://github.com/anthropics/claude-code-action/blob/main/docs/security.md), [CodeQL setup](https://docs.github.com/en/code-security/how-tos/find-and-fix-code-vulnerabilities/configure-code-scanning/configure-code-scanning), and [GitHub Actions secure use](https://docs.github.com/en/actions/reference/security/secure-use).
