package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
)

// sharedStreamID is the id of the one shared, projected-to-the-room stream. Its
// privileged controls (skip/remove/clear/shuffle) are admin-gated; every other
// stream id is a guest's private stream and stays open. Matches the houseID
// const in main.go.
const sharedStreamID = "house"

// adminOpen reports whether the gate is disabled (no password configured). When
// open every request passes — the default, backwards-compatible state.
func (s *Server) adminOpen() bool { return s.adminPassword == "" }

// bearerToken extracts the token from an "Authorization: Bearer <token>" header,
// or "" when absent/malformed.
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return h[len(prefix):]
	}
	return ""
}

// validToken reports whether tok is a currently-issued admin token.
func (s *Server) validToken(tok string) bool {
	if tok == "" {
		return false
	}
	s.adminMu.Lock()
	defer s.adminMu.Unlock()
	return s.adminTokens[tok]
}

// isAdmin reports whether the request is allowed admin actions: either the gate
// is open or it carries a valid token.
func (s *Server) isAdmin(r *http.Request) bool {
	return s.adminOpen() || s.validToken(bearerToken(r))
}

// requireAdmin gates next behind admin mode. With no password configured it
// passes through (open); otherwise a valid bearer token is required, 403 if not.
func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.isAdmin(r) {
			next(w, r)
			return
		}
		writeErr(w, http.StatusForbidden, "admin required")
	}
}

// requireAdminShared gates next only when the request targets the shared house
// stream. Private streams (a guest's "me") pass through untouched so guests can
// always drive their own queue.
func (s *Server) requireAdminShared(next http.HandlerFunc) http.HandlerFunc {
	gated := s.requireAdmin(next)
	return func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") == sharedStreamID {
			gated(w, r)
			return
		}
		next(w, r)
	}
}

// adminLogin checks the posted password against the configured secret with a
// constant-time compare and, on success, issues a random bearer token.
func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) {
	if s.adminOpen() {
		// No password configured — admin mode is unconditionally on, so there is
		// nothing to log into.
		writeErr(w, http.StatusUnauthorized, "admin mode is not enabled")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if subtle.ConstantTimeCompare([]byte(body.Password), []byte(s.adminPassword)) != 1 {
		writeErr(w, http.StatusUnauthorized, "incorrect password")
		return
	}
	tok, err := randomToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	s.adminMu.Lock()
	s.adminTokens[tok] = true
	s.adminMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]string{"token": tok})
}

// adminLogout revokes the request's bearer token. Idempotent: unknown tokens are
// a no-op success.
func (s *Server) adminLogout(w http.ResponseWriter, r *http.Request) {
	if tok := bearerToken(r); tok != "" {
		s.adminMu.Lock()
		delete(s.adminTokens, tok)
		s.adminMu.Unlock()
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// randomToken returns a 256-bit random hex token.
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
