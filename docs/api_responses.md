# API response helpers

Use `paycore/api/internal/http/responses` from controllers and middleware. JSON serialization
is centralized in `responses.JSON`; prefer named helpers for common statuses.

```go
import "paycore/api/internal/http/responses"

responses.OK(c, user)                    // 200, resource body
responses.Created(c, attachment)         // 201, resource body
responses.List(c, items, limit, offset)  // 200, items/limit/offset; nil items become []
responses.NoContent(c)                   // 204, no body
```

Return after an error in a controller:

```go
if err != nil {
    responses.BadRequest(c, "invalid_request", "Check the submitted fields.")
    return
}
```

Available error helpers include `BadRequest`, `Unauthorized`, `Forbidden`,
`NotFound`, `Conflict`, `UnsupportedMediaType`, `ServiceUnavailable`, and
`InternalError`. Use `Error` for another status or `ErrorWithDetails` for
structured validation details. Do not expose raw database errors or secrets.

Errors use this envelope; `details` is optional:

```json
{"error":"invalid_request","message":"Check the submitted fields."}
```

Middleware must stop the controller chain when rejecting a request:

```go
responses.Abort(c, http.StatusForbidden, "access_denied", "Access denied.")
return
```

`responses.Error` writes a response without aborting the chain; `responses.Abort`
does both. Headers such as `Retry-After` must be set before writing the responses.

Health endpoints preserve their existing status/database bodies through `OK`
or `JSON`. File downloads continue to stream bytes rather than return JSON.
