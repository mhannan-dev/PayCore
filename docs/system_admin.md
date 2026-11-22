# System administrator

`ADMIN` is the System Administrator role. The development fixture
`admin@yopmail.com` has `ADMIN` and `ACCOUNTANT` roles.

To explicitly grant an existing account administrator access, run from `api`:

```powershell
go run ./cmd/grant-system-admin -email admin@yopmail.com
```

This command uses the configured database, activates only the target account,
preserves its password/profile/existing roles, synchronizes role membership,
and grants ADMIN every permission currently registered in the database. It fails
if the account does not exist. Repeating it does not duplicate memberships.
Granting permissions to ADMIN also applies to other users assigned that role.

RBAC seeding grants ADMIN all registered permissions, including custom ones.
New permission migrations should also grant their permissions to ADMIN, or run
the RBAC seeder or the explicit grant command afterwards.

Refresh the current session through `/api/v1/auth/me` or sign in again to refresh
the permissions shown by the web app. Authentication, file ownership, and
transaction/business checks continue to apply. This grants application access,
not PostgreSQL superuser privileges.
