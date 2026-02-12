// Package velocity is a server library for the WEB/1 protocol built on top of
// nwep-go. It provides routing, middleware, request context, and lifecycle
// management in the style Go developers expect from HTTP frameworks like Echo
// or Chi, while keeping the underlying nwep-go types (nwep.Keypair,
// nwep.NodeID, nwep.Conn, etc.) fully transparent - nothing is re-wrapped.
//
// A minimal server:
//
//	srv, err := velocity.New(":6937")
//	if err != nil { log.Fatal(err) }
//	srv.Handle("/hello", func(c *velocity.Context) error {
//	    return c.OK([]byte("hello from velocity"))
//	})
//	log.Fatal(srv.Run())
//
// Handlers receive a *Context that exposes the request, provides response
// helpers (OK, JSON, NotFound, etc.), and carries a key-value store for
// sharing state between middleware and handlers. Middleware is composable
// and follows the func(next HandlerFunc) HandlerFunc pattern.
//
// velocity has zero external dependencies beyond nwep-go.
package velocity

import (
	"fmt"
	"net"
	"sync"
	"time"

	nwep "github.com/usenwep/nwep-go"
)

var initOnce sync.Once

// HandlerFunc is the signature for velocity request handlers. The handler
// receives a Context containing the request and response writer, and returns
// an error. A non-nil error is logged by the server but does not automatically
// generate a response - the handler is responsible for sending a response
// before returning. If the handler panics and the Recover middleware is
// installed, the panic is caught and an "internal_error" response is sent.
type HandlerFunc func(c *Context) error

// Option configures a Server during construction. Options are passed to New
// and are applied in order. If an Option returns a non-nil error, New fails
// immediately with that error, allowing eager validation of configuration
// (e.g. loading a key file or building a trust store).
type Option func(*Server) error

// Server is a WEB/1 server that manages routing, middleware, connection
// lifecycle, and notifications. It owns an underlying nwep.Server and a Router.
//
// A Server is created with New, configured with Option functions, and started
// with either Run (blocking) or Start (non-blocking). Call Shutdown to stop
// the server gracefully.
//
// The Server's Router, middleware, and options must be configured before
// calling Run or Start. After startup, only the notification methods (Notify,
// NotifyAll, etc.) and read-only accessors (NodeID, Addr, URL, etc.) are safe
// to call concurrently.
type Server struct {
	addr     string
	keypair  *nwep.Keypair
	settings *nwep.Settings
	logger   Logger
	router   *Router
	mw       []MiddlewareFunc

	nwep *nwep.Server

	logServer    *nwep.LogServer
	anchorServer *nwep.AnchorServer

	onConnect    func(*nwep.Conn)
	onDisconnect func(*nwep.Conn, int)
	onStart      []func(*Server)
	onShutdown   []func(*Server)

	trustStore *nwep.TrustStore
}

// New creates a new velocity Server that will listen on addr (in "host:port"
// format). The nwep library is initialized automatically via sync.Once on the
// first call to New.
//
// Options are applied in order. If no keypair option is provided (WithKeypair,
// WithKeyFile, WithKeyEnv, or WithConfig with a key field), a random Ed25519
// keypair is generated. The default logger is slog.Default.
//
// This function returns a non-nil error if nwep initialization fails, if any
// option returns an error, or if keypair generation fails.
func New(addr string, opts ...Option) (*Server, error) {
	var initErr error
	initOnce.Do(func() { initErr = nwep.Init() })
	if initErr != nil {
		return nil, fmt.Errorf("velocity: nwep init: %w", initErr)
	}

	s := &Server{
		addr:   addr,
		logger: DefaultLogger(),
		router: NewRouter(),
	}

	for _, opt := range opts {
		if err := opt(s); err != nil {
			return nil, fmt.Errorf("velocity: option: %w", err)
		}
	}

	if s.keypair == nil {
		kp, err := nwep.GenerateKeypair()
		if err != nil {
			return nil, fmt.Errorf("velocity: generate keypair: %w", err)
		}
		s.keypair = kp
	}

	return s, nil
}

// Router returns the server's Router for direct route registration. In most
// cases the convenience methods on Server (Handle, Group, etc.) are
// sufficient; Router is exposed for advanced use cases such as setting a
// custom not-found handler or inspecting registered routes.
func (s *Server) Router() *Router { return s.router }

// Use appends one or more global middleware to the server. Global middleware
// runs on every request, in registration order, before any route-level or
// group-level middleware. Use must be called before Run or Start.
func (s *Server) Use(mw ...MiddlewareFunc) { s.mw = append(s.mw, mw...) }

// Handle registers h on the server's Router for the given path, matching all
// request methods. Optional middleware mw is applied to this route only, after
// global middleware. This is a convenience shorthand for
// s.Router().Handle(path, h, mw...).
func (s *Server) Handle(path string, h HandlerFunc, mw ...MiddlewareFunc) {
	s.router.Handle(path, h, mw...)
}

// Group creates a route group on the server's Router with the given path
// prefix and optional middleware. This is a convenience shorthand for
// s.Router().Group(prefix, mw...).
func (s *Server) Group(prefix string, mw ...MiddlewareFunc) *Group {
	return s.router.Group(prefix, mw...)
}

// Run starts the server and blocks until Shutdown is called or a termination
// signal (SIGINT, SIGTERM) is received. It is equivalent to calling Start
// followed by waiting for the underlying nwep event loop to exit.
//
// This function returns a non-nil error if the server fails to start (e.g.
// address already in use). After a successful start, Run blocks indefinitely
// and returns nil on clean shutdown.
func (s *Server) Run() error {
	if err := s.Start(); err != nil {
		return err
	}
	// Block on the underlying nwep server's Run (which blocks until shutdown).
	return s.nwep.Run()
}

// Start creates the underlying nwep.Server, binds to the configured address,
// and fires OnStart callbacks, but does not block. The caller must eventually
// call Shutdown to release resources, and must call nwep.Server.Run (via
// NWEPServer().Run()) or Server.Run to actually process packets.
//
// For most use cases, prefer Run which combines Start and the event loop.
// Start is provided for scenarios that require non-blocking initialization
// (e.g. obtaining the resolved address before entering the event loop).
//
// This function returns a non-nil error if the nwep server cannot be created
// (e.g. invalid address, socket error, or key error).
func (s *Server) Start() error {
	handler := s.buildHandler()

	var nwepOpts []nwep.ServerOption
	if s.settings != nil {
		nwepOpts = append(nwepOpts, nwep.WithSettings(*s.settings))
	}
	if s.onConnect != nil {
		nwepOpts = append(nwepOpts, nwep.WithOnConnect(s.onConnect))
	}
	if s.onDisconnect != nil {
		nwepOpts = append(nwepOpts, nwep.WithOnDisconnect(s.onDisconnect))
	}

	srv, err := nwep.NewServer(s.addr, s.keypair, handler, nwepOpts...)
	if err != nil {
		return fmt.Errorf("velocity: start server: %w", err)
	}
	s.nwep = srv

	if s.logServer != nil {
		s.nwep.SetLogServer(s.logServer)
	}
	if s.anchorServer != nil {
		s.nwep.SetAnchorServer(s.anchorServer)
	}

	for _, fn := range s.onStart {
		fn(s)
	}

	return nil
}

// Shutdown gracefully stops the server. It fires OnShutdown callbacks, closes
// all connections, and frees the underlying nwep server and trust store. After
// Shutdown returns, the Server must not be reused.
//
// Shutdown is safe to call on a server that has not been started - it is a
// no-op in that case.
func (s *Server) Shutdown() {
	if s.nwep == nil {
		return
	}
	for _, fn := range s.onShutdown {
		fn(s)
	}
	s.nwep.Shutdown()
	if s.logServer != nil {
		s.logServer.Free()
		s.logServer = nil
	}
	if s.anchorServer != nil {
		s.anchorServer.Free()
		s.anchorServer = nil
	}
	if s.trustStore != nil {
		s.trustStore.Free()
		s.trustStore = nil
	}
}

// NodeID returns the server's 32-byte node ID, derived from its Ed25519
// keypair. This is available immediately after New - it does not require the
// server to be started.
func (s *Server) NodeID() nwep.NodeID {
	if s.nwep != nil {
		return s.nwep.NodeID()
	}
	nid, _ := s.keypair.NodeID()
	return nid
}

// URL returns the WEB/1 URL for the given path on this server. The URL
// includes the server's IP address, port, and node ID in the standard WEB/1
// format: web://[Base58(IP||NodeID)]:port/path.
//
// This function returns an empty string if the server has not been started
// (the listen address is not yet known).
func (s *Server) URL(path string) string {
	if s.nwep != nil {
		return s.nwep.URL(path)
	}
	return ""
}

// Addr returns the server's resolved listen address as a net.Addr. This is
// particularly useful when binding to port 0 to discover the assigned port.
// It returns nil if the server has not been started.
func (s *Server) Addr() net.Addr {
	if s.nwep != nil {
		return s.nwep.Addr()
	}
	return nil
}

// NWEPServer returns the underlying nwep.Server for advanced usage that is not
// covered by the velocity API. The returned pointer is nil if the server has
// not been started. The caller must not call Shutdown on the returned server
// directly - use Server.Shutdown instead.
func (s *Server) NWEPServer() *nwep.Server { return s.nwep }

// buildHandler converts the velocity router and middleware chain into a single
// nwep.HandlerFunc suitable for nwep.NewServer. Each inbound request acquires
// a pooled Context, performs route lookup with middleware composition, invokes
// the matched handler, and releases the Context.
func (s *Server) buildHandler() nwep.HandlerFunc {
	return func(w *nwep.ResponseWriter, r *nwep.Request) {
		c := acquireContext(w, r, s)
		defer releaseContext(c)

		h := s.router.Find(r.Path, r.Method, s.mw)
		if h == nil {
			_ = c.NotFound("not found")
			return
		}
		if err := h(c); err != nil {
			s.logger.Error("handler error",
				"path", r.Path,
				"method", r.Method,
				"error", err.Error(),
			)
		}
	}
}

func nowNanos() uint64 {
	return uint64(time.Now().UnixNano())
}

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

// WithKeypair sets the server's Ed25519 keypair directly. The keypair is used
// for the server's identity (node ID) and for the mutual authentication
// handshake with connecting peers. kp must not be nil.
func WithKeypair(kp *nwep.Keypair) Option {
	return func(s *Server) error {
		s.keypair = kp
		return nil
	}
}

// WithKeyFile loads (or generates and saves) a keypair from the file at path.
// See LoadOrGenerateKeypair for the file format and creation behavior. This
// option returns an error if the file exists but cannot be read or contains an
// invalid seed.
func WithKeyFile(path string) Option {
	return func(s *Server) error {
		kp, err := LoadOrGenerateKeypair(path)
		if err != nil {
			return err
		}
		s.keypair = kp
		return nil
	}
}

// WithKeyEnv loads a keypair from the hex-encoded seed stored in the
// environment variable named by envVar. This option returns an error if the
// variable is not set or the seed is malformed.
func WithKeyEnv(envVar string) Option {
	return func(s *Server) error {
		kp, err := KeypairFromEnv(envVar)
		if err != nil {
			return err
		}
		s.keypair = kp
		return nil
	}
}

// WithSettings sets the nwep transport-level settings for the server. Fields
// include MaxStreams, MaxMessageSize, TimeoutMs, Compression, and Role. See
// nwep.Settings for defaults and valid ranges.
func WithSettings(settings nwep.Settings) Option {
	return func(s *Server) error {
		s.settings = &settings
		return nil
	}
}

// WithLogger sets the Logger used by the server, middleware, and handler error
// reporting. If not set, DefaultLogger (backed by slog.Default) is used.
// l must not be nil.
func WithLogger(l Logger) Option {
	return func(s *Server) error {
		s.logger = l
		return nil
	}
}

// WithOnConnect registers a callback that is invoked when a new peer
// connection is established, after the mutual authentication handshake
// completes. The callback receives the nwep.Conn for the new connection.
// Only one OnConnect callback can be active - setting a new one replaces the
// previous.
func WithOnConnect(fn func(*nwep.Conn)) Option {
	return func(s *Server) error {
		s.onConnect = fn
		return nil
	}
}

// WithOnDisconnect registers a callback that is invoked when a peer connection
// is closed. The callback receives the nwep.Conn and the error code (0 for
// graceful close). Only one OnDisconnect callback can be active.
func WithOnDisconnect(fn func(*nwep.Conn, int)) Option {
	return func(s *Server) error {
		s.onDisconnect = fn
		return nil
	}
}

// WithTrust configures the server's trust store for identity verification.
// The TrustConfig is built eagerly - if construction fails (e.g. bad anchor
// key), this option returns an error and New fails. The Server takes ownership
// of the resulting TrustStore and frees it on Shutdown.
//
// After constructing the server with WithTrust, use the TrustVerify middleware
// to perform per-request identity verification.
func WithTrust(tc *TrustConfig) Option {
	return func(s *Server) error {
		ts, err := tc.Build()
		if err != nil {
			return fmt.Errorf("velocity: build trust store: %w", err)
		}
		s.trustStore = ts
		return nil
	}
}

// WithRole sets the server's advertised role in the WEB/1 handshake. Common
// values are "regular", "log_server", and "anchor". If WithSettings is also
// used, WithRole should be applied after WithSettings to avoid being
// overwritten.
func WithRole(role string) Option {
	return func(s *Server) error {
		if s.settings == nil {
			s.settings = &nwep.Settings{}
		}
		s.settings.Role = role
		return nil
	}
}

// WithLogServer attaches a pre-created nwep.LogServer. Requests to /log and
// /log/* are routed directly to its HandleRequest, bypassing velocity's router.
// The Server takes ownership and frees the LogServer on Shutdown.
func WithLogServer(ls *nwep.LogServer) Option {
	return func(s *Server) error {
		s.logServer = ls
		return nil
	}
}

// WithAnchorServer attaches a pre-created nwep.AnchorServer. Requests to
// /checkpoint and /checkpoint/* are routed directly to its HandleRequest,
// bypassing velocity's router. The Server takes ownership and frees the
// AnchorServer on Shutdown.
func WithAnchorServer(as *nwep.AnchorServer) Option {
	return func(s *Server) error {
		s.anchorServer = as
		return nil
	}
}

// LogServer returns the attached LogServer, or nil if none was configured.
func (s *Server) LogServer() *nwep.LogServer { return s.logServer }

// AnchorServer returns the attached AnchorServer, or nil if none was configured.
func (s *Server) AnchorServer() *nwep.AnchorServer { return s.anchorServer }

// WithConfig applies a Config struct to the server. This is a convenience for
// declarative configuration - see Config for the available fields and their
// behavior. Fields with zero values are ignored.
func WithConfig(cfg *Config) Option {
	return func(s *Server) error {
		return cfg.Apply(s)
	}
}

// OnStart registers a callback that is invoked after the underlying nwep
// server is created and bound to its listen address, but before the event loop
// begins processing packets. Multiple OnStart callbacks can be registered and
// are called in registration order.
//
// This is a good place to log the server's resolved address and URL:
//
//	velocity.OnStart(func(s *velocity.Server) {
//	    log.Printf("listening %s - %s", s.Addr(), s.URL("/"))
//	})
func OnStart(fn func(*Server)) Option {
	return func(s *Server) error {
		s.onStart = append(s.onStart, fn)
		return nil
	}
}

// OnShutdown registers a callback that is invoked when Shutdown is called,
// before the underlying nwep server is closed. Multiple OnShutdown callbacks
// can be registered and are called in registration order. Use this for cleanup
// tasks such as flushing logs or closing database connections.
func OnShutdown(fn func(*Server)) Option {
	return func(s *Server) error {
		s.onShutdown = append(s.onShutdown, fn)
		return nil
	}
}
