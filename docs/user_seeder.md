# Seed sample users

Start PostgreSQL from the project root, then run the seeder:

```powershell
docker compose up -d --wait postgres
cd api
go run ./cmd/seed-users
```

The command asks for a password and confirmation without echoing them. Development seed passwords may be 8–72 bytes; ordinary account creation requires exactly 10 characters. Sample users share that input password, but each stored bcrypt hash has its own salt. The password and hashes are never printed or hardcoded.

| Email | First name | Last name | Roles |
| --- | --- | --- | --- |
| admin@yopmail.com | Admin | Admin | ADMIN, ACCOUNTANT |
| accountant@yopmail.com | Accountant | NULL | ACCOUNTANT |
| auditor@yopmail.com | Auditor | Auditor | COMPLIANCE_AUDITOR |

The photo attachment link defaults to NULL and new users are active. All files/photos share the [attachments table](attachments.md). There is no `name` column. Fixtures live in `api/database/seeders/users.go`.

The command prints explicit `first_name`, `last_name`, `email`, `roles`, and `status` columns. An absent last name is printed as `NULL`. Seeder results also expose `first_name` and nullable `last_name` JSON fields; skipped users report their current database values rather than fixture defaults.

The command applies pending migrations before seeding. Earlier migrations add ADMIN support and convert the old scalar `role` into one-item `roles` arrays. Migration 026 consolidates the role set to ADMIN, ACCOUNTANT and COMPLIANCE_AUDITOR: OPERATOR becomes ACCOUNTANT. The runner stops before 026 if LEAD_OPERATOR or STAFF_ENGINEER users still need explicit role mapping; follow the [legacy upgrade review](rbac.md#legacy-role-upgrade-review). Only a newly seeded admin gets both ADMIN and ACCOUNTANT; an existing admin's role list stays unchanged on rerun.

Rerunning skips existing emails: existing passwords, roles, active status and profile fields are preserved. If an email already existed, use its original password, not the newly entered seed password. Inserts are committed together; a failed insert rolls back that seed transaction. Successfully applied schema migrations remain applied.

To explicitly refresh the three fixture emails, run `go run ./cmd/seed-users -refresh`. It resets their names, roles, password, photo and active status to fixture values, and revokes their existing database sessions in the same transaction. Other users remain untouched; user IDs and creation times are preserved. The command still prompts for the password unless `PAYCORE_SEED_PASSWORD` is supplied.

For noninteractive local/test runs, supply `PAYCORE_SEED_PASSWORD` through your environment or secret manager; avoid putting passwords in command arguments or committed files. The CLI runs only with `ENV=development`, `local`, or `test`. Seeding is explicit and never runs during ordinary API startup. Backend CAPTCHA verification remains required for login; the current frontend still uses demo state.

Verify public fields from the project root:

```powershell
docker compose exec postgres psql -U paycore -d paycore -c "SELECT first_name, last_name, email, roles, is_active FROM users ORDER BY email;"
```
