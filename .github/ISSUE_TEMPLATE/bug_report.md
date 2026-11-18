---
name: Bug report
about: Report something that does not work as documented
title: "[Bug] "
labels: bug
assignees: ""
---

<!--
Security vulnerabilities must NOT be reported here. Use the repository's
Security tab → "Report a vulnerability" (see SECURITY.md).

Never paste tokens, passwords, .env contents, or invitation/reset links.
-->

## Summary

A clear, one-sentence description of the bug.

## Steps to reproduce

1. Go to / call …
2. Do …
3. See …

## Expected behaviour

What you expected to happen.

## Actual behaviour

What happened instead. Include the API error envelope (`{"error": "...", "message": "..."}`), HTTP status, or the message shown in the console.

## Area

- [ ] API (`api/`)
- [ ] Web console (`web/`)
- [ ] Database / migrations
- [ ] Email (invitations, password reset)
- [ ] Setup / `run.ps1` / Docker
- [ ] Documentation

## Environment

- OS:
- Browser and version (for web issues):
- Go version (`go version`):
- Node.js version (`node --version`):
- How PayCore was started (`run.ps1`, manual, Docker, …):
- Commit or version:

## Logs and screenshots

Relevant API log lines or screenshots, with secrets and personal data removed.

## Additional context

Anything else that might help (recent changes, workarounds tried).
