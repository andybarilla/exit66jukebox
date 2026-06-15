package api

import "net/http"

// getConfig exposes runtime settings the frontend needs. It is the seam a future
// settings UI will read/write; for now it only carries the mute-on-cast flag.
func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	peers := []string{}
	if s.fedPeers != nil {
		peers = s.fedPeers()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mute_local_on_cast": s.muteLocalOnCast,
		"fed_peers":          peers,
	})
}
