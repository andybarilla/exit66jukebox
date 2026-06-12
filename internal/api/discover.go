package api

import (
	"encoding/json"
	"net/http"

	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

func (s *Server) discover(w http.ResponseWriter, r *http.Request, orderBy string) {
	list, err := store.DiscoverTracks(s.db, store.DiscoverOpts{
		Genre:   r.URL.Query().Get("genre"),
		OrderBy: orderBy,
		Limit:   queryInt(r, "limit", 50),
		Offset:  queryInt(r, "offset", 0),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []model.Track{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) discoverRediscover(w http.ResponseWriter, r *http.Request) {
	s.discover(w, r, "rediscover")
}

func (s *Server) discoverRecent(w http.ResponseWriter, r *http.Request) {
	s.discover(w, r, "recent")
}

func (s *Server) discoverGenres(w http.ResponseWriter, r *http.Request) {
	list, err := store.GenreCounts(s.db)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []store.GenreCount{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) getStationHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if st, ok := s.jb.GetStation(id); ok {
		writeJSON(w, http.StatusOK, st)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}

func (s *Server) startStationHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Genre string `json:"genre"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Genre == "" {
		writeErr(w, http.StatusBadRequest, "missing genre")
		return
	}
	// Defaults per spec: refill when fewer than 3 remain, add 10 at a time.
	if err := s.jb.StartStation(id, body.Genre, 3, 10); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.publishQueueChanged(id)
	st, _ := s.jb.GetStation(id)
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) stopStationHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.jb.StopStation(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
