# Unified attachments and user photos

All uploaded file metadata lives in **one `attachments` table**. `users.photo_attachment_id` links to a row in that table; `users` no longer stores its own photo path. A composite foreign key ensures that a user can only link an attachment they own.

## Local storage

File bytes live in the local `UPLOAD_DIR` folder, defaulting to `api/uploads/` when the API is launched through `run.ps1`. This folder is ignored by Git. Back up the database and upload folder together. Set an absolute `UPLOAD_DIR` if launching from a different working directory. Containerized API deployments will need a persistent volume for this directory; the current PostgreSQL-only Compose file does not mount API uploads.

The table records ID, owner, display filename, server-detected MIME type, byte size, SHA-256 checksum, storage kind/key, creation time and deletion time. Storage keys are random and never exposed in JSON. Filenames never become filesystem paths; Go's confined `os.Root` filesystem API keeps access inside the upload directory.

## Endpoints

Every request requires the real backend bearer token. Owner IDs are taken from that session, not client input. Even ADMIN does not automatically gain access to another user's files.

| Method | Endpoint | Purpose |
| --- | --- | --- |
| POST | `/api/v1/attachments` | Upload exactly one multipart field named `file` |
| GET | `/api/v1/attachments?limit=20&offset=0` | List the current user's active attachments; limit 1–100, offset 0–10000 |
| GET | `/api/v1/attachments/:id` | Read owned attachment metadata |
| GET | `/api/v1/attachments/:id/content` | Download the owned local file |
| DELETE | `/api/v1/attachments/:id` | Soft-delete the record and clear its user-photo link |
| PUT | `/api/v1/auth/photo` | Assign an owned attachment as the current user's photo, or clear it |
| GET | `/api/v1/auth/photo` | Read the current user's private photo bytes |

Limits: one nonempty file up to **10 MiB**. General uploads can contain any file type, but downloads always use attachment disposition, `nosniff`, and restrictive content-security headers. There is no unauthenticated static uploads directory. User-photo assignment accepts JPEG, PNG and GIF with valid image headers, at most 25 million pixels and at most 10000 pixels in either dimension. SVG/HTML and other general files cannot become inline profile photos. File/metadata errors do not expose storage paths or raw database details.

Upload with a client that already has a token (substitute the actual API port):

```powershell
$upload = Invoke-RestMethod -Uri 'http://localhost:8080/api/v1/attachments' `
  -Method Post -Headers @{ Authorization = "Bearer $token" } `
  -Form @{ file = Get-Item '.\avatar.png' }

Invoke-RestMethod -Uri 'http://localhost:8080/api/v1/auth/photo' `
  -Method Put -Headers @{ Authorization = "Bearer $token" } `
  -ContentType 'application/json' -Body (@{ attachment_id = $upload.id } | ConvertTo-Json)
```

Clear a photo using `{"attachment_id":null}`. Assigning a photo does not delete the previous attachment. `/auth/me` returns `photo_attachment_id` and a backward-compatible `photo` URL. Local photo URLs require Authorization; a browser must fetch the bytes with its token and display a blob URL, or use an authenticated proxy. A bare `<img src>` cannot attach the bearer header. The frontend upload/gallery controls are not implemented by this backend change.

## Migration and retention

Migration 007 creates the table, imports any old `users.photo` strings as `EXTERNAL` attachment records, links those IDs, then removes the old photo column. External URLs are preserved as metadata and are never fetched by the server. Their size is reported as 0 (unknown); there is no local checksum or storage key. Legacy external downloads return `409` rather than proxying remote content.

Deletion is logical: metadata gets `deleted_at`, listing/downloading excludes it, and the user's photo link is cleared in the same database transaction. File bytes remain on disk until a separately implemented retention/purge job removes them. Local disk and PostgreSQL cannot commit atomically: files are saved before metadata; a process crash or uncertain database write can leave an unreferenced file. Such files are retained rather than risking deletion of an acknowledged-lost database commit. Orphan reconciliation, quotas, malware scanning, and physical purge are not implemented.

## Verify

```powershell
cd api
go test ./...
$env:DATABASE_TEST_URL = 'postgres://paycore:paycore_local_dev@127.0.0.1:5434/paycore?sslmode=disable'
go test ./internal/handler -run TestAttachmentsIntegration -v
```

The integration test uses a temporary database schema and upload directory. It exercises real sessions, uploads, downloads, owner isolation, photo validation/linking/clearing, and soft deletion without leaving test files or users in the project database.
