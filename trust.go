package velocity

import (
	nwep "github.com/usenwep/nwep-go"
)

const contextKeyVerifiedIdentity = "velocity.verified_identity"

// TrustConfig holds the parameters for constructing a nwep.TrustStore. It is
// passed to WithTrust to configure identity verification on a Server.
//
// All fields are optional. If Settings is nil, nwep.TrustSettingsDefault is
// used. If Storage is nil, an in-memory store is created. Anchors are added to
// the trust store after construction; if any anchor fails to be added, Build
// returns an error and frees the partially constructed store.
type TrustConfig struct {
	// Settings controls staleness thresholds, identity cache TTL, and
	// the anchor quorum threshold. If nil, the nwep defaults are used.
	Settings *nwep.TrustSettings

	// Storage provides persistent anchor and checkpoint storage
	// callbacks. If nil, the trust store operates in memory only and
	// state is lost when the server shuts down.
	Storage *nwep.TrustStorage

	// Anchors is the initial set of BLS public keys to trust as
	// checkpoint signers. These are added as non-builtin anchors.
	Anchors []nwep.BLSPubkey
}

// Build constructs a nwep.TrustStore from the configuration. The caller is
// responsible for calling Free on the returned TrustStore when it is no longer
// needed. When used with WithTrust, the Server manages the TrustStore lifetime
// automatically.
//
// This function returns a non-nil error if the underlying nwep trust store
// cannot be created or if any anchor in Anchors fails to be added.
func (tc *TrustConfig) Build() (*nwep.TrustStore, error) {
	settings := tc.Settings
	if settings == nil {
		settings = nwep.TrustSettingsDefault()
	}
	var ts *nwep.TrustStore
	var err error
	if tc.Storage != nil {
		ts, err = nwep.NewTrustStoreWithStorage(settings, tc.Storage)
	} else {
		ts, err = nwep.NewTrustStore(settings)
	}
	if err != nil {
		return nil, err
	}
	for _, anchor := range tc.Anchors {
		if err := ts.AddAnchor(anchor, false); err != nil {
			ts.Free()
			return nil, err
		}
	}
	return ts, nil
}

// TrustVerify returns middleware that performs identity verification for each
// request. For every authenticated peer (non-zero node ID), the middleware
// looks up the peer's verified identity in the given TrustStore. If a verified
// identity is found, it is stored in the context and can be retrieved by
// calling VerifiedIdentity.
//
// If the peer has no verified identity (the lookup fails or returns nil), the
// request proceeds without a verified identity in the context - no error is
// returned. To reject unverified peers, chain TrustVerify with RequirePeer or
// a custom middleware that checks VerifiedIdentity.
//
// ts must not be nil and must remain valid for the lifetime of the server.
func TrustVerify(ts *nwep.TrustStore) MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			peer := c.PeerNodeID()
			if !peer.IsZero() {
				vi, err := ts.LookupIdentity(peer, nwep.Tstamp(nowNanos()))
				if err == nil && vi != nil {
					c.Set(contextKeyVerifiedIdentity, vi)
				}
			}
			return next(c)
		}
	}
}

// VerifiedIdentity extracts the peer's verified identity from the context. It
// returns nil if the TrustVerify middleware was not used, if the peer was not
// authenticated, or if the identity lookup did not find a verified entry.
//
// The returned pointer is valid only for the lifetime of the handler.
func VerifiedIdentity(c *Context) *nwep.VerifiedIdentity {
	v, ok := c.Get(contextKeyVerifiedIdentity)
	if !ok {
		return nil
	}
	vi, _ := v.(*nwep.VerifiedIdentity)
	return vi
}
