# PayCore Web

The PayCore operations console: a Next.js 16 App Router app (React 19, strict TypeScript, Tailwind CSS 4). For the whole project, setup and configuration, see the [root README](../README.md).

Sign-in, roles, user management, invitations, password reset and profiles call the real Go API. **Business screens (Accounts, Transfers, Ledger, Audit, Reports, Notifications, Settlements, Simulator) still use shared, balanced sample data** until the ledger core API exists; they send no financial commands.

## Run

From the repository root, `.\run.ps1` starts the database, API and web together and passes the right API URL. To run only the web against an API that is already running:

```powershell
cd web
Copy-Item .env.example .env.local   # set NEXT_PUBLIC_API_URL if the API is not on :8080
npm install
npm run dev
```

## Pages

| Route | Content | Access |
| --- | --- | --- |
| `/login` | Email, password and math CAPTCHA sign-in; demo identity quick-fill in development | public |
| `/forgot-password`, `/reset-password` | Request a reset link; choose a new password from the emailed link | public |
| `/set-password` | Accept an invitation and choose a password | public |
| `/` | Overview, sample activity chart, balanced journal summary | signed in |
| `/accounts` | Accounts and posting history (sample) | `ledger:read` |
| `/transfers` | Transfer validation and request preview (sample) | `transfers:read` |
| `/ledger` | Journals with expandable postings (sample) | `ledger:read` |
| `/audit` | Zero-sum invariant check (sample) | `audit:read` |
| `/reports` | Balance summary and CSV export (sample) | `reports:read` |
| `/notifications`, `/settlements` | Delivery records; payout lifecycle (sample) | signed in |
| `/simulator` | Idempotency replay demonstration (in memory) | `simulator:run` |
| `/profile` | Name, profile photo, roles and permissions | signed in |
| `/admin/users` | Users & access: search, paging, invite, edit roles, delete | `users:read` (changes need `users:write`) |
| `/admin/roles` | Roles & permissions matrix | `users:read` |

Page access is decided by `ROUTE_PERMISSIONS` in `src/lib/rbac.ts`; it controls what is shown, while the API enforces permissions.

## Source layout

- `src/app/**/page.tsx`: routes and metadata (rendered statically).
- `src/components/`: UI, grouped by area (`app-shell/`, `admin/`, `login/`, `set-password/`, `user-profile/`, `workspace/`, `ui/`).
- `src/lib/auth-api.ts`: typed API client; the TypeScript mirror of the Go API contract.
- `src/lib/auth-session.ts`: browser session store (token and user, refreshed from `/auth/me`).
- `src/lib/rbac.ts`: permission codes and route permissions.
- `src/lib/demo-data.ts`: sample ledger fixtures with `bigint` balance helpers.

## Checks

```powershell
npm run check   # tsc --noEmit + type-aware ESLint (see ../docs/coding_rules.md)
npm run build
```

Typography uses local system fonts so builds do not depend on Google Fonts access.
