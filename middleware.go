package velocity

import (
	"fmt"
	"time"

	nwep "github.com/usenwep/nwep-go"
)

// MiddlewareFunc is a function that wraps a HandlerFunc to add cross-cutting
// behavior such as logging, authentication, or panic recovery. Middleware
// receives the next handler in the chain and returns a new handler that
// typically calls next after performing its work:
//
//	func MyMiddleware() velocity.MiddlewareFunc {
//	    return func(next velocity.HandlerFunc) velocity.HandlerFunc {
//	        return func(c *velocity.Context) error {
//	            // before
//	            err := next(c)
//	            // after
//	            return err
//	        }
//	    }
//	}
//
// Middleware is applied in registration order: the first middleware passed to
// Server.Use is the outermost wrapper and executes first on the way in and
// last on the way out.
type MiddlewareFunc func(next HandlerFunc) HandlerFunc

// applyMiddleware composes a slice of middleware around a handler. The first
// element of mw becomes the outermost wrapper. If mw is empty, h is returned
// unchanged.
func applyMiddleware(h HandlerFunc, mw []MiddlewareFunc) HandlerFunc {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}

// Recover returns middleware that catches panics in downstream handlers and
// converts them to an "internal_error" response. The panic value and the
// request path are logged at error level through the server's Logger.
//
// Recover should be the first middleware in the chain (registered first with
// Server.Use) so that it catches panics from all subsequent middleware and
// handlers.
func Recover() MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) (err error) {
			defer func() {
				if r := recover(); r != nil {
					c.Logger().Error("panic recovered", "panic", fmt.Sprint(r), "path", c.Path())
					err = c.InternalError("internal error")
				}
			}()
			return next(c)
		}
	}
}

// RequestLogger returns middleware that logs every completed request. Each log
// entry includes the method, path, peer node ID, and wall-clock duration. The
// entry is emitted at info level after the downstream handler returns,
// regardless of whether the handler returned an error.
func RequestLogger() MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			start := time.Now()
			err := next(c)
			dur := time.Since(start)
			peer := c.PeerNodeID()
			c.Logger().Info("request",
				"method", c.Method(),
				"path", c.Path(),
				"peer", peer.String(),
				"duration", dur.String(),
			)
			return err
		}
	}
}

// RequirePeer returns middleware that rejects requests from peers whose node ID
// is zero-valued, meaning they have not completed mutual authentication. When
// rejected, the peer receives an "unauthorized" response with the message
// "peer identity required".
//
// This middleware should be placed after Recover (if used) so that panics are
// still caught.
func RequirePeer() MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			if c.PeerNodeID().IsZero() {
				return c.Unauthorized("peer identity required")
			}
			return next(c)
		}
	}
}

// AllowPeers returns middleware that restricts access to the specified node
// IDs. Requests from any peer not in the allowed set receive a "forbidden"
// response with the message "peer not allowed". The allowed set is built once
// at middleware creation time and is safe for concurrent use.
//
// AllowPeers implicitly requires that the peer is authenticated - a
// zero-valued node ID will never match the allowed set.
func AllowPeers(allowed ...nwep.NodeID) MiddlewareFunc {
	set := make(map[nwep.NodeID]struct{}, len(allowed))
	for _, id := range allowed {
		set[id] = struct{}{}
	}
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			peer := c.PeerNodeID()
			if _, ok := set[peer]; !ok {
				return c.Forbidden("peer not allowed")
			}
			return next(c)
		}
	}
}

// MethodFilter returns middleware that only permits the specified request
// methods. Requests with any other method receive a "bad_request" response
// with the message "method not allowed". The allowed set is built once at
// middleware creation time.
//
// For simple cases where a handler only serves a single method, prefer the
// convenience registration methods (Router.Read, Router.Write, etc.) which
// combine route registration with method filtering.
func MethodFilter(methods ...string) MiddlewareFunc {
	set := make(map[string]struct{}, len(methods))
	for _, m := range methods {
		set[m] = struct{}{}
	}
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			if _, ok := set[c.Method()]; !ok {
				return c.Error(nwep.StatusBadRequest, "method not allowed")
			}
			return next(c)
		}
	}
}
