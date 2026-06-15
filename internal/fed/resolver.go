package fed

import "net/http"

// Resolver serves the audio for a track owned by another peer. The api layer
// calls it when a track row carries a non-empty source_peer, keeping all
// networking out of the handlers. Later phases supply the real implementation; a
// nil resolver means "not federated" and remote tracks return 503.
type Resolver interface {
	ServeRemoteAudio(w http.ResponseWriter, r *http.Request, peer string, remoteID int64)
}
