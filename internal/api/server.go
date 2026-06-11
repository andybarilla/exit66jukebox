package api

import (
	"database/sql"
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"

	"github.com/andybarilla/exit66jukebox/internal/jukebox"
)

// Server holds dependencies and builds the HTTP handler.
type Server struct {
	db *sql.DB
	jb *jukebox.Jukebox
	ui fs.FS
}

func NewServer(db *sql.DB, jb *jukebox.Jukebox, ui fs.FS) *Server {
	return &Server{db: db, jb: jb, ui: ui}
}

// Handler returns the routed mux. Handlers live in sibling files.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/artists", s.listArtists)
	mux.HandleFunc("GET /api/albums", s.listAlbums)
	mux.HandleFunc("GET /api/tracks", s.listTracks)
	mux.HandleFunc("GET /api/streams/{id}", s.getStream)
	mux.HandleFunc("GET /api/streams/{id}/next", s.nextTrack)
	mux.HandleFunc("POST /api/streams/{id}/requests", s.request)
	mux.HandleFunc("DELETE /api/streams/{id}/requests/{trackID}", s.removeRequest)
	mux.HandleFunc("DELETE /api/streams/{id}/requests", s.clearRequests)
	mux.HandleFunc("GET /api/tracks/{id}/audio", s.trackAudio)
	mux.HandleFunc("GET /api/tracks/{id}/cover", s.trackCover)
	mux.HandleFunc("GET /api/albums/{id}/cover", s.albumCover)
	if s.ui != nil {
		mux.Handle("GET /", http.FileServerFS(s.ui))
	}
	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

// queryInt reads an int query parameter, returning def when absent or invalid.
func queryInt(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
