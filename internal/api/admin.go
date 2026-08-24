package api

import (
	"net/http"

	"github.com/andybarilla/exit66jukebox/internal/auth"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

// sharedStreamID is the id of the one shared, projected-to-the-room stream. Its
// privileged controls (skip/remove/clear/shuffle) are admin-gated; every other
// stream id is a guest's private stream and stays open. Matches the houseID
// const in main.go.
const sharedStreamID = "house"

// currentUser resolves the request's session cookie to a user. ok is false for
// anonymous requests (no/invalid/expired cookie).
func (s *Server) currentUser(r *http.Request) (store.User, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return store.User{}, false
	}
	u, ok, err := store.UserBySession(s.db, auth.HashToken(c.Value))
	if err != nil || !ok {
		return store.User{}, false
	}
	return u, true
}

// isAdmin reports whether the request carries a valid admin session.
func (s *Server) isAdmin(r *http.Request) bool {
	u, ok := s.currentUser(r)
	return ok && u.IsAdmin
}

// requireAdmin gates next behind an admin session: 401 when not logged in at
// all, 403 when logged in but not an admin.
func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := s.currentUser(r)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "login required")
			return
		}
		if !u.IsAdmin {
			writeErr(w, http.StatusForbidden, "admin required")
			return
		}
		if !store.AdminMFARequired(s.db) {
			next(w, r)
			return
		}
		factor, ok, err := store.GetMFAFactor(s.db, u.ID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "db error")
			return
		}
		if !ok || factor.EnabledAt <= 0 {
			writeErr(w, http.StatusForbidden, "mfa required for admin access")
			return
		}
		next(w, r)
	}
}

// requireAdminShared gates next only when the request targets the shared house
// stream. Private streams (a guest's "me") pass through untouched so guests can
// always drive their own queue.
func (s *Server) requireAdminShared(next http.HandlerFunc) http.HandlerFunc {
	gated := s.requireAdmin(next)
	return func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") != sharedStreamID {
			next(w, r)
			return
		}
		mode := store.SecurityModeSetting(s.db)
		if mode == store.SecurityModeOpen {
			next(w, r)
			return
		}
		gated(w, r)
	}
}
