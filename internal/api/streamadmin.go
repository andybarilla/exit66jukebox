package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

// maxStreamNameLen bounds the display label. Names are free-form and may
// collide; only the length is enforced.
const maxStreamNameLen = 60

// listStreams returns the shared streams, house included. Private streams are
// never listed: they belong to one listener and the client pins its own.
func (s *Server) listStreams(w http.ResponseWriter, r *http.Request) {
	list, err := store.ListStreams(s.db, store.KindShared)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, st := range list {
		out = append(out, map[string]any{
			"id":        st.ID,
			"name":      st.Name,
			"kind":      st.Kind,
			"house":     st.ID == houseStreamID,
			"listeners": s.listenerCount(st.ID),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// createStream makes a named shared stream with a fresh opaque id. The cap is
// enforced by the store, not here, so it cannot be bypassed by another caller.
func (s *Server) createStream(w http.ResponseWriter, r *http.Request) {
	name, ok := decodeStreamName(w, r)
	if !ok {
		return
	}
	id, err := newStreamID()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not generate a stream id")
		return
	}
	switch err := store.CreateSharedStream(s.db, id, name); {
	case errors.Is(err, store.ErrStreamCapReached):
		writeErr(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": id, "name": name, "kind": store.KindShared, "house": false, "listeners": 0,
	})
}

// renameStream changes the display label. The id is the stable handle, so it is
// untouched and two streams may end up sharing a name.
func (s *Server) renameStream(w http.ResponseWriter, r *http.Request) {
	name, ok := decodeStreamName(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if err := store.RenameStream(s.db, id, name); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "name": name})
}

// deleteStream removes a shared stream, its queue and its pipeline. house is
// always-on and cannot be deleted.
func (s *Server) deleteStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == houseStreamID {
		writeErr(w, http.StatusForbidden, "the house stream cannot be deleted")
		return
	}
	// Stop any speaker playing this stream before the feed goes away, so the
	// speaker ends the cast itself rather than having its connection dropped
	// mid-play. Nothing records which speaker is on which stream, so this asks
	// the speakers (#130).
	s.stopSpeakersPlaying(id)
	// Then tear the pipeline down: connected listeners get a stream-closed
	// event and their channels closed, rather than hanging on a feed whose rows
	// are about to vanish.
	s.stopPipeline(id)
	if err := store.DeleteStream(s.db, id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// decodeStreamName reads {"name": ...} from the body, or the `name` form field.
// It writes the 400 itself and reports false when the name is missing or too long.
func decodeStreamName(w http.ResponseWriter, r *http.Request) (string, bool) {
	var body struct {
		Name string `json:"name"`
	}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		json.NewDecoder(r.Body).Decode(&body)
	} else {
		r.ParseForm()
		body.Name = r.FormValue("name")
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeErr(w, http.StatusBadRequest, "missing name")
		return "", false
	}
	if len(name) > maxStreamNameLen {
		writeErr(w, http.StatusBadRequest, "name too long")
		return "", false
	}
	return name, true
}

// newStreamID mints an opaque, URL-safe stream id. Nothing derives it from the
// name, so renaming never moves a stream and duplicate names are harmless.
func newStreamID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
