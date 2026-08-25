package fed

import "net/http"

// peerVisibleAppRoutes is the allowlist of application routes a peer reaches
// over a federation session. It is the single place to widen that view: adding
// a route here exposes it on every federation handler at once, and nothing
// outside this list is reachable.
//
// The list is deliberately one route long. A peer carries no browser session,
// so the public listener's auth middleware — which is what protects the rest of
// the application — never runs on a federation session. Anything mounted here
// is therefore reachable by any token-authenticated peer with no further check
// of its own.
//
// GET /api/tracks/{id}/audio is on the list because all three audio transports
// fetch a peer's track through it: the hub relay (Relay.ServeHTTP), the
// yamux-direct tier (directResolver.ServeRemoteAudio) and the WebRTC tier
// (serveOneOverConn). trackAudio has no authorization of its own; exposing it
// to peers is the deliberate trade for remote playback.
var peerVisibleAppRoutes = []string{
	"GET /api/tracks/{id}/audio",
}

// mountAppRoutes mounts the allowlisted application routes of app onto mux.
// A nil app mounts nothing, so every route is refused.
func mountAppRoutes(mux *http.ServeMux, app http.Handler) {
	if app == nil {
		return
	}
	for _, route := range peerVisibleAppRoutes {
		mux.Handle(route, app)
	}
}

// AppRoutes returns a handler exposing only peerVisibleAppRoutes of app. It is
// what a federation session sees of the application: an unlisted path gets 404
// from the empty mux (405 when the path matches an allowlisted pattern under
// another method) and never reaches app.
func AppRoutes(app http.Handler) http.Handler {
	mux := http.NewServeMux()
	mountAppRoutes(mux, app)
	return mux
}
