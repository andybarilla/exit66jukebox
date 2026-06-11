package api

import (
	"net/http"
	"strconv"

	"github.com/andybarilla/exit66jukebox/internal/jukebox"
	"github.com/andybarilla/exit66jukebox/internal/model"
)

func (s *Server) getStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.jb.EnsureStream(id, "private")
	q, err := s.jb.Queue(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if q == nil {
		q = []model.Track{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "queue": q})
}

func (s *Server) nextTrack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.jb.EnsureStream(id, "private")
	tr, ok := s.jb.Next(id)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "track": tr})
}

func (s *Server) request(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.jb.EnsureStream(id, "private")
	r.ParseForm()
	kind := r.FormValue("kind") // "track" | "album" | "artist"
	targetID, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)

	switch kind {
	case "album":
		n := s.jb.RequestAlbum(id, targetID)
		writeJSON(w, http.StatusOK, map[string]any{"queued": n})
	case "artist":
		n := s.jb.RequestArtist(id, targetID)
		writeJSON(w, http.StatusOK, map[string]any{"queued": n})
	default:
		res, err := s.jb.Request(id, targetID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"queued":  res == jukebox.Requested,
			"message": res.Message(),
		})
	}
}

func (s *Server) removeRequest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	trackID, _ := strconv.ParseInt(r.PathValue("trackID"), 10, 64)
	if err := s.jb.Remove(id, trackID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) clearRequests(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.jb.Clear(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
