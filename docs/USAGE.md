# Usage Guide

This guide covers the full velocity API: server setup, routing, the request context, middleware, notifications, identity verification, and configuration.

## Table of Contents

- [Server](#server)
  - [Creating a server](#creating-a-server)
  - [Options](#options)
  - [Lifecycle](#lifecycle)
- [Routing](#routing)
  - [Exact routes](#exact-routes)
  - [Method-specific routes](#method-specific-routes)
  - [Prefix routes](#prefix-routes)
  - [Route groups](#route-groups)
  - [Not found](#not-found)
  - [Lookup order](#lookup-order)
- [Context](#context)
  - [Request accessors](#request-accessors)
  - [Response helpers](#response-helpers)
  - [JSON](#json)
  - [Streaming](#streaming)
  - [Peer identity](#peer-identity)
  - [Key-value store](#key-value-store)
- [Middleware](#middleware)
  - [Writing middleware](#writing-middleware)
  - [Middleware options](#middleware-options)
  - [Short-circuiting](#short-circuiting)
  - [Built-in middleware](#built-in-middleware)
- [Notifications](#notifications)
  - [Sending to a single peer](#sending-to-a-single-peer)
  - [Broadcasting](#broadcasting)
  - [JSON notifications](#json-notifications)
  - [Advanced options](#advanced-options)
  - [Connected peers](#connected-peers)
- [Keypairs](#keypairs)
- [Trust and Identity Verification](#trust-and-identity-verification)
- [Configuration](#configuration)
- [Logging](#logging)

## Server

### Creating a server

`velocity.New` creates a server bound to the given address. It accepts functional options for configuration.

```go
srv, err := velocity.New(":6937")
if err != nil {
    log.Fatal(err)
}
```

If no keypair option is provided, a random Ed25519 keypair is generated. For a persistent identity across restarts, use `WithKeyFile`:

```go
srv, err := velocity.New(":6937",
    velocity.WithKeyFile("server.key"),
)
```

### Options

Options configure the server at construction time. They are applied in order; if any returns an error, `New` fails immediately.

```go
srv, err := velocity.New(":6937",
    velocity.WithKeyFile("server.key"),
    velocity.WithSettings(nwep.Settings{
        MaxStreams: 200,
        TimeoutMs: 60000,
    }),
    velocity.WithLogger(velocity.DefaultLogger()),
    velocity.WithRole("regular"),
    velocity.WithOnConnect(func(c *nwep.Conn) {
        _, nid := c.PeerIdentity()
        log.Printf("connected: %s", nid)
    }),
    velocity.WithOnDisconnect(func(c *nwep.Conn, code int) {
        _, nid := c.PeerIdentity()
        log.Printf("disconnected: %s (code %d)", nid, code)
    }),
    velocity.OnStart(func(s *velocity.Server) {
        log.Printf("listening %s", s.Addr())
        log.Printf("url %s", s.URL("/"))
    }),
    velocity.OnShutdown(func(s *velocity.Server) {
        log.Printf("shutting down")
    }),
)
```

Available options:

| Option | Description |
|--------|-------------|
| `WithKeypair(kp)` | Set Ed25519 keypair directly |
| `WithKeyFile(path)` | Load or generate keypair from file |
| `WithKeyEnv(envVar)` | Load keypair from environment variable |
| `WithSettings(s)` | Set nwep transport settings |
| `WithLogger(l)` | Set logger instance |
| `WithRole(role)` | Set WEB/1 handshake role |
| `WithOnConnect(fn)` | Callback when peer connects |
| `WithOnDisconnect(fn)` | Callback when peer disconnects |
| `WithTrust(tc)` | Configure trust store for identity verification |
| `WithConfig(cfg)` | Apply a Config struct |
| `OnStart(fn)` | Callback after server binds |
| `OnShutdown(fn)` | Callback before server closes |

### Lifecycle

`Run` starts the server and blocks until shutdown:

```go
if err := srv.Run(); err != nil {
    log.Fatal(err)
}
```

For non-blocking startup, use `Start` and `Shutdown` separately:

```go
if err := srv.Start(); err != nil {
    log.Fatal(err)
}
// server is now accepting connections
// ...
srv.Shutdown()
```

After `Shutdown`, the server must not be reused.

Server identity is available immediately after `New`:

```go
fmt.Println(srv.NodeID())     // available before Start
fmt.Println(srv.URL("/"))     // available after Start
fmt.Println(srv.Addr())       // available after Start
```

The underlying nwep server is accessible for anything velocity doesn't wrap:

```go
raw := srv.NWEPServer() // *nwep.Server, nil before Start
```

## Routing

Register all routes before calling `Run` or `Start`. After startup, route lookup is safe for concurrent use.

### Exact routes

`Handle` registers a handler for a path, matching any request method:

```go
srv.Handle("/health", func(c *velocity.Context) error {
    return c.OK([]byte("ok"))
})
```

### Method-specific routes

These take precedence over path-only routes. Use the convenience methods `Read`, `Write`, `Update`, `Delete`, or the general `Method`:

```go
srv.Router().Read("/users", listUsers)
srv.Router().Write("/users", createUser)
srv.Router().Update("/users", updateUser)
srv.Router().Delete("/users", deleteUser)

// equivalent to:
srv.Router().Method(velocity.MethodRead, "/users", listUsers)
```

### Prefix routes

`HandlePrefix` matches any path starting with the given prefix. When multiple prefixes match, the longest one wins. Prefix routes are checked after all exact routes.

```go
srv.Router().HandlePrefix("/files/", serveFiles)
```

### Route groups

Groups share a path prefix and middleware. All routes registered on a group inherit its prefix, and the group's middleware runs after global middleware but before any route-level middleware.

```go
api := srv.Group("/api/v1")
api.Read("/users", listUsers)      // matches /api/v1/users
api.Write("/users", createUser)    // matches /api/v1/users
```

Groups nest:

```go
api := srv.Group("/api/v1")

admin := api.Group("/admin", velocity.RequirePeer())
admin.Handle("/stats", statsHandler) // matches /api/v1/admin/stats
// RequirePeer runs on all /api/v1/admin/* routes
```

Groups support all the same registration methods as Router: `Handle`, `Method`, `Read`, `Write`, `Update`, `Delete`, `HandlePrefix`, and `Group`.

### Not found

When no route matches, the not-found handler is called. If none is set, the server responds with status `not_found` and body `not found`.

```go
srv.Router().SetNotFound(func(c *velocity.Context) error {
    return c.JSON(map[string]string{
        "error": "route not found",
        "path":  c.Path(),
    })
})
```

### Lookup order

For each incoming request, the router checks in this order:

1. Method-specific exact match (`Router.Method`, `Read`, `Write`, etc.)
2. Path-only exact match (`Router.Handle`)
3. Longest prefix match (`Router.HandlePrefix`)
4. Not-found handler

## Context

Every handler receives a `*Context`. It wraps the nwep request and response, provides helpers for common patterns, and carries a key-value store for passing data between middleware and handlers.

Contexts are pooled and reused. Do not hold a reference after the handler returns.

### Request accessors

```go
c.Method()       // "read", "write", "update", "delete"
c.Path()         // "/api/v1/users"
c.Body()         // raw request body as []byte
c.Header("name") // (value string, ok bool)
c.Headers()      // all headers as []nwep.Header
c.RequestID()    // [16]byte request identifier
c.TraceID()      // [16]byte trace identifier
```

### Response helpers

```go
c.OK(body)             // status "ok" with body
c.Created(body)        // status "created" with body
c.NoContent()          // status "no_content", no body
c.Respond(status, body) // arbitrary status and body

c.NotFound("msg")      // status "not_found"
c.BadRequest("msg")    // status "bad_request"
c.Unauthorized("msg")  // status "unauthorized"
c.Forbidden("msg")     // status "forbidden"
c.InternalError("msg") // status "internal_error"
c.Error(status, "msg") // arbitrary error status
```

Only one response per request. For fine-grained control, use `SetStatus`, `SetHeader`, and `Write`:

```go
c.SetStatus(velocity.StatusOK)
c.SetHeader("x-request-id", "abc123")
return c.Write(body)
```

### JSON

`Bind` deserializes the request body. `JSON` serializes the response.

```go
srv.Router().Write("/users", func(c *velocity.Context) error {
    var req CreateUserRequest
    if err := c.Bind(&req); err != nil {
        return c.BadRequest(err.Error())
    }
    user := createUser(req)
    return c.JSON(user)
})
```

`Bind` returns `velocity.ErrEmptyBody` if the body is nil or empty.

### Streaming

For responses that need to be sent incrementally:

```go
srv.Handle("/stream", func(c *velocity.Context) error {
    for i := 0; i < 10; i++ {
        _, err := c.StreamWrite([]byte(fmt.Sprintf("chunk %d\n", i)))
        if err != nil {
            return err
        }
    }
    c.StreamClose(0) // 0 = graceful close
    return nil
})
```

`c.StreamID()` returns the stream identifier. `c.IsServerInitiated()` reports whether the stream was opened by the server rather than by a client request.

### Peer identity

Every WEB/1 connection is mutually authenticated with Ed25519. The connected peer's identity is always available:

```go
nodeID := c.PeerNodeID()           // nwep.NodeID (32 bytes)
pubkey, nodeID := c.PeerIdentity() // Ed25519 pubkey + node ID
conn := c.Conn()                   // underlying *nwep.Conn
```

These return zero values if the connection is unavailable. Use `nodeID.IsZero()` to check.

### Key-value store

The context carries a per-request store for passing data between middleware and handlers:

```go
// in middleware
c.Set("user_id", 42)

// in handler
val, ok := c.Get("user_id")
uid := c.MustGet("user_id") // panics if key not set
```

## Middleware

### Writing middleware

A middleware takes the next handler, returns a new handler. Call `next` to continue the chain.

```go
func requestID() velocity.MiddlewareFunc {
    return func(next velocity.HandlerFunc) velocity.HandlerFunc {
        return func(c *velocity.Context) error {
            rid := c.RequestID()
            c.SetHeader("x-request-id", hex.EncodeToString(rid[:]))
            return next(c)
        }
    }
}
```

Global middleware is registered with `Use`. Per-route middleware is passed as trailing arguments to route registration methods.

```go
// global: runs on every request
srv.Use(velocity.Recover(), requestID())

// per-route: only runs on this route, after global middleware
srv.Handle("/admin", adminHandler, velocity.RequirePeer())
```

### Middleware options

When writing middleware intended for reuse, accept options through a struct parameter even if the middleware currently needs none. This makes the API easy to extend later without breaking callers.

```go
type LoggerOptions struct {
    Level string
}

func Logger(opts LoggerOptions) velocity.MiddlewareFunc {
    return func(next velocity.HandlerFunc) velocity.HandlerFunc {
        return func(c *velocity.Context) error {
            // ...
            return next(c)
        }
    }
}
```

### Short-circuiting

A middleware that skips calling `next` prevents all remaining middleware and the handler from running. This is how access control works:

```go
func RequireAdmin(adminID nwep.NodeID) velocity.MiddlewareFunc {
    return func(next velocity.HandlerFunc) velocity.HandlerFunc {
        return func(c *velocity.Context) error {
            if c.PeerNodeID() != adminID {
                return c.Forbidden("admin only")
            }
            return next(c) // only reached if peer is admin
        }
    }
}
```

### Built-in middleware

**Recover** catches panics and responds with `internal_error`. Register it first so it wraps everything else.

```go
srv.Use(velocity.Recover())
```

**RequestLogger** logs every completed request at info level with method, path, peer node ID, and duration.

```go
srv.Use(velocity.RequestLogger())
```

**RequirePeer** rejects requests where the peer has a zero-valued node ID (not authenticated) with status `unauthorized`.

```go
admin := srv.Group("/admin", velocity.RequirePeer())
```

**AllowPeers** restricts access to a set of node IDs. Other peers receive status `forbidden`.

```go
admin := srv.Group("/admin", velocity.AllowPeers(trustedNodeA, trustedNodeB))
```

**MethodFilter** restricts which request methods a route accepts. Other methods receive status `bad_request`.

```go
srv.Handle("/readonly", handler, velocity.MethodFilter(velocity.MethodRead))
```

## Notifications

velocity servers can push notifications to connected peers at any point: inside a handler, from a goroutine, or during a lifecycle callback.

### Sending to a single peer

```go
err := srv.Notify(peerID, "update", "/resource/123", []byte("new data"))
```

`event` is an application-defined name. `path` identifies the resource. `body` may be nil.

### Broadcasting

```go
srv.NotifyAll("shutdown", "/", []byte("server restarting in 60s"))
```

`NotifyAll` sends to every connected peer. It is a no-op if the server is not running.

### JSON notifications

```go
srv.NotifyJSON(peerID, "update", "/users/1", map[string]string{"name": "alice"})
srv.NotifyAllJSON("tick", "/clock", map[string]int64{"time": time.Now().Unix()})
```

### Advanced options

For custom headers or protocol-level options, use `NotifyWithOptions`:

```go
srv.NotifyWithOptions(peerID, "update", "/data", body, &nwep.NotifyOptions{
    Headers: []nwep.Header{
        {Name: "priority", Value: "high"},
    },
})
```

### Connected peers

```go
count := srv.ConnectionCount()
peers := srv.ConnectedPeers() // []nwep.NodeID snapshot
```

## Keypairs

velocity provides helpers for loading and managing Ed25519 keypairs.

**From a file.** Loads an existing hex seed or generates and writes a new one (mode 0600):

```go
kp, err := velocity.LoadOrGenerateKeypair("server.key")
```

The file contains a 64-character hex-encoded 32-byte seed.

**From a hex string:**

```go
kp, err := velocity.KeypairFromHexSeed("abcdef0123456789...")
```

**From an environment variable:**

```go
kp, err := velocity.KeypairFromEnv("SERVER_KEY")
```

**In tests.** Panics on failure:

```go
kp := velocity.MustKeypair(nwep.GenerateKeypair())
```

## Trust and Identity Verification

velocity integrates with nwep's trust system for verifying peer identities against trusted anchors.

```go
srv, err := velocity.New(":6937",
    velocity.WithTrust(&velocity.TrustConfig{
        Anchors: []nwep.BLSPubkey{anchorKey},
    }),
)
```

The `TrustVerify` middleware checks peer identity on each request. Verified identities are stored in the context and retrieved with `VerifiedIdentity`:

```go
ts, _ := (&velocity.TrustConfig{Anchors: anchors}).Build()

srv.Use(velocity.TrustVerify(ts))

srv.Handle("/secure", func(c *velocity.Context) error {
    vi := velocity.VerifiedIdentity(c)
    if vi == nil {
        return c.Unauthorized("identity not verified")
    }
    return c.OK([]byte("verified"))
})
```

`TrustVerify` does not reject unverified peers on its own. It only populates the context. Combine it with `RequirePeer` or custom middleware to enforce verification.

## Configuration

For declarative setup, use the `Config` struct with `WithConfig`. Zero-valued fields are ignored.

```go
cfg := velocity.DefaultConfig() // port 4433, 30s timeout, info logging
cfg.Addr = ":6937"
cfg.KeyFile = "server.key"
cfg.MaxStreams = 200
cfg.TimeoutMs = 60000

srv, err := velocity.New(cfg.Addr, velocity.WithConfig(cfg))
```

`Config` fields:

| Field | Type | Description |
|-------|------|-------------|
| `Addr` | `string` | UDP listen address |
| `KeyFile` | `string` | Path to hex seed file |
| `KeyEnv` | `string` | Environment variable with hex seed |
| `Role` | `string` | WEB/1 handshake role |
| `MaxStreams` | `uint32` | Max concurrent streams per connection |
| `MaxMessageSize` | `uint32` | Max protocol message size in bytes |
| `TimeoutMs` | `uint32` | Connection idle timeout in ms |
| `Compression` | `string` | Compression algorithm |
| `LogLevel` | `nwep.LogLevel` | Minimum nwep C library log level |

## Logging

velocity uses a structured `Logger` interface compatible with `log/slog`.

```go
// Default logger (slog.Default)
srv, _ := velocity.New(":6937", velocity.WithLogger(velocity.DefaultLogger()))

// Custom slog.Logger
l := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
srv, _ := velocity.New(":6937", velocity.WithLogger(velocity.SlogLogger(l)))
```

Within handlers, use `c.Logger()`:

```go
srv.Handle("/thing", func(c *velocity.Context) error {
    c.Logger().Info("handling request", "path", c.Path(), "peer", c.PeerNodeID())
    return c.OK(nil)
})
```

To forward nwep C library logs through the velocity logger:

```go
velocity.BridgeNWEPLogs(velocity.DefaultLogger())
velocity.SetLogLevel(nwep.LogInfo) // TRACE, DEBUG, INFO, WARN, ERROR
```

Call this once at startup. Only one log callback is active at a time; calling `BridgeNWEPLogs` again replaces the previous one.
