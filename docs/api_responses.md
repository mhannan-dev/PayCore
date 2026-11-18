# API response helpers

Use `paycore/api/pkg/response` from handlers and middleware. JSON serialization
is centralized in `response.JSON`; prefer named helpers for common statuses.

```go
import "paycore/api/pkg/response"

response.OK(c, user)                    // 200, resource body
response.Created(c, attachment)         // 201, resource body
response.List(c, items, limit, offset)  // 200, items/limit/offset; nil items become []
response.NoContent(c)                   // 204, no body
```

Return after an error in a handler:

```go
if err != nil {
    response.BadRequest(c, "invalid_request", "Check the submitted fields.")
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

Middleware must stop the handler chain when rejecting a request:

```go
response.Abort(c, http.StatusForbidden, "access_denied", "Access denied.")
return
```

`response.Error` writes a response without aborting the chain; `response.Abort`
does both. Headers such as `Retry-After` must be set before writing the response.

Health endpoints preserve their existing status/database bodies through `OK`
or `JSON`. File downloads continue to stream bytes rather than return JSON.
