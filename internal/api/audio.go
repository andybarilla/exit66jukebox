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
	_, path, ok := store.GetTrack(s.db, id)
	if !ok {
		writeErr(w, http.StatusNotFound, "track not found")
		return
	}
	http.ServeFile(w, r, path) // sets type + supports Range for <audio> seeking
}
