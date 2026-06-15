package api

import (
	"net/http"
	"strconv"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

func (s *Server) trackAudio(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	t, path, ok := store.GetTrack(s.db, id)
	if !ok {
		writeErr(w, http.StatusNotFound, "track not found")
		return
	}
	if t.SourcePeer != "" {
		if s.fedResolver == nil {
			writeErr(w, http.StatusServiceUnavailable, "remote track unavailable")
			return
		}
		s.fedResolver.ServeRemoteAudio(w, r, t.SourcePeer, t.RemoteID)
		return
	}
	http.ServeFile(w, r, path) // sets type + supports Range for <audio> seeking
}
