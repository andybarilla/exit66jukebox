package fed

import (
	"database/sql"
	"log"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

// Named listening groups scope catalog DISCOVERY between peers (#88). They are
// not a playback boundary: /api/tracks/{id}/audio stays reachable by any
// token-authenticated peer regardless of membership, because it is the one
// entry on peerVisibleAppRoutes and all three transports fetch through it.
// Andy's decision, for the reason in #167 — a peer id is claimed rather than
// proved, so a boundary built on it would look like security without being it.
//
// Two authorities apply the same table, depending on topology:
//
//	direct peer session  each peer's own groups decide what it serves (PeerRoutes)
//	hub topology         the HUB's groups decide what members discover (serveMerged)
//
// A denied peer is answered with an EMPTY catalog rather than an error, so
// revoking membership deletes the rows a peer already cached: ApplyCatalog with
// no rows falls through to DeleteRemoteTracks. An error would leave them.
//
// Both surfaces fail CLOSED on a request that carries no session identity: once
// groups exist an untagged request shares no group with anyone, so it discovers
// nothing. The requesting peer's id comes from SessionPeer — the same per-session
// tag #158 stamps onto forwarded signals, not a second one — and production
// always applies it (servePeerConn, dialPeer, serveHubConn), so failing closed
// only matters if a future path forgets to.

// catalogVisible reports whether viewer may discover owner's catalog. A nil db
// means no group storage is attached (tests, and the hub relay's audio-only
// fixtures), which leaves discovery unscoped as it was before groups existed. A
// query error denies rather than leaking, and is logged because a silent empty
// catalog is otherwise indistinguishable from an empty library.
func catalogVisible(db *sql.DB, owner, viewer string) bool {
	if db == nil {
		return true
	}
	ok, err := store.FederationCatalogVisible(db, owner, viewer)
	if err != nil {
		log.Printf("fed group check %s->%s: %v", owner, viewer, err)
		return false
	}
	return ok
}
