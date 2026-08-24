package api

import (
	"net/http"

	"github.com/andybarilla/exit66jukebox/internal/auth"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

// houseStreamID is the always-on shared stream: created at boot, the only cast
// target, and the only stream whose playback is scrobbled. It cannot be
// deleted. Its display name IS editable like any other shared stream's
// (criterion 2) — nothing derives behaviour from the name, only from this id.
// Matches the houseID const in main.go.
const houseStreamID = "house"

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

// streamGate resolves the route's stream once and applies the admin gate when
// its kind is shared, read from the store — not by comparing the id to a
// constant, which left every id but house completely ungated.
//
// An unknown id is rejected rather than passed through, because the handlers
// behind this gate used to implicit-create the stream they were handed: the
// gate would decide against a row that did not exist yet, and a caller could
// reach a privileged handler simply by inventing a URL.
//
// allowPrivate says what a private stream means for this particular operation.
// The queue controls let it through ungated, because there it is a listener
// driving their own queue — and since #128 the only private stream that can
// reach this gate is the caller's own, resolvePersonalStream having refused
// every other route into one. Rename and delete reject it: destroying a stream
// is not a queue control, and a personal stream has no owner-facing delete.
func (s *Server) streamGate(next http.HandlerFunc, allowPrivate bool) http.HandlerFunc {
	gated := s.requireAdmin(next)
	return func(w http.ResponseWriter, r *http.Request) {
		st, ok, err := store.GetStream(s.db, r.PathValue("id"))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "db error")
			return
		}
		if !ok {
			writeErr(w, http.StatusNotFound, "no such stream")
			return
		}
		if st.Kind != store.KindShared {
			if !allowPrivate {
				writeErr(w, http.StatusNotFound, "no such shared stream")
				return
			}
			next(w, r)
			return
		}
		if store.SecurityModeSetting(s.db) == store.SecurityModeOpen {
			next(w, r)
			return
		}
		gated(w, r)
	}
}

// requireAdminShared gates the per-stream queue controls: admin-only on a
// shared stream, left open on a listener's own private stream.
func (s *Server) requireAdminShared(next http.HandlerFunc) http.HandlerFunc {
	return s.streamGate(next, true)
}

// requireAdminOnSharedOnly gates the operations that are meaningless on a
// private stream — rename and delete — so neither can fall through ungated.
func (s *Server) requireAdminOnSharedOnly(next http.HandlerFunc) http.HandlerFunc {
	return s.streamGate(next, false)
}

// requireAdminOrOpen is requireAdminShared's gate without a stream to read it
// from: used by stream creation, which has no id yet but must be admin-only on
// the same terms, open-mode escape hatch included.
func (s *Server) requireAdminOrOpen(next http.HandlerFunc) http.HandlerFunc {
	gated := s.requireAdmin(next)
	return func(w http.ResponseWriter, r *http.Request) {
		if store.SecurityModeSetting(s.db) == store.SecurityModeOpen {
			next(w, r)
			return
		}
		gated(w, r)
	}
}

// isSharedStream reports whether the id names a shared stream. A missing row or
// a read error is not shared — the safe answer for a lazy-start decision.
func (s *Server) isSharedStream(id string) bool {
	st, ok, err := store.GetStream(s.db, id)
	return err == nil && ok && st.Kind == store.KindShared
}
