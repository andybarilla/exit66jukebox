package fed

import (
	"database/sql"
	"net/http"
)

// The handler served over each kind of federation session, in one place.
//
// These exist because the composition is a security boundary and it was
// previously spelled out twice — once in main.go and once in
// internal/api/federation_boundary_test.go, which asserts what a session can
// reach. A copy drifts silently: the test keeps passing while it pins a
// composition production no longer builds. Both callers now go through here, so
// there is nothing to drift.
//
// What each layer contributes:
//
//	WithCapsRoute    /fed/caps, so a peer can discover transports post-handshake
//	WithSignalRelay  POST /fed/signal/{to}, the inbound half of WebRTC signaling
//	AppRoutes        the peerVisibleAppRoutes allowlist and nothing else
//	PeerRoutes       /fed/catalog plus that same allowlist
//
// None of them mounts app at "/" — see peerVisibleAppRoutes.

// MemberSessionHandler is what a member serves back to its hub.
func MemberSessionHandler(caps Capabilities, app http.Handler) http.Handler {
	return WithCapsRoute(caps, AppRoutes(app))
}

// HubSessionHandler is what a hub serves to its members: the relay routes plus
// the signaling endpoint.
func HubSessionHandler(caps Capabilities, signaler *Signaler, relay *Relay) http.Handler {
	return WithCapsRoute(caps, WithSignalRelay(signaler, relay.Routes()))
}

// PeerSessionHandler is what a peer serves over a direct peer session. The
// signal relay is what lets a remote peer's WebRTC negotiation reach this
// process's mailbox; without it the transport's outbound POST has nothing to
// arrive at and the tier never engages (#152).
func PeerSessionHandler(caps Capabilities, signaler *Signaler, db *sql.DB, app http.Handler) http.Handler {
	return WithCapsRoute(caps, WithSignalRelay(signaler, PeerRoutes(db, app)))
}
