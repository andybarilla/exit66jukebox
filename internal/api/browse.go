package api

import (
	"net/http"
	"strconv"

	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

// writeList serializes a browse page, advertising the unpaged total via
// X-Total-Count so the client can render "N results" and page.
func writeList(w http.ResponseWriter, list any, total int) {
	w.Header().Set("X-Total-Count", strconv.Itoa(total))
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) listArtists(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	list, err := store.ListArtistsEnriched(s.db, search, queryInt(r, "limit", 50), queryInt(r, "offset", 0))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	total, err := store.CountArtists(s.db, search)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []model.EnrichedArtist{}
	}
	writeList(w, list, total)
}

func (s *Server) listAlbums(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	list, err := store.ListAlbumsEnriched(s.db, search, queryInt(r, "limit", 50), queryInt(r, "offset", 0))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	total, err := store.CountAlbums(s.db, search)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []model.EnrichedAlbum{}
	}
	writeList(w, list, total)
}

func (s *Server) listTracks(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	list, err := store.ListTracksEnriched(s.db, search, queryInt(r, "limit", 50), queryInt(r, "offset", 0))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	total, err := store.CountTracks(s.db, search)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []model.EnrichedTrack{}
	}
	writeList(w, list, total)
}

// enrichOne enriches a single track for now-playing-style payloads, falling back
// to the bare track if enrichment fails.
func (s *Server) enrichOne(t model.Track) model.EnrichedTrack {
	enriched, err := store.EnrichTracks(s.db, []model.Track{t})
	if err != nil || len(enriched) == 0 {
		return model.EnrichedTrack{Track: t}
	}
	return enriched[0]
}

// albumTracks returns one album's tracks (enriched) for the album dialog.
func (s *Server) albumTracks(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid album id")
		return
	}
	list, err := store.TracksByAlbumEnriched(s.db, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []model.EnrichedTrack{}
	}
	writeJSON(w, http.StatusOK, list)
}
