# Security policy

PayCore handles authentication, permissions and (in future) money movement, so security reports are taken seriously.

## Supported versions

PayCore is in alpha. Only the latest commit on `main` receives security fixes.

## Reporting a vulnerability

**Please do not report security vulnerabilities in public issues, discussions or pull requests.**

Use GitHub's private vulnerability reporting: open the repository's **Security** tab and choose **Report a vulnerability**. Include:

- a description of the issue and its impact,
- the affected component (API endpoint, page, migration, configuration),
- steps to reproduce or a proof of concept,
- any suggested fix.

Do not include real credentials, tokens or personal data in the report.

You can expect an acknowledgement within 5 working days and a status update within 14 days. We will coordinate a fix and a disclosure date with you, and credit you in the release notes unless you prefer otherwise.

## Scope

In scope: the Go API (`api/`), the web console (`web/`), database migrations, and the default configuration and launch scripts in this repository.

Out of scope: vulnerabilities in third-party services or dependencies that are already publicly known (report those upstream), and issues that require an already-compromised machine or administrator account.

## Hardening notes for operators

- Set `ENV=production`, an explicit `WEB_ORIGINS`, and a public `APP_URL` in production.
- Use a mail provider account dedicated to PayCore and keep SMTP credentials in a secret manager, not in files.
- Replace the development database password from `compose.yaml` and never expose PostgreSQL publicly.
- Leave `NEXT_PUBLIC_DEMO_PASSWORD` unset in any shared or production build.
