package velocity

import (
	"encoding/json"

	nwep "github.com/usenwep/nwep-go"
)

// Notify sends a server-initiated notification to a specific peer. The
// notification is delivered as a WEB/1 NOTIFY message with the given event
// name, path, and body.
//
// peer is the 32-byte node ID of the target peer. The peer must be currently
// connected - if it is not, the notification is silently dropped by the
// underlying nwep server. event is an application-defined event name (e.g.
// "update", "delete"). path identifies the resource the event relates to.
// body may be nil for events that carry no payload.
//
// This function returns ErrServerNotRunning if the server has not been started,
// or a non-nil error if the underlying nwep notification fails.
func (s *Server) Notify(peer nwep.NodeID, event, path string, body []byte) error {
	if s.nwep == nil {
		return ErrServerNotRunning
	}
	return s.nwep.Notify(peer, event, path, body)
}

// NotifyWithOptions sends a notification to a specific peer with additional
// protocol options such as custom headers or a caller-supplied notify ID.
//
// opts must not be nil. See nwep.NotifyOptions for the available fields. This
// function returns ErrServerNotRunning if the server has not been started.
func (s *Server) NotifyWithOptions(peer nwep.NodeID, event, path string, body []byte, opts *nwep.NotifyOptions) error {
	if s.nwep == nil {
		return ErrServerNotRunning
	}
	return s.nwep.NotifyWithOptions(peer, event, path, body, opts)
}

// NotifyAll broadcasts a notification to every currently connected peer. The
// notification is delivered as a WEB/1 NOTIFY message with the given event
// name, path, and body. body may be nil.
//
// If the server has not been started, NotifyAll is a no-op.
func (s *Server) NotifyAll(event, path string, body []byte) {
	if s.nwep == nil {
		return
	}
	s.nwep.NotifyAll(event, path, body)
}

// NotifyJSON marshals v to JSON and sends the result as a notification to the
// specified peer. This is a convenience wrapper around Notify.
//
// This function returns a non-nil error if JSON marshaling fails, if the
// server has not been started, or if the underlying notification fails.
func (s *Server) NotifyJSON(peer nwep.NodeID, event, path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.Notify(peer, event, path, data)
}

// NotifyAllJSON marshals v to JSON and broadcasts the result to all connected
// peers. This is a convenience wrapper around NotifyAll.
//
// This function returns a non-nil error if JSON marshaling fails. If the
// server has not been started, the broadcast is silently skipped.
func (s *Server) NotifyAllJSON(event, path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	s.NotifyAll(event, path, data)
	return nil
}

// ConnectionCount returns the number of active peer connections. If the server
// has not been started, it returns 0.
func (s *Server) ConnectionCount() int {
	if s.nwep == nil {
		return 0
	}
	return s.nwep.ConnectionCount()
}

// ConnectedPeers returns the node IDs of all currently connected peers. The
// returned slice is a snapshot - it may become stale as peers connect and
// disconnect. If the server has not been started, it returns nil.
func (s *Server) ConnectedPeers() []nwep.NodeID {
	if s.nwep == nil {
		return nil
	}
	return s.nwep.ConnectedPeers()
}
