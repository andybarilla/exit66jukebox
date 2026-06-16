package api

import (
	"database/sql"
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/broadcast"
	"github.com/andybarilla/exit66jukebox/internal/enrich"
	"github.com/andybarilla/exit66jukebox/internal/events"
	"github.com/andybarilla/exit66jukebox/internal/fed"
	"github.com/andybarilla/exit66jukebox/internal/jukebox"
	"github.com/andybarilla/exit66jukebox/internal/recommend"
	"github.com/andybarilla/exit66jukebox/internal/scan"
	"github.com/andybarilla/exit66jukebox/internal/sonos"
)

// Server holds dependencies and builds the HTTP handler.
type Server struct {
	db         *sql.DB
	jb         *jukebox.Jukebox
	ui         fs.FS
	listenAddr string // server's own listen addr, for building Sonos-reachable URLs
	hubs       map[string]*broadcast.Hub
	buses      map[string]*events.Bus
	nowPlaying map[string]*NowPlaying // current-track trackers for shared streams
	enrich      *enrich.Runner         // nil until SetEnrichRunner; endpoints 503 while nil
	recommend   *recommend.Runner      // nil until SetRecommendRunner; endpoint returns [] while nil
	scan        *scan.Progress         // nil until SetScanProgress (no library); endpoint 503 while nil
	fedResolver fed.Resolver           // nil unless federation is configured
	fedPeers    func() []string        // returns online peer ids; nil when federation off

	// muteLocalOnCast is exposed via GET /api/config so the frontend can mute the
	// local <audio> while a Sonos cast is active. Sourced from config (env for now).
	muteLocalOnCast bool

	// signingSecret is the HMAC secret used to sign Sonos media URLs; loaded once
	// at startup from the store (store.MediaSigningSecret).
	signingSecret []byte
	// loginAttempts throttles the password form per client IP (soft brute-force
	// guard); guarded by loginMu.
	loginMu       sync.Mutex
	loginAttempts map[string][]int64 // ip -> recent attempt unix-millis

	// sonosIPs is the allowlist of IPs from the most recent discovery; casts are
	// restricted to it so an arbitrary ip can't be used to make the server POST
	// to an internal host (SSRF). sonosManual holds IPs added via /api/sonos/manual
	// (ip→room name) so an SSDP-blocked network can still cast; manual IPs survive
	// rediscovery and are allowed alongside discovered ones. Both guarded by sonosMu.
	sonosMu     sync.Mutex
	sonosIPs    map[string]bool
	sonosManual map[string]string

	// emailInvite, when non-nil (SMTP configured), sends an invite link to an
	// address. Best-effort: called in a goroutine, errors logged not surfaced.
	emailInvite func(to, link string)

	// manualVerify confirms a manually-entered IP actually serves a Sonos
	// descriptor before it's trusted (injectable for tests).
	manualVerify func(ip string) (name string, ok bool)

	// sonosDiscover runs SSDP multicast discovery; scanUnicast is the unicast /24
	// fallback used when SSDP finds nothing (both injectable for tests).
	sonosDiscover func() ([]sonos.Device, error)
	scanUnicast   func() []sonos.Device
}

func NewServer(db *sql.DB, jb *jukebox.Jukebox, ui fs.FS) *Server {
	return &Server{
		db: db, jb: jb, ui: ui,
		hubs:        make(map[string]*broadcast.Hub),
		buses:       make(map[string]*events.Bus),
		nowPlaying:  make(map[string]*NowPlaying),
		loginAttempts: make(map[string][]int64),
		sonosIPs:      make(map[string]bool),
		sonosManual: make(map[string]string),
		manualVerify: func(ip string) (string, bool) {
			return sonos.Verify(sonos.DescriptorURL(ip))
		},
		sonosDiscover: func() ([]sonos.Device, error) {
			return sonos.Discover(2 * time.Second)
		},
		scanUnicast: func() []sonos.Device {
			return sonos.ScanUnicast(sonos.OutboundIP(), 200*time.Millisecond)
		},
	}
}

// SetListenAddr records the server's own listen address (e.g. ":8066") so cast
// URLs can be built from the server's detected IP + this port rather than from
// the client-controlled Host header.
func (s *Server) SetListenAddr(addr string) { s.listenAddr = addr }

// SetEnrichRunner attaches the MusicBrainz/CAA enrichment runner that backs the
// /api/enrich endpoints.
func (s *Server) SetEnrichRunner(r *enrich.Runner) { s.enrich = r }

// SetRecommendRunner attaches the external-recommendation runner that backs
// GET /api/discover/recommended. Left nil when no recommendation source is
// configured; the endpoint then returns an empty list.
func (s *Server) SetRecommendRunner(r *recommend.Runner) { s.recommend = r }

// SetScanProgress attaches the library scan progress that backs GET /api/scan.
// Left nil when no library is configured (no scan ever runs).
func (s *Server) SetScanProgress(p *scan.Progress) { s.scan = p }

// SetFedResolver attaches the federation resolver used to proxy audio for
// tracks owned by other peers. Left nil when federation is off.
func (s *Server) SetFedResolver(r fed.Resolver) { s.fedResolver = r }

// SetFedPeers attaches a source of currently-online federation peer ids,
// surfaced via GET /api/config so the UI can grey out offline peers' tracks.
func (s *Server) SetFedPeers(fn func() []string) { s.fedPeers = fn }

// SetMuteLocalOnCast records whether the frontend should mute local audio while
// casting; exposed via GET /api/config.
func (s *Server) SetMuteLocalOnCast(v bool) { s.muteLocalOnCast = v }

// SetInviteEmailer attaches the optional SMTP invite sender.
func (s *Server) SetInviteEmailer(fn func(to, link string)) { s.emailInvite = fn }

// SetSigningSecret records the HMAC secret used to sign Sonos media URLs.
// Loaded once at startup from the store (store.MediaSigningSecret).
func (s *Server) SetSigningSecret(secret []byte) { s.signingSecret = secret }

// RegisterStream attaches a broadcast hub, event bus, and now-playing tracker
// for a shared stream id. np may be nil for streams that don't track current
// track (GET /api/streams/{id} then reports now_playing: null).
func (s *Server) RegisterStream(id string, hub *broadcast.Hub, bus *events.Bus, np *NowPlaying) {
	s.hubs[id] = hub
	s.buses[id] = bus
	if np != nil {
		s.nowPlaying[id] = np
	}
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
	// next/remove/clear/shuffle mutate the queue. They're gated only for the shared
	// house stream (requireAdminShared); each guest's private "me" stream stays open
	// so they can always drive their own queue.
	mux.HandleFunc("GET /api/streams/{id}/next", s.requireAdminShared(s.nextTrack))
	mux.HandleFunc("POST /api/streams/{id}/requests", s.request)
	mux.HandleFunc("DELETE /api/streams/{id}/requests/{trackID}", s.requireAdminShared(s.removeRequest))
	mux.HandleFunc("DELETE /api/streams/{id}/requests", s.requireAdminShared(s.clearRequests))
	mux.HandleFunc("POST /api/streams/{id}/shuffle", s.requireAdminShared(s.setShuffle))
	mux.HandleFunc("GET /api/tracks/{id}/audio", s.trackAudio)
	mux.HandleFunc("GET /api/tracks/{id}/cover", s.trackCover)
	mux.HandleFunc("GET /api/albums/{id}/cover", s.albumCover)
	mux.HandleFunc("GET /api/albums/{id}/tracks", s.albumTracks)
	mux.HandleFunc("GET /stream/", s.streamAudioGuarded)
	mux.HandleFunc("GET /api/streams/{id}/events", s.streamEvents)
	mux.HandleFunc("GET /api/sonos/devices", s.sonosDevices)
	// Casting drives the shared house stream onto the room speakers, so cast/stop
	// and volume changes are admin-only. Discovery, manual add, and reading the
	// volume stay open.
	mux.HandleFunc("POST /api/sonos/cast", s.requireAdmin(s.sonosCast))
	mux.HandleFunc("POST /api/sonos/stop", s.requireAdmin(s.sonosStop))
	mux.HandleFunc("GET /api/sonos/volume", s.sonosGetVolume)
	mux.HandleFunc("POST /api/sonos/volume", s.requireAdmin(s.sonosSetVolume))
	mux.HandleFunc("POST /api/sonos/manual", s.sonosManualAdd)
	mux.HandleFunc("POST /api/auth/signup", s.signup)
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.HandleFunc("GET /api/auth/me", s.me)
	mux.HandleFunc("POST /api/auth/invite/accept", s.inviteAccept)
	mux.HandleFunc("GET /api/discover/rediscover", s.discoverRediscover)
	mux.HandleFunc("GET /api/discover/recent", s.discoverRecent)
	mux.HandleFunc("GET /api/discover/genres", s.discoverGenres)
	mux.HandleFunc("GET /api/discover/recommended", s.discoverRecommended)
	mux.HandleFunc("POST /api/enrich", s.requireAdmin(s.enrichStart))
	mux.HandleFunc("GET /api/enrich", s.enrichStatus)
	mux.HandleFunc("GET /api/scan", s.scanStatus)
	mux.HandleFunc("GET /api/config", s.getConfig)
	mux.HandleFunc("GET /api/streams/{id}/station", s.getStationHandler)
	mux.HandleFunc("POST /api/streams/{id}/station", s.requireAdminShared(s.startStationHandler))
	mux.HandleFunc("DELETE /api/streams/{id}/station", s.requireAdminShared(s.stopStationHandler))
	mux.HandleFunc("GET /api/admin/settings", s.requireAdmin(s.getAdminSettings))
	mux.HandleFunc("POST /api/admin/settings", s.requireAdmin(s.setAdminSettings))
	mux.HandleFunc("POST /api/admin/invites", s.requireAdmin(s.createInvite))
	mux.HandleFunc("GET /api/admin/invites", s.requireAdmin(s.listInvites))
	mux.HandleFunc("DELETE /api/admin/invites/{id}", s.requireAdmin(s.deleteInvite))
	mux.HandleFunc("GET /api/admin/users", s.requireAdmin(s.listUsers))
	mux.HandleFunc("DELETE /api/admin/users/{id}", s.requireAdmin(s.deleteUser))
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
