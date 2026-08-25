package fed

import "net/http"

// peerVisibleAppRoutes is the allowlist of application routes a peer reaches
// over a federation session. It is the single place to widen that view: adding
// a route here exposes it on every federation handler at once, and nothing
// outside this list is reachable.
//
// Why an allowlist rather than an auth check: a peer carries no browser session,
// so the public listener's RequireAuthMiddleware — which is what protects the
// rest of the application — cannot run on this path. Anything mounted here is
// reachable by any token-authenticated peer with no further check of its own.
//
// The list is one route long. GET /api/tracks/{id}/audio is on it because all
// three audio transports fetch a peer's track through it: the hub relay
// (Relay.ServeHTTP), the yamux-direct tier (directResolver.ServeRemoteAudio) and
// the WebRTC tier (serveOneOverConn). trackAudio has no authorization of its
// own; exposing it to peers is the deliberate trade for remote playback.
//
// The boundary holds by two mechanisms, not one, and only the first is here.
// Go's {id} wildcard matches a percent-encoded slash as a literal, so
// /api/tracks/..%2f..%2fapi%2fstreams%2fhouse/audio does match this pattern and
// does reach the application. What refuses it there is trackAudio's
// strconv.ParseInt rejecting the id — 400, not 404.
//
// That second mechanism works only because the application re-routes on the same
// escaped path this mux matched. Inserting anything between AppRoutes and the
// application that decodes or normalizes r.URL.Path before dispatch removes it,
// and the request's routing identity changes: with such a shim in place the
// example above stops being an invalid id and becomes a redirect to
// /api/streams/house/audio. That particular one still reaches no privileged
// handler, because cleaning cannot eat this pattern's trailing literal "audio"
// segment — so the trailing literal is doing load-bearing work that nothing
// states. A future allowlist entry ending in a wildcard would not have it.
//
// Both mechanisms are pinned: TestAppRoutesEncodedPathsMatchOnlyTheAllowlistedPattern
// here, and TestFederationSessionEncodedPathsReachNoOtherHandler in internal/api,
// which fails if a normalizing shim is introduced.
var peerVisibleAppRoutes = []string{
	"GET /api/tracks/{id}/audio",
}

// mountAppRoutes mounts peerVisibleAppRoutes of app onto mux. A nil app mounts
// nothing, so every route is refused.
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
