package velocity

import nwep "github.com/usenwep/nwep-go"

// WEB/1 request method constants, re-exported from nwep for convenience.
//
// These are string constants that appear in Context.Method and are used
// with Router.Method, MethodFilter, and the convenience registration
// methods (Read, Write, Update, Delete). They correspond to the method
// values defined in the WEB/1 specification.
const (
	// MethodRead requests retrieval of a resource. Idempotent.
	// Equivalent to HTTP GET.
	MethodRead = nwep.MethodRead

	// MethodWrite requests creation of a new resource. Not idempotent.
	// Equivalent to HTTP POST.
	MethodWrite = nwep.MethodWrite

	// MethodUpdate requests modification of an existing resource.
	// Idempotent. Equivalent to HTTP PUT/PATCH.
	MethodUpdate = nwep.MethodUpdate

	// MethodDelete requests removal of a resource. Idempotent.
	// Equivalent to HTTP DELETE.
	MethodDelete = nwep.MethodDelete

	// MethodConnect is used during the initial connection handshake
	// between client and server. Application handlers typically do not
	// need to handle this method directly.
	MethodConnect = nwep.MethodConnect

	// MethodAuthenticate is used during the mutual authentication
	// phase of the connection handshake. Application handlers typically
	// do not need to handle this method directly.
	MethodAuthenticate = nwep.MethodAuthenticate

	// MethodHeartbeat is used for keepalive probes between client and
	// server. Application handlers typically do not need to handle this
	// method directly.
	MethodHeartbeat = nwep.MethodHeartbeat
)
