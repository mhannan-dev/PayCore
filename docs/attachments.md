# Unified attachments and user photos

All uploaded file metadata lives in **one `attachments` table**. `users.photo_attachment_id` links to a row in that table; `users` no longer stores its own photo path. A composite foreign key ensures that a user can only link an attachment they own.

## Local storage

Image uploads are converted on the API to WebP before storage. JPEG, PNG, GIF,
BMP, TIFF and still WebP are supported; GIF animation and image alpha are
preserved. Other image formats (including SVG/HEIC/AVIF and animated PNG or
animated WebP) are rejected with a validation error rather than saved unchanged.
No external image executable or CGo library is required.

Converted images use `.webp` in both the display filename and random storage key,
with `image/webp` content type. Size and SHA-256 describe the converted bytes.
Sources are limited to 10 MiB; converted images are limited to 3 MiB and are
scaled down as needed to fit. Dimensions are limited to 10000 px per side and 25
megapixels; GIFs additionally allow at most 200 frames and 25 million total canvas
pixels across frames. At most two images convert concurrently. Existing uploaded
images are not modified by new-upload conversion. Profile photos accept new WebP
attachments and retain compatibility with older JPEG, PNG and GIF attachments.

Non-image uploads use a random 64-hex storage key plus the lowercase original extension,
for example `<random-key>.jpg` or `<random-key>.pdf`. Extensions must contain 1–10
letters or digits; missing/unsafe extensions use `.bin`. The original filename
and detected content type remain in the database and control downloads. An
extension does not make the file public or change its content type.

Migration 012 permits both extended and legacy extensionless storage keys. To
repair existing database-linked files, run from `api`:

```powershell
go run ./cmd/repair-upload-extensions
```

The repair verifies size/checksum, creates the new file without overwriting,
updates the locked database row, then removes the old copy after commit. It
retains the original if the database update fails. Unreferenced/orphan files are
left untouched because their original filenames are unknown.

File bytes live in the local `UPLOAD_DIR` folder, defaulting to `api/uploads/` when the API is launched through `run.ps1` and `/app/uploads` in the Compose API container. This folder is ignored by Git. Back up the database and upload folder together. Set an absolute `UPLOAD_DIR` if launching from a different working directory; the Compose API uses the persistent `api_uploads` volume.

The table records ID, owner, display filename, server-detected MIME type, byte size, SHA-256 checksum, storage kind/key, creation time and deletion time. Storage keys are random and never exposed in JSON. Filenames never become filesystem paths; Go's confined `os.Root` filesystem API keeps access inside the upload directory.

## Endpoints

Every request requires the real backend bearer token and an explicit capability: `attachments:read` for listing/metadata/download/photo bytes, `attachments:write` for upload/photo assignment or clearing, and `attachments:delete` for deletion. Migration 016 grants these own-file capabilities to all five built-in roles. Owner IDs are taken from the session, not client input. Even ADMIN does not automatically gain access to another user's files. Missing permission returns `403`; an inaccessible or other-user attachment still returns `404` once the caller has the route's capability.

| Method | Endpoint | Purpose |
| --- | --- | --- |
| POST | `/api/v1/attachments` | Upload exactly one multipart field named `file` |
| GET | `/api/v1/attachments?limit=20&offset=0` | List the current user's active attachments; limit 1–100, offset 0–10000 |
| GET | `/api/v1/attachments/:id` | Read owned attachment metadata |
| GET | `/api/v1/attachments/:id/content` | Download the owned local file |
| DELETE | `/api/v1/attachments/:id` | Soft-delete the record and clear its user-photo link |
| PUT | `/api/v1/auth/photo` | Assign an owned attachment as the current user's photo, or clear it |
| GET | `/api/v1/auth/photo` | Read the current user's private photo bytes |

Limits: one nonempty file up to **10 MiB**; images are converted to WebP and capped at **3 MiB**. Non-image attachments retain their bytes; image uploads follow the WebP conversion rules above. Downloads always use attachment disposition, `nosniff`, and restrictive content-security headers. There is no unauthenticated static uploads directory. User-photo assignment accepts WebP and legacy JPEG, PNG and GIF with valid image headers, at most 25 million pixels and at most 10000 pixels in either dimension. HTML and other general files cannot become inline profile photos. File/metadata errors do not expose storage paths or raw database details.

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

## Service boundary and upload failures

`AttachmentController` handles multipart parsing, request limits, UUID/pagination
validation and response headers. It calls an injected `AttachmentService` for
upload, listing, metadata, download, deletion and photo-content workflows. The
service receives `context.Context`, a session-derived owner and plain stream/model
values; it has no Gin dependency. `PhotoService` owns profile-photo assignment.
`helpers.PrepareUpload` and the injected WebP converter handle filename, MIME and
image validation without selecting HTTP status codes. The central error table
maps typed upload errors to the existing response codes.

Storage is injected through `Write`, `Read` and `Remove` interfaces. Download
ownership and local-file/photo policy are checked before opening any bytes. The
service checks the opened file's size against metadata; failed validation closes
the stream, while successful downloads transfer closing responsibility to the
controllers. Soft deletion keeps the existing retention policy.

Attachment creation uses an explicit database transaction. If metadata creation
definitely fails, the service removes the newly stored file using a fresh cleanup
context with a five-second deadline, even if the request was canceled. Cleanup
failures preserve both error causes for server-side handling. An uncertain commit
acknowledgement is marked explicitly and retains the bytes: deleting them could
break metadata that actually committed. Process crashes, cleanup failures and
uncertain commits still require orphan reconciliation.

## Migration and retention

Migration 007 creates the table, imports any old `users.photo` strings as `EXTERNAL` attachment records, links those IDs, then removes the old photo column. External URLs are preserved as metadata and are never fetched by the server. Their size is reported as 0 (unknown); there is no local checksum or storage key. Legacy external downloads return `409` rather than proxying remote content.

Deletion is logical: metadata gets `deleted_at`, listing/downloading excludes it, and the user's photo link is cleared in the same database transaction. File bytes remain on disk until a separately implemented retention/purge job removes them. Local disk and PostgreSQL cannot commit atomically: files are saved before metadata. Definite metadata failures trigger cleanup; process crashes, failed cleanup or uncertain commits can still leave an unreferenced file. Orphan reconciliation, quotas, malware scanning, and physical purge are not implemented. This service extraction requires no new migration or endpoint change.

## Verify

```powershell
cd api
go test ./...
$env:DATABASE_TEST_URL = 'postgres://paycore:paycore_local_dev@127.0.0.1:5434/paycore?sslmode=disable'
go test ./tests/http/controllers -run TestAttachmentsIntegration -v
```

The integration test uses a temporary database schema and upload directory. It exercises real sessions, uploads, downloads, owner isolation, photo validation/linking/clearing, soft deletion, validation responses and cleanup after an actual database rejection. Service tests cover canceled-request cleanup, cleanup failures, uncertain commits, unauthorized access, image-conversion rejection and closing streams after failed download validation.
