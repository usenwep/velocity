# velocity

A WEB/1 server framework for Go, built on [nwep-go](https://github.com/usenwep/nwep-go).

velocity handles routing, middleware, request/response plumbing, peer-to-peer notifications, and server lifecycle so you can focus on your application. Every WEB/1 connection is mutually authenticated with Ed25519, and velocity makes peer identity a first-class concept throughout the handler chain.

nwep-go types (`nwep.Keypair`, `nwep.NodeID`, `nwep.Conn`, `nwep.Settings`) are used directly. Nothing is re-wrapped.

## Installation

Requires __Go 1.25__ or higher.

```sh
go get github.com/usenwep/velocity
```

velocity depends on [nwep-go](https://github.com/usenwep/nwep-go), which needs platform-specific C libraries. After adding velocity to your project, vendor your dependencies and run the nwep-go setup script:

```sh
go mod vendor
cd vendor/github.com/usenwep/nwep-go && bash setup.sh
go build -mod=vendor ./...
```

The setup script downloads pre-built nwep binaries for your platform. It only needs to run once (or again after updating dependencies).

### Makefile for your project

To avoid repeating these steps, drop this into your project's Makefile:

```makefile
NWEP_VENDOR := vendor/github.com/usenwep/nwep-go
STAMP := $(NWEP_VENDOR)/.nwep-setup

build: $(STAMP)
	go build -mod=vendor ./...

vet: $(STAMP)
	go vet -mod=vendor ./...

test: $(STAMP)
	go test -mod=vendor ./...

$(STAMP): go.mod go.sum
	go mod vendor
	cd $(NWEP_VENDOR) && bash setup.sh
	@touch $@

clean:
	rm -rf vendor
```

Then `make build` handles everything. The setup is cached and only re-runs when `go.mod` or `go.sum` change.

## Hello velocity

```go
package main

import (
    "log"

    "github.com/usenwep/velocity"
)

func main() {
    srv, err := velocity.New(":6937")
    if err != nil {
        log.Fatal(err)
    }

    srv.Handle("/hello", func(c *velocity.Context) error {
        return c.OK([]byte("hello from velocity"))
    })

    log.Fatal(srv.Run())
}
```

## Middleware

Middleware wraps handlers to add cross-cutting behavior. Each middleware receives the next handler, does its work, and decides whether to continue the chain.

```go
func timing() velocity.MiddlewareFunc {
    return func(next velocity.HandlerFunc) velocity.HandlerFunc {
        return func(c *velocity.Context) error {
            start := time.Now()
            err := next(c)
            c.Logger().Info("request",
                "path", c.Path(),
                "duration", time.Since(start),
            )
            return err
        }
    }
}

srv.Use(timing())
```

### Built-in middleware

- `Recover()` catches panics and responds with `internal_error`
- `RequestLogger()` logs method, path, peer, and duration for every request
- `RequirePeer()` rejects unauthenticated peers
- `AllowPeers(ids...)` restricts access to specific node IDs
- `MethodFilter(methods...)` restricts allowed request methods

```go
srv.Use(velocity.Recover(), velocity.RequestLogger())
```

## Routing

The router supports exact matches, method-specific matches, prefix matches, and route groups.

```go
// Any method on /health
srv.Handle("/health", healthHandler)

// Method-specific
srv.Router().Read("/users", listUsers)
srv.Router().Write("/users", createUser)

// Prefix match (longest prefix wins)
srv.Router().HandlePrefix("/static/", fileHandler)

// Not found
srv.Router().SetNotFound(func(c *velocity.Context) error {
    return c.NotFound("nothing here")
})
```

### Groups

Groups share a path prefix and middleware. They nest arbitrarily.

```go
api := srv.Group("/api/v1")
api.Read("/messages", listMessages)
api.Write("/messages", createMessage)

admin := api.Group("/admin", velocity.AllowPeers(trustedPeer))
admin.Handle("/stats", statsHandler)
// registers at /api/v1/admin/stats with AllowPeers middleware
```

## Context

Each handler receives a `*Context` with accessors for the request, helpers for building responses, and a key-value store for passing data between middleware.

```go
srv.Handle("/echo", func(c *velocity.Context) error {
    c.Logger().Info("echo",
        "method", c.Method(),
        "peer", c.PeerNodeID(),
    )
    return c.OK(c.Body())
})
```

The underlying `nwep.Request` and `nwep.ResponseWriter` are available as `c.Request` and `c.Response` for anything the helpers don't cover.

## Notifications

Servers can push notifications to connected peers at any time, not just during request handling.

```go
srv.NotifyAll("update", "/feed", []byte(`{"new_items": 3}`))
srv.NotifyJSON(peerID, "welcome", "/", map[string]string{"msg": "hello"})
```

## Documentation

- [Usage Guide](docs/USAGE.md) covers routing, middleware, context, notifications, lifecycle, trust, and configuration in depth.
- [Error Handling](docs/ERRORS.md) covers error patterns, sentinel errors, and recovery.

## Related

- [nwep-go](https://github.com/usenwep/nwep-go) - Go bindings for libnwep
- [WEB/1 Specification](https://github.com/usenwep/spec) - The WEB/1 protocol specification

## Acknowledgments

The structure and tone of this documentation were inspired by [Koa](https://github.com/koajs/koa).

## License

[MIT](LICENSE)
