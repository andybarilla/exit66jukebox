package api

import "net/http"

// enrichStart begins a background enrichment pass (or reports the in-progress
// one). It always returns the current status; the body's running field tells
// the caller whether a new pass began. 503 if no runner is wired.
func (s *Server) enrichStart(w http.ResponseWriter, r *http.Request) {
	if s.enrich == nil {
		writeErr(w, http.StatusServiceUnavailable, "enrichment not available")
		return
	}
	status, _ := s.enrich.Start(r.Context())
	writeJSON(w, http.StatusOK, status)
}

// enrichStatus reports the current/last pass progress. 503 if no runner.
func (s *Server) enrichStatus(w http.ResponseWriter, r *http.Request) {
	if s.enrich == nil {
		writeErr(w, http.StatusServiceUnavailable, "enrichment not available")
		return
	}
	writeJSON(w, http.StatusOK, s.enrich.Status())
}
