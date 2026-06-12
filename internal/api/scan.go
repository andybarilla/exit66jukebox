package api

import "net/http"

// scanStatus reports a snapshot of the library scan progress. 503 when no
// library is configured (no scan ever runs, so there's nothing to report).
func (s *Server) scanStatus(w http.ResponseWriter, r *http.Request) {
	if s.scan == nil {
		writeErr(w, http.StatusServiceUnavailable, "scan not available")
		return
	}
	writeJSON(w, http.StatusOK, s.scan.Snapshot())
}
