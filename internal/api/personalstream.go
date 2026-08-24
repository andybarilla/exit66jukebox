package api

import (
	"net/http"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

// callerPersonalStream returns the stream id that "my personal stream" names
// for this request, and false when the request has none.
//
// It has none in the two open security modes: those admit requests carrying no
// user at all, so a personal stream there would be a single row shared by
// every listener again — the exact bug (#128). The Personal control is hidden
// in those modes rather than backed by a session-keyed stream, so there is no
// per-caller row to hand back.
//
// In the two secured modes every request that reaches a stream route has
// already resolved to a user (RequireAuthMiddleware refuses anonymous ones), so
// the id is derived from that user and never from anything the client sent.
func (s *Server) callerPersonalStream(r *http.Request) (string, bool) {
	switch store.SecurityModeSetting(s.db) {
	case store.SecurityModeHouseholdProfiles, store.SecurityModeFullLogin:
	default:
		return "", false
	}
	u, ok := s.currentUser(r)
	if !ok {
		return "", false
	}
	return store.PersonalStreamID(u.ID), true
}

// resolvePersonalStream rewrites the route's {id} when the client sent the
// personal-stream alias, and refuses any other route into a private stream.
// Every /api/streams/{id} route goes through it, so the rules hold for reads,
// queue controls, stations, rename and delete alike.
//
// Three cases, in order:
//
//   - The alias. Resolved to the caller's own id and provisioned if this is
//     their first use — boot used to create the one global row, and with the id
//     now derived per user there is nothing at boot that knows the users.
//   - An id in the per-user namespace. Always refused, even the caller's own:
//     the ids are derived from a user id, so honouring one in a path would let
//     anyone read or wipe anyone's queue by counting upwards. The alias is the
//     only way in, which is what keeps the derivation server-side.
//   - Any other private row. Refused too. Nothing creates private streams
//     outside the namespace any more, so this only catches rows left by an
//     older build — but a private stream is somebody's queue whichever way it
//     got there.
//
// A refusal is 404 rather than 403 so the answer does not reveal whether the
// stream exists.
func (s *Server) resolvePersonalStream(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		if id == store.PersonalStreamAlias {
			mine, ok := s.callerPersonalStream(r)
			if !ok {
				writeErr(w, http.StatusNotFound, "no such stream")
				return
			}
			if err := store.EnsurePrivateStream(s.db, mine); err != nil {
				writeErr(w, http.StatusInternalServerError, "db error")
				return
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
