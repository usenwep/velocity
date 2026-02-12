package velocity

import (
	"encoding/json"
	"sync"

	nwep "github.com/usenwep/nwep-go"
)

// Context carries the state for a single request-response cycle. It is passed
// to every HandlerFunc and provides accessors for the request, convenience
// methods for building responses, and a key-value store for sharing data
// between middleware and handlers.
//
// Context instances are pooled and reused across requests. The caller must not
// retain a reference to the Context after the handler returns - doing so leads
// to undefined behavior because the same Context may be recycled for a
// subsequent request.
//
// The Response and Request fields are the underlying nwep types and are
// exported so that handlers requiring low-level protocol access can use them
// directly.
type Context struct {
	// Response is the underlying nwep response writer. Handlers that
	// need direct access to protocol-level response features (such as
	// stream user data or the raw nwep_conn) can use this field.
	Response *nwep.ResponseWriter

	// Request is the underlying nwep request. Handlers that need
	// access to fields not exposed through Context convenience methods
	// can read them directly from this field.
	Request *nwep.Request

	server *Server
	store  map[string]any
}

var ctxPool = sync.Pool{
	New: func() any { return &Context{} },
}

func acquireContext(w *nwep.ResponseWriter, r *nwep.Request, s *Server) *Context {
	c := ctxPool.Get().(*Context)
	c.Response = w
	c.Request = r
	c.server = s
	c.store = nil
	return c
}

func releaseContext(c *Context) {
	c.Response = nil
	c.Request = nil
	c.server = nil
	c.store = nil
	ctxPool.Put(c)
}

// ---------------------------------------------------------------------------
// Request accessors
// ---------------------------------------------------------------------------

// Method returns the WEB/1 request method (e.g. "read", "write", "update",
// "delete"). See the Method* constants for the full set of defined values.
func (c *Context) Method() string { return c.Request.Method }

// Path returns the request path as sent by the client. The path always begins
// with a "/" and is not URL-decoded.
func (c *Context) Path() string { return c.Request.Path }

// Body returns the raw request body as a byte slice. The returned slice is
// valid only for the lifetime of the handler - it must not be retained after
// the handler returns. If the request has no body, Body returns nil.
func (c *Context) Body() []byte { return c.Request.Body }

// Bind deserializes the JSON request body into v using encoding/json. v must
// be a pointer to the target type. This function returns ErrEmptyBody if the
// request body is empty or nil, or a json.UnmarshalError if the body is not
// valid JSON for the target type.
func (c *Context) Bind(v any) error {
	if len(c.Request.Body) == 0 {
		return ErrEmptyBody
	}
	return json.Unmarshal(c.Request.Body, v)
}

// Header returns the value of the request header with the given name. The
// second return value is false if the header is not present. Header names are
// case-sensitive in WEB/1.
func (c *Context) Header(name string) (string, bool) {
	return c.Request.Header(name)
}

// Headers returns all request headers as a slice of nwep.Header. The returned
// slice is valid only for the lifetime of the handler.
func (c *Context) Headers() []nwep.Header {
	return c.Request.Headers()
}

// RequestID returns the 16-byte request identifier assigned by the client.
// Every request carries a unique RequestID that can be used for correlation
// in logs and responses.
func (c *Context) RequestID() [16]byte { return c.Request.RequestID }

// TraceID returns the 16-byte trace identifier for distributed tracing. If
// the client did not set a trace ID, the returned array is all zeros.
func (c *Context) TraceID() [16]byte { return c.Request.TraceID }

// ---------------------------------------------------------------------------
// Identity
// ---------------------------------------------------------------------------

// Conn returns the underlying nwep connection for this request. The returned
// pointer may be nil if the nwep server could not associate the request with
// a tracked connection (e.g. during early handshake states). The caller should
// check for nil before using Conn methods.
func (c *Context) Conn() *nwep.Conn { return c.Request.Conn }

// PeerNodeID returns the 32-byte node ID of the connected peer. If the
// connection is not available or the peer has not completed mutual
// authentication, the returned NodeID is zero-valued. Use NodeID.IsZero to
// check.
func (c *Context) PeerNodeID() nwep.NodeID {
	if c.Request.Conn == nil {
		return nwep.NodeID{}
	}
	_, nid := c.Request.Conn.PeerIdentity()
	return nid
}

// PeerIdentity returns the Ed25519 public key and node ID of the connected
// peer. If the connection is not available or the peer has not completed mutual
// authentication, both values are zero-filled.
func (c *Context) PeerIdentity() ([32]byte, nwep.NodeID) {
	if c.Request.Conn == nil {
		return [32]byte{}, nwep.NodeID{}
	}
	return c.Request.Conn.PeerIdentity()
}

// ---------------------------------------------------------------------------
// Response helpers
// ---------------------------------------------------------------------------

// Respond sends a complete response with the given status and body. The status
// must be one of the Status* constants (e.g. StatusOK, StatusNotFound). body
// may be nil for responses with no payload.
//
// This function returns a non-nil error if the underlying nwep response write
// fails. Only one response may be sent per request - calling Respond (or any
// other response method) more than once is undefined.
func (c *Context) Respond(status string, body []byte) error {
	return c.Response.Respond(status, body)
}

// OK sends a response with status "ok" and the given body. body may be nil.
func (c *Context) OK(body []byte) error {
	return c.Response.Respond(nwep.StatusOK, body)
}

// Created sends a response with status "created" and the given body. body may
// be nil.
func (c *Context) Created(body []byte) error {
	return c.Response.Respond(nwep.StatusCreated, body)
}

// NoContent sends a response with status "no_content" and no body.
func (c *Context) NoContent() error {
	return c.Response.Respond(nwep.StatusNoContent, nil)
}

// JSON marshals v to JSON using encoding/json and sends a response with status
// "ok" and a "content-type: application/json" header. This function returns a
// non-nil error if JSON marshaling fails or the response write fails.
func (c *Context) JSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.Response.SetHeader("content-type", "application/json")
	return c.Response.Respond(nwep.StatusOK, data)
}

// Error sends an error response with an arbitrary status and a plain-text
// message body. The status should be one of the error Status* constants
// (e.g. StatusBadRequest, StatusInternalError).
func (c *Context) Error(status string, msg string) error {
	return c.Response.Respond(status, []byte(msg))
}

// NotFound sends a response with status "not_found" and the given message.
func (c *Context) NotFound(msg string) error {
	return c.Response.Respond(nwep.StatusNotFound, []byte(msg))
}

// BadRequest sends a response with status "bad_request" and the given message.
func (c *Context) BadRequest(msg string) error {
	return c.Response.Respond(nwep.StatusBadRequest, []byte(msg))
}

// Unauthorized sends a response with status "unauthorized" and the given
// message.
func (c *Context) Unauthorized(msg string) error {
	return c.Response.Respond(nwep.StatusUnauthorized, []byte(msg))
}

// Forbidden sends a response with status "forbidden" and the given message.
func (c *Context) Forbidden(msg string) error {
	return c.Response.Respond(nwep.StatusForbidden, []byte(msg))
}

// InternalError sends a response with status "internal_error" and the given
// message. Prefer this over Error(StatusInternalError, msg) for clarity.
func (c *Context) InternalError(msg string) error {
	return c.Response.Respond(nwep.StatusInternalError, []byte(msg))
}

// ---------------------------------------------------------------------------
// Streaming
// ---------------------------------------------------------------------------

// StreamWrite writes data to the current stream. It returns the number of
// bytes written and a non-nil error if the write fails. StreamWrite may be
// called multiple times to send a response incrementally. The caller must call
// StreamClose when finished.
func (c *Context) StreamWrite(data []byte) (int, error) {
	return c.Response.StreamWrite(data)
}

// StreamClose closes the stream with the given error code. Use 0 for a
// graceful close. After StreamClose, no further writes are permitted on this
// stream.
func (c *Context) StreamClose(errCode int) {
	c.Response.StreamClose(errCode)
}

// StreamID returns the numeric identifier for the current stream. Each stream
// within a connection has a unique ID.
func (c *Context) StreamID() int64 {
	return c.Response.StreamID()
}

// IsServerInitiated reports whether this stream was initiated by the server
// (as opposed to being opened by the client request). Server-initiated streams
// are used for push-style notifications.
func (c *Context) IsServerInitiated() bool {
	return c.Response.IsServerInitiated()
}

// ---------------------------------------------------------------------------
// Response building
// ---------------------------------------------------------------------------

// SetHeader sets a response header. This must be called before Write or
// Respond - headers set after the response body is sent are silently dropped.
// Header names are case-sensitive in WEB/1.
func (c *Context) SetHeader(name, value string) {
	c.Response.SetHeader(name, value)
}

// SetStatus sets the response status. This must be called before Write. If
// Respond is used instead, SetStatus is unnecessary because Respond sets the
// status internally.
func (c *Context) SetStatus(status string) {
	c.Response.SetStatus(status)
}

// Write sends the response body. The caller must call SetStatus (and
// optionally SetHeader) before calling Write. For most handlers, the Respond
// or JSON convenience methods are simpler. This function returns a non-nil
// error if the write fails.
func (c *Context) Write(body []byte) error {
	return c.Response.Write(body)
}

// ---------------------------------------------------------------------------
// Key-value store
// ---------------------------------------------------------------------------

// Set stores an arbitrary key-value pair in the context. The store is scoped
// to the current request and is the primary mechanism for passing data between
// middleware and handlers. The store is lazily initialized on first use.
func (c *Context) Set(key string, val any) {
	if c.store == nil {
		c.store = make(map[string]any)
	}
	c.store[key] = val
}

// Get retrieves a value from the context store. The second return value is
// false if the key has not been set. The caller must type-assert the returned
// value to the expected type.
func (c *Context) Get(key string) (any, bool) {
	if c.store == nil {
		return nil, false
	}
	v, ok := c.store[key]
	return v, ok
}

// MustGet retrieves a value from the context store and panics if the key is
// not present. Use this only when the key is guaranteed to have been set by a
// preceding middleware - for example, retrieving a value set by TrustVerify.
func (c *Context) MustGet(key string) any {
	v, ok := c.Get(key)
	if !ok {
		panic("velocity: context key not found: " + key)
	}
	return v
}

// ---------------------------------------------------------------------------
// Server access
// ---------------------------------------------------------------------------

// Server returns the velocity Server that is handling this request. This is
// useful for accessing server-level state such as NodeID or for sending
// notifications to other peers from within a handler.
func (c *Context) Server() *Server { return c.server }

// Logger returns the Logger configured on the server. This is a convenience
// shorthand for c.Server().logger and is the recommended way to emit log
// messages from within a handler.
func (c *Context) Logger() Logger { return c.server.logger }
