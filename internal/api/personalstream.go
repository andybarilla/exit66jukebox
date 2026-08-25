package api

import (
	"net/http"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

// personalStreamFor returns the stream id that "my personal stream" names for a
// caller, and false when there is none.
//
// The two open modes have none: they admit requests carrying no user, so a
// personal stream there would be one row shared by every listener (#128).
//
// Mode and user are passed in rather than read, so getConfig and the resolver
// share one rule without either repeating the other's lookups.
func personalStreamFor(mode store.SecurityMode, u store.User, authed bool) (string, bool) {
	switch mode {
	case store.SecurityModeHouseholdProfiles, store.SecurityModeFullLogin:
	default:
		return "", false
	}
	if !authed {
		return "", false
	}
	return store.PersonalStreamID(u.ID), true
}

// callerPersonalStream resolves personalStreamFor from the request itself.
func (s *Server) callerPersonalStream(r *http.Request) (string, bool) {
	u, authed := s.currentUser(r)
	return personalStreamFor(store.SecurityModeSetting(s.db), u, authed)
}

// resolvePersonalStream rewrites the route's {id} when the client sent the
// personal-stream alias, and refuses every other route into a private stream.
// All of /api/streams/{id} goes through it, so reads, queue controls, stations,
// rename and delete are covered alike.
//
// An id in the per-user namespace is refused even when it is the caller's own:
// the ids are derived from a user id, so honouring one in a path would let
// anyone reach anyone's queue by counting. The alias is the only way in, which
// is what keeps the derivation server-side. Any other private row is refused
// too — it is somebody's queue however it came to exist.
//
// provision says whether resolving the alias may create the row, which is how a
// user gets their first personal stream. Rename and delete pass false: they
// refuse a private stream downstream, so provisioning would make a 404 write.
//
// Refusals are 404 rather than 403, so the answer does not reveal whether the
// stream exists.
func (s *Server) resolvePersonalStream(next http.HandlerFunc, provision bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		if id == store.PersonalStreamAlias {
			mine, ok := s.callerPersonalStream(r)
			if !ok {
				writeErr(w, http.StatusNotFound, "no such stream")
				return
			}
			if provision {
				if err := store.EnsurePrivateStream(s.db, mine); err != nil {
					writeErr(w, http.StatusInternalServerError, "db error")
					return
				}
			}
			r.SetPathValue("id", mine)
			next(w, r)
			return
		}

		if store.IsPersonalStreamID(id) {
			writeErr(w, http.StatusNotFound, "no such stream")
			return
		}

		st, found, err := store.GetStream(s.db, id)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "db error")
			return
		}
		if found && st.Kind == store.KindPrivate {
			writeErr(w, http.StatusNotFound, "no such stream")
			return
		}
		next(w, r)
	}
}

// personalStream serves the routes a caller drives against their own stream.
func (s *Server) personalStream(next http.HandlerFunc) http.HandlerFunc {
	return s.resolvePersonalStream(next, true)
}

// personalStreamNoProvision serves rename and delete. See provision above.
func (s *Server) personalStreamNoProvision(next http.HandlerFunc) http.HandlerFunc {
	return s.resolvePersonalStream(next, false)
}
