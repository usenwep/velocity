package velocity

import nwep "github.com/usenwep/nwep-go"

// WEB/1 response status constants, re-exported from nwep for convenience.
//
// These are string constants used with Context.Respond, Context.Error,
// and Context.SetStatus. They correspond to the status values defined in
// the WEB/1 specification. Success statuses can be checked with
// nwep.StatusIsSuccess; error statuses with nwep.StatusIsError.
const (
	// StatusOK indicates the request was processed successfully. This
	// is the default status used by Context.OK and Context.JSON.
	StatusOK = nwep.StatusOK

	// StatusCreated indicates a new resource was created as a result
	// of the request. Typically returned from write handlers.
	StatusCreated = nwep.StatusCreated

	// StatusAccepted indicates the request has been accepted for
	// processing, but processing has not completed. Useful for
	// asynchronous operations.
	StatusAccepted = nwep.StatusAccepted

	// StatusNoContent indicates the request succeeded but there is no
	// response body. This is the status used by Context.NoContent.
	StatusNoContent = nwep.StatusNoContent

	// StatusBadRequest indicates the request was malformed or contained
	// invalid parameters. Used by Context.BadRequest.
	StatusBadRequest = nwep.StatusBadRequest

	// StatusUnauthorized indicates the peer has not provided valid
	// authentication credentials. Used by Context.Unauthorized and the
	// RequirePeer middleware.
	StatusUnauthorized = nwep.StatusUnauthorized

	// StatusForbidden indicates the peer is authenticated but lacks
	// permission for the requested operation. Used by Context.Forbidden
	// and the AllowPeers middleware.
	StatusForbidden = nwep.StatusForbidden

	// StatusNotFound indicates no handler matched the request path.
	// Used by Context.NotFound and the default not-found handler.
	StatusNotFound = nwep.StatusNotFound

	// StatusConflict indicates the request conflicts with the current
	// state of the resource (e.g. a duplicate write).
	StatusConflict = nwep.StatusConflict

	// StatusRateLimited indicates the peer has exceeded the allowed
	// request rate. The peer should back off before retrying.
	StatusRateLimited = nwep.StatusRateLimited

	// StatusInternalError indicates an unexpected server-side failure.
	// Used by Context.InternalError and the Recover middleware when a
	// handler panics.
	StatusInternalError = nwep.StatusInternalError

	// StatusUnavailable indicates the server is temporarily unable to
	// handle the request (e.g. during startup or overload).
	StatusUnavailable = nwep.StatusUnavailable
)
