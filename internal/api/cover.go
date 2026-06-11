package api

import (
	"net/http"
	"os"
	"strconv"

	"github.com/dhowden/tag"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

func (s *Server) trackCover(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	serveCover(w, s, id)
}

func (s *Server) albumCover(w http.ResponseWriter, r *http.Request) {
	albumID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	trackID, ok := store.FirstTrackIDOfAlbum(s.db, albumID)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	serveCover(w, s, trackID)
}

// serveCover writes a track's embedded cover image, or 404 if there is none.
func serveCover(w http.ResponseWriter, s *Server, trackID int64) {
	_, path, ok := store.GetTrack(s.db, trackID)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	defer f.Close()
	m, err := tag.ReadFrom(f)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	pic := m.Picture()
	if pic == nil || len(pic.Data) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", pic.MIMEType)
	w.Header().Set("Cache-Control", "max-age=86400")
	w.Write(pic.Data)
}
