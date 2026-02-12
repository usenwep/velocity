package velocity

import "strings"

// combineMW returns a new slice containing the elements of a followed by b.
// It always allocates a fresh backing array so that appending to the result
// cannot mutate either input slice.
func combineMW(a, b []MiddlewareFunc) []MiddlewareFunc {
	combined := make([]MiddlewareFunc, len(a)+len(b))
	copy(combined, a)
	copy(combined[len(a):], b)
	return combined
}

type route struct {
	handler    HandlerFunc
	middleware []MiddlewareFunc
}

// Router maps request paths (and optionally methods) to handlers. It supports
// three kinds of routes, checked in the following order:
//
//  1. Method-specific exact match - registered with Router.Method or the
//     convenience methods Read, Write, Update, Delete. The route matches only
//     when both the path and the method are equal.
//
//  2. Path-only exact match - registered with Router.Handle. The route matches
//     any method for the given path.
//
//  3. Prefix match - registered with Router.HandlePrefix. When multiple prefix
//     routes match, the longest prefix wins.
//
// If no route matches, the not-found handler set by SetNotFound is called. If
// no not-found handler has been set, the server returns a "not_found" response
// with the body "not found".
//
// Router is not safe for concurrent use during registration. All routes should
// be registered before the server is started. After startup, route lookup
// (Find) is safe for concurrent use.
type Router struct {
	exact    map[string]*route
	prefixes []prefixRoute
	notFound HandlerFunc
}

type prefixRoute struct {
	prefix string
	route  *route
}

// NewRouter creates a new empty Router. In most cases the caller does not need
// to call this directly - Server creates a Router internally that is accessible
// via Server.Router.
func NewRouter() *Router {
	return &Router{
		exact: make(map[string]*route),
	}
}

// Handle registers h for the given path, matching all request methods.
// Optional middleware mw is applied to this route only, after global
// middleware. If a handler is already registered for path, it is replaced.
func (rt *Router) Handle(path string, h HandlerFunc, mw ...MiddlewareFunc) {
	rt.exact[path] = &route{handler: h, middleware: mw}
}

// Method registers h for a specific method and path combination. Optional
// middleware mw is applied to this route only. Method-specific routes take
// precedence over path-only routes registered with Handle.
func (rt *Router) Method(method, path string, h HandlerFunc, mw ...MiddlewareFunc) {
	key := method + " " + path
	rt.exact[key] = &route{handler: h, middleware: mw}
}

// Read registers h for MethodRead ("read") on the given path. It is a
// convenience shorthand for rt.Method(MethodRead, path, h, mw...).
func (rt *Router) Read(path string, h HandlerFunc, mw ...MiddlewareFunc) {
	rt.Method(MethodRead, path, h, mw...)
}

// Write registers h for MethodWrite ("write") on the given path. It is a
// convenience shorthand for rt.Method(MethodWrite, path, h, mw...).
func (rt *Router) Write(path string, h HandlerFunc, mw ...MiddlewareFunc) {
	rt.Method(MethodWrite, path, h, mw...)
}

// Update registers h for MethodUpdate ("update") on the given path. It is a
// convenience shorthand for rt.Method(MethodUpdate, path, h, mw...).
func (rt *Router) Update(path string, h HandlerFunc, mw ...MiddlewareFunc) {
	rt.Method(MethodUpdate, path, h, mw...)
}

// Delete registers h for MethodDelete ("delete") on the given path. It is a
// convenience shorthand for rt.Method(MethodDelete, path, h, mw...).
func (rt *Router) Delete(path string, h HandlerFunc, mw ...MiddlewareFunc) {
	rt.Method(MethodDelete, path, h, mw...)
}

// HandlePrefix registers h for all paths that begin with prefix. When multiple
// prefix routes match a request, the route with the longest matching prefix is
// selected. Optional middleware mw is applied to this route only.
//
// Prefix routes are checked after all exact routes. Use this for catch-all
// handlers or subtree delegation.
func (rt *Router) HandlePrefix(prefix string, h HandlerFunc, mw ...MiddlewareFunc) {
	rt.prefixes = append(rt.prefixes, prefixRoute{
		prefix: prefix,
		route:  &route{handler: h, middleware: mw},
	})
}

// SetNotFound sets the handler that is called when no registered route matches
// the request path. If not set, the server responds with status "not_found"
// and the body "not found". The not-found handler receives global middleware
// but no route-level middleware.
func (rt *Router) SetNotFound(h HandlerFunc) {
	rt.notFound = h
}

// Group creates a new route group that shares the given path prefix and
// middleware. All routes registered through the group are prefixed with prefix,
// and the group's middleware runs after global middleware but before any
// route-level middleware.
//
// Groups can be nested: a sub-group inherits its parent's prefix and
// middleware.
func (rt *Router) Group(prefix string, mw ...MiddlewareFunc) *Group {
	return &Group{
		prefix:     prefix,
		router:     rt,
		middleware: mw,
	}
}

// Find looks up a handler for the given path and method, composing globalMW
// and any route-level middleware around the matched handler. Find returns nil
// if no route matches and no not-found handler is set.
//
// The lookup order is: method-specific exact match, then path-only exact
// match, then longest prefix match, then the not-found handler.
func (rt *Router) Find(path, method string, globalMW []MiddlewareFunc) HandlerFunc {
	// Try method-specific exact match first.
	if r, ok := rt.exact[method+" "+path]; ok {
		return applyMiddleware(r.handler, combineMW(globalMW, r.middleware))
	}
	// Try path-only exact match.
	if r, ok := rt.exact[path]; ok {
		return applyMiddleware(r.handler, combineMW(globalMW, r.middleware))
	}
	// Try prefix match (longest prefix wins).
	var best *route
	bestLen := 0
	for _, pr := range rt.prefixes {
		if strings.HasPrefix(path, pr.prefix) && len(pr.prefix) > bestLen {
			best = pr.route
			bestLen = len(pr.prefix)
		}
	}
	if best != nil {
		return applyMiddleware(best.handler, combineMW(globalMW, best.middleware))
	}
	// Not found handler.
	if rt.notFound != nil {
		return applyMiddleware(rt.notFound, globalMW)
	}
	return nil
}

// Group is a collection of routes that share a common path prefix and
// middleware. Routes registered on a Group are prefixed with the group's prefix
// and wrapped with the group's middleware (which runs after global middleware
// but before any per-route middleware).
//
// Groups are created with Router.Group or Server.Group and can be nested
// arbitrarily:
//
//	api := srv.Group("/api/v1")
//	api.Read("/users", listUsers)
//
//	admin := api.Group("/admin", velocity.RequirePeer())
//	admin.Handle("/shutdown", shutdownHandler)
type Group struct {
	prefix     string
	router     *Router
	middleware []MiddlewareFunc
}

// Handle registers h for the given path (prefixed by the group prefix),
// matching all request methods. Optional middleware mw is applied after the
// group's middleware.
func (g *Group) Handle(path string, h HandlerFunc, mw ...MiddlewareFunc) {
	g.router.Handle(g.prefix+path, h, combineMW(g.middleware, mw)...)
}

// Method registers h for a specific method and path (prefixed by the group
// prefix). Optional middleware mw is applied after the group's middleware.
func (g *Group) Method(method, path string, h HandlerFunc, mw ...MiddlewareFunc) {
	g.router.Method(method, g.prefix+path, h, combineMW(g.middleware, mw)...)
}

// Read registers h for MethodRead on the given path within the group.
func (g *Group) Read(path string, h HandlerFunc, mw ...MiddlewareFunc) {
	g.Method(MethodRead, path, h, mw...)
}

// Write registers h for MethodWrite on the given path within the group.
func (g *Group) Write(path string, h HandlerFunc, mw ...MiddlewareFunc) {
	g.Method(MethodWrite, path, h, mw...)
}

// Update registers h for MethodUpdate on the given path within the group.
func (g *Group) Update(path string, h HandlerFunc, mw ...MiddlewareFunc) {
	g.Method(MethodUpdate, path, h, mw...)
}

// Delete registers h for MethodDelete on the given path within the group.
func (g *Group) Delete(path string, h HandlerFunc, mw ...MiddlewareFunc) {
	g.Method(MethodDelete, path, h, mw...)
}

// HandlePrefix registers h for all paths beginning with prefix, prepended
// with the group's prefix. Optional middleware mw is applied after the group's
// middleware.
func (g *Group) HandlePrefix(prefix string, h HandlerFunc, mw ...MiddlewareFunc) {
	g.router.HandlePrefix(g.prefix+prefix, h, combineMW(g.middleware, mw)...)
}

// Group creates a sub-group that inherits this group's prefix and middleware.
// The sub-group's prefix is appended to the parent prefix, and the sub-group's
// middleware runs after the parent group's middleware.
func (g *Group) Group(prefix string, mw ...MiddlewareFunc) *Group {
	return &Group{
		prefix:     g.prefix + prefix,
		router:     g.router,
		middleware: combineMW(g.middleware, mw),
	}
}
