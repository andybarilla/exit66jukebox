package api

import "net/http"

// getConfig exposes runtime settings the frontend needs. It is the seam a future
// settings UI will read/write, and the status seam admin mode reuses:
// admin_required (a password is configured) and is_admin (this request carries a
// valid token, or the gate is open). is_admin reflects the request's bearer
// token, so callers must send it here too.
func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	peers := []string{}
	if s.fedPeers != nil {
		peers = s.fedPeers()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mute_local_on_cast": s.muteLocalOnCast,
		"fed_peers":          peers,
		"admin_required":     !s.adminOpen(),
		"is_admin":           s.isAdmin(r),
	})
}
