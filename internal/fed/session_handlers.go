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
//	                 (plus, on the hub only, the onward half)
//	AppRoutes        the peerVisibleAppRoutes allowlist and nothing else
//	PeerRoutes       /fed/catalog (group-scoped) plus that same allowlist
//
// None of them mounts app at "/" — see peerVisibleAppRoutes.

// MemberSessionHandler is what a member serves back to its hub.
//
// It carries the signal relay because this — not the peer session — is the
// handler a hub reaches when it forwards a signal onward. A peer with a HubAddr
// serves this over its hub session (runPeer -> runMember), and the hub's
// forwarder addresses it through that same session, so without the relay here
// the forward 404s and the hub falls back to the 503 #158 exists to remove.
//
// The forwarder is nil: a member relays to its own mailboxes only, which is what
// bounds hub relaying to a single hop.
func MemberSessionHandler(caps Capabilities, signaler *Signaler, app http.Handler) http.Handler {
	return WithCapsRoute(caps, WithSignalRelay(signaler, nil, AppRoutes(app)))
}

// HubSessionHandler is what a hub serves to its members: the relay routes plus
// the signaling endpoint.
//
// The hub is the only composition whose relay forwards. It hosts no signaling
// mailbox of its own — no WebRTC transport is built for the hub role — so every
// recipient it is asked about is one of its members, and forwarding over that
// member's session is the only path between two peers that can each reach the
// hub and nothing else (#158). The forwarder is built from the relay's registry
// because that is the hub's registry: main.go passes the Manager's.
//
// Callers must serve the result per session through WithSessionPeer, or the
// relay has no sender to attribute a forwarded message to and refuses it.
// Manager.serveHubConn does.
func HubSessionHandler(caps Capabilities, signaler *Signaler, relay *Relay) http.Handler {
	return WithCapsRoute(caps, WithSignalRelay(signaler, hubSignalForwarder(relay.reg, nil), relay.Routes()))
}

// PeerSessionHandler is what a peer serves over a direct peer session. The
// signal relay is what lets a remote peer's WebRTC negotiation reach this
// process's mailbox; without it the transport's outbound POST has nothing to
// arrive at and the tier never engages (#152). It takes no forwarder: a peer
// relays to itself only, which is what stops a hub-forwarded message being
// forwarded again.
//
// selfPeerID is this instance's own peer id, which PeerRoutes needs to decide
// whether the requesting peer shares a listening group with it (#88). Callers
// must serve the result per session through WithSessionPeer, or /fed/catalog has
// no requester to scope against; Manager.servePeerConn and dialPeer do.
func PeerSessionHandler(caps Capabilities, signaler *Signaler, db *sql.DB, selfPeerID string, app http.Handler) http.Handler {
	return WithCapsRoute(caps, WithSignalRelay(signaler, nil, PeerRoutes(db, selfPeerID, app)))
}
