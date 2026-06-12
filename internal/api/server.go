package api

import (
	"database/sql"
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"
	"sync"

	"github.com/andybarilla/exit66jukebox/internal/broadcast"
	"github.com/andybarilla/exit66jukebox/internal/enrich"
	"github.com/andybarilla/exit66jukebox/internal/events"
	"github.com/andybarilla/exit66jukebox/internal/jukebox"
	"github.com/andybarilla/exit66jukebox/internal/scan"
)

// Server holds dependencies and builds the HTTP handler.
type Server struct {
	db         *sql.DB
	jb         *jukebox.Jukebox
	ui         fs.FS
	listenAddr string // server's own listen addr, for building Sonos-reachable URLs
	hubs       map[string]*broadcast.Hub
	buses      map[string]*events.Bus
	enrich     *enrich.Runner // nil until SetEnrichRunner; endpoints 503 while nil
	scan       *scan.Progress // nil until SetScanProgress (no library); endpoint 503 while nil

	// sonosIPs is the allowlist of IPs from the most recent discovery; casts are
	// restricted to it so an arbitrary ip can't be used to make the server POST
	// to an internal host (SSRF). Guarded by sonosMu.
	sonosMu  sync.Mutex
	sonosIPs map[string]bool
}

func NewServer(db *sql.DB, jb *jukebox.Jukebox, ui fs.FS) *Server {
	return &Server{
		db: db, jb: jb, ui: ui,
		hubs:     make(map[string]*broadcast.Hub),
		buses:    make(map[string]*events.Bus),
		sonosIPs: make(map[string]bool),
	}
}

// SetListenAddr records the server's own listen address (e.g. ":8066") so cast
// URLs can be built from the server's detected IP + this port rather than from
// the client-controlled Host header.
func (s *Server) SetListenAddr(addr string) { s.listenAddr = addr }

// SetEnrichRunner attaches the MusicBrainz/CAA enrichment runner that backs the
// /api/enrich endpoints.
func (s *Server) SetEnrichRunner(r *enrich.Runner) { s.enrich = r }

// SetScanProgress attaches the library scan progress that backs GET /api/scan.
// Left nil when no library is configured (no scan ever runs).
func (s *Server) SetScanProgress(p *scan.Progress) { s.scan = p }

// RegisterStream attaches a broadcast hub and event bus for a shared stream id.
func (s *Server) RegisterStream(id string, hub *broadcast.Hub, bus *events.Bus) {
	s.hubs[id] = hub
	s.buses[id] = bus
}

// listenerCount returns connected listeners for a registered shared stream, or
// 0 for private streams with no hub.
func (s *Server) listenerCount(streamID string) int {
	if hub, ok := s.hubs[streamID]; ok {
		return hub.ListenerCount()
	}
	return 0
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
	mux.HandleFunc("POST /api/streams/{id}/shuffle", s.setShuffle)
	mux.HandleFunc("GET /api/tracks/{id}/audio", s.trackAudio)
	mux.HandleFunc("GET /api/tracks/{id}/cover", s.trackCover)
	mux.HandleFunc("GET /api/albums/{id}/cover", s.albumCover)
	mux.HandleFunc("GET /stream/", s.streamAudio)
	mux.HandleFunc("GET /api/streams/{id}/events", s.streamEvents)
	mux.HandleFunc("GET /api/sonos/devices", s.sonosDevices)
	mux.HandleFunc("POST /api/sonos/cast", s.sonosCast)
	mux.HandleFunc("POST /api/sonos/stop", s.sonosStop)
	mux.HandleFunc("GET /api/discover/rediscover", s.discoverRediscover)
	mux.HandleFunc("GET /api/discover/recent", s.discoverRecent)
	mux.HandleFunc("GET /api/discover/genres", s.discoverGenres)
	mux.HandleFunc("POST /api/enrich", s.enrichStart)
	mux.HandleFunc("GET /api/enrich", s.enrichStatus)
	mux.HandleFunc("GET /api/scan", s.scanStatus)
	mux.HandleFunc("GET /api/streams/{id}/station", s.getStationHandler)
	mux.HandleFunc("POST /api/streams/{id}/station", s.startStationHandler)
	mux.HandleFunc("DELETE /api/streams/{id}/station", s.stopStationHandler)
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
