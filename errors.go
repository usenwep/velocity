package velocity

import "errors"

// Sentinel errors returned by velocity functions.
//
// These errors use the standard errors package and can be matched with
// errors.Is. All velocity errors are prefixed with "velocity:" for
// clear attribution in wrapped error chains.
var (
	// ErrEmptyBody is returned by Context.Bind when the request body
	// is empty or nil. The caller should check for a body before
	// attempting to decode, or handle this error to send an appropriate
	// bad_request response.
	ErrEmptyBody = errors.New("velocity: empty body")

	// ErrServerNotRunning is returned by notification methods (Notify,
	// NotifyAll, and their JSON variants) when called on a Server that
	// has not been started with Start or Run, or has already been shut
	// down. The caller must ensure the server is running before sending
	// notifications.
	ErrServerNotRunning = errors.New("velocity: server not running")
)
