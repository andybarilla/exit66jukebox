package api

import (
	"net/http"

	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

func (s *Server) listArtists(w http.ResponseWriter, r *http.Request) {
	list, err := store.ListArtists(s.db,
		r.URL.Query().Get("search"), queryInt(r, "limit", 50), queryInt(r, "offset", 0))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []model.Artist{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) listAlbums(w http.ResponseWriter, r *http.Request) {
	list, err := store.ListAlbums(s.db,
		r.URL.Query().Get("search"), queryInt(r, "limit", 50), queryInt(r, "offset", 0))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []model.Album{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) listTracks(w http.ResponseWriter, r *http.Request) {
	list, err := store.ListTracks(s.db,
		r.URL.Query().Get("search"), queryInt(r, "limit", 50), queryInt(r, "offset", 0))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []model.Track{}
	}
	writeJSON(w, http.StatusOK, list)
}
