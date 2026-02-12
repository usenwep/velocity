# Error Handling

## Handler Errors

Every velocity handler returns an `error`. This gives middleware a central place to log failures, convert them to responses, or both.

```go
srv.Handle("/users", func(c *velocity.Context) error {
    user, err := db.FindUser(c.Body())
    if err != nil {
        return c.InternalError("something went wrong")
    }
    return c.JSON(user)
})
```

If a handler returns a non-nil error without sending a response, the error propagates back through the middleware chain. Your middleware decides what to do with it.

## Error Middleware

A common pattern is to place an error-catching middleware near the top of the chain. It calls the next handler, inspects the returned error, and sends a response if one hasn't been sent already:

```go
func errorHandler() velocity.MiddlewareFunc {
    return func(next velocity.HandlerFunc) velocity.HandlerFunc {
        return func(c *velocity.Context) error {
            err := next(c)
            if err != nil {
                c.Logger().Error("unhandled error",
                    "path", c.Path(),
                    "error", err,
                )
                return c.InternalError("internal error")
            }
            return nil
        }
    }
}

srv.Use(errorHandler())
```

Register the error handler early (after `Recover`) so it wraps all subsequent middleware and handlers.

## Panic Recovery

The built-in `Recover` middleware catches panics and converts them to an `internal_error` response. It also logs the panic value and request path.

```go
srv.Use(velocity.Recover())
```

`Recover` should be the first middleware registered so it wraps everything. Without it, a panic in a handler crashes the server.

A typical middleware stack looks like this:

```go
srv.Use(velocity.Recover())        // 1st: catches panics
srv.Use(errorHandler())            // 2nd: catches returned errors
srv.Use(velocity.RequestLogger())  // 3rd: logs all requests
```

## Sentinel Errors

velocity defines two sentinel errors for common conditions.

### ErrEmptyBody

Returned by `Context.Bind` when the request body is nil or empty.

```go
srv.Router().Write("/data", func(c *velocity.Context) error {
    var req DataRequest
    if err := c.Bind(&req); err != nil {
        if errors.Is(err, velocity.ErrEmptyBody) {
            return c.BadRequest("request body required")
        }
        return c.BadRequest("invalid json: " + err.Error())
    }
    // ...
})
```

### ErrServerNotRunning

Returned by notification methods (`Notify`, `NotifyAll`, `NotifyJSON`, `NotifyAllJSON`, `NotifyWithOptions`) when called on a server that hasn't been started or has already shut down.

```go
if err := srv.Notify(peer, "update", "/data", body); err != nil {
    if errors.Is(err, velocity.ErrServerNotRunning) {
        log.Println("server not running, skipping notification")
        return
    }
    log.Printf("notify failed: %v", err)
}
```

## Response Status Constants

velocity re-exports nwep's response status constants for use in handlers:

| Constant | Value | Used by |
|----------|-------|---------|
| `StatusOK` | `"ok"` | `c.OK()`, `c.JSON()` |
| `StatusCreated` | `"created"` | `c.Created()` |
| `StatusAccepted` | `"accepted"` | |
| `StatusNoContent` | `"no_content"` | `c.NoContent()` |
| `StatusBadRequest` | `"bad_request"` | `c.BadRequest()`, `MethodFilter` |
| `StatusUnauthorized` | `"unauthorized"` | `c.Unauthorized()`, `RequirePeer` |
| `StatusForbidden` | `"forbidden"` | `c.Forbidden()`, `AllowPeers` |
| `StatusNotFound` | `"not_found"` | `c.NotFound()`, default not-found handler |
| `StatusConflict` | `"conflict"` | |
| `StatusRateLimited` | `"rate_limited"` | |
| `StatusInternalError` | `"internal_error"` | `c.InternalError()`, `Recover` |
| `StatusUnavailable` | `"unavailable"` | |

Use `c.Error(status, msg)` or `c.Respond(status, body)` for statuses without a dedicated helper.

## Error Patterns

### Validation errors

```go
srv.Router().Write("/users", func(c *velocity.Context) error {
    var req CreateUserRequest
    if err := c.Bind(&req); err != nil {
        return c.BadRequest(err.Error())
    }
    if req.Name == "" {
        return c.BadRequest("name is required")
    }
    // ...
})
```

### Not found with context

```go
srv.Router().Read("/users", func(c *velocity.Context) error {
    user, err := db.FindUser(userID)
    if err == ErrNotFound {
        return c.NotFound("user not found")
    }
    if err != nil {
        return c.InternalError("database error")
    }
    return c.JSON(user)
})
```

### Access control

```go
srv.Handle("/admin/shutdown", shutdownHandler,
    velocity.RequirePeer(),
    velocity.AllowPeers(adminNodeID),
)
```

Unauthenticated peers receive `unauthorized`. Authenticated peers not in the allow list receive `forbidden`. The handler only runs for allowed peers.
