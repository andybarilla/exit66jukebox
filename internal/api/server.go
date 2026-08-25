package api

import (
	"bytes"
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/auth"
	"github.com/andybarilla/exit66jukebox/internal/enrich"
	"github.com/andybarilla/exit66jukebox/internal/fed"
	"github.com/andybarilla/exit66jukebox/internal/jukebox"
	"github.com/andybarilla/exit66jukebox/internal/recommend"
	"github.com/andybarilla/exit66jukebox/internal/scan"
	"github.com/andybarilla/exit66jukebox/internal/sonos"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

// Server holds dependencies and builds the HTTP handler.
type Server struct {
	db           *sql.DB
	jb           *jukebox.Jukebox
	ui           fs.FS
	listenAddr   string // server's own listen addr, for building Sonos-reachable URLs
	publicOrigin string // trusted browser-facing origin for email links
	// Shared-stream broadcast pipelines (hub + bus + now-playing), one per
	// shared stream. house's is registered at boot; the rest are built on first
	// use by newPipe and reaped once idle. See pipeline.go.
	pipesMu   sync.Mutex
	pipes     map[string]*StreamPipeline
	pipeCtx   context.Context
	newPipe   func(id string) *StreamPipeline
	idleTTL   time.Duration
	idleCheck time.Duration
	// pipeWG tracks the goroutines of lazily-started pipelines so shutdown can
	// wait for their ffmpeg children to be killed. See WaitForPipelines.
	pipeWG      sync.WaitGroup
	enrich      *enrich.Runner    // nil until SetEnrichRunner; endpoints 503 while nil
	recommend   *recommend.Runner // nil until SetRecommendRunner; endpoint returns [] while nil
	scanMu      sync.Mutex
	scan        *scan.Progress // nil until SetScanProgress (no library); endpoint 503 while nil
	scanWorkers int
	fedResolver fed.Resolver    // nil unless federation is configured
	fedPeers    func() []string // returns online peer ids; nil when federation off
	activeFed   store.FederationSettings

	// muteLocalOnCast is exposed via GET /api/config so the frontend can mute the
	// local <audio> while a Sonos is casting the stream that browser is playing.
	// Sourced from config (env for now).
	muteLocalOnCast bool

	// signingSecret is the HMAC secret used to sign Sonos media URLs; loaded once
	// at startup from the store (store.MediaSigningSecret).
	signingSecret []byte
	mfaKey        []byte

	// sso holds the enabled sign-in providers in the order the sign-in surface
	// offers them, and ssoByID indexes the same values by their route segment.
	// Both are empty when sign-in through a provider is off (see SetSSO), and
	// both are written once at startup and only read afterwards.
	sso     []*ssoProvider
	ssoByID map[string]*ssoProvider
	// bootstrapTokenHash is the armed first-admin bootstrap token, hashed;
	// empty once claimed. Read by concurrent signups, cleared by the winner.
	bootstrapMu        sync.RWMutex
	bootstrapTokenHash string
	bootstrapClaimed   bool
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
	emailInvite        func(to, link string)
	emailPasswordReset func(to, link string)
	emailVerification  func(to, link string) error

	// manualVerify confirms a manually-entered IP actually serves a Sonos
	// descriptor before it's trusted (injectable for tests).
	manualVerify func(ip string) (name string, ok bool)

	// sonosDiscover runs SSDP multicast discovery; scanUnicast is the unicast /24
	// fallback used when SSDP finds nothing (both injectable for tests).
	sonosDiscover func() ([]sonos.Device, error)
	scanUnicast   func() []sonos.Device

	// castTo points a speaker at a stream URL, stopCast stops one, and deviceURI
	// reads back the URI a speaker reports plus whether it is playing it. The
	// read-back is how a device-to-stream mapping is answered without storing
	// one (#130). All three are injectable so tests exercise the handlers
	// without a player on the LAN.
	castTo    func(ip, url, title string) error
	stopCast  func(ip string) error
	deviceURI func(ip string) (uri string, playing bool, err error)

	// hostIPs reports the addresses this host answers on, so a URI a speaker
	// reports can be told apart from another jukebox's on the same LAN.
	// Injectable: tests pin it rather than depending on the machine.
	hostIPs func() []string
}

func NewServer(db *sql.DB, jb *jukebox.Jukebox, ui fs.FS) *Server {
	return &Server{
		db: db, jb: jb, ui: ui,
		pipes:         make(map[string]*StreamPipeline),
		idleTTL:       defaultStreamIdleTTL,
		idleCheck:     defaultStreamIdleCheck,
		loginAttempts: make(map[string][]int64),
		sonosIPs:      make(map[string]bool),
		sonosManual:   make(map[string]string),
		manualVerify: func(ip string) (string, bool) {
			return sonos.Verify(sonos.DescriptorURL(ip))
		},
		sonosDiscover: func() ([]sonos.Device, error) {
			return sonos.Discover(2 * time.Second)
		},
		scanUnicast: func() []sonos.Device {
			return sonos.ScanUnicast(sonos.OutboundIP(), 200*time.Millisecond)
		},
		castTo:    sonos.Cast,
		stopCast:  sonos.Stop,
		deviceURI: sonos.NowPlaying,
		hostIPs:   localIPs,
	}
}

// SetListenAddr records the server's own listen address (e.g. ":8066") so cast
// URLs can be built from the server's detected IP + this port rather than from
// the client-controlled Host header.
func (s *Server) SetListenAddr(addr string) { s.listenAddr = addr }

func (s *Server) SetPublicOrigin(origin string) { s.publicOrigin = strings.TrimRight(origin, "/") }

// SetEnrichRunner attaches the MusicBrainz/CAA enrichment runner that backs the
// /api/enrich endpoints.
func (s *Server) SetEnrichRunner(r *enrich.Runner) { s.enrich = r }

// SetRecommendRunner attaches the external-recommendation runner that backs
// GET /api/discover/recommended. Left nil when no recommendation source is
// configured; the endpoint then returns an empty list.
func (s *Server) SetRecommendRunner(r *recommend.Runner) { s.recommend = r }

// SetScanProgress attaches the library scan progress that backs GET /api/scan.
// Left nil when no library is configured (no scan ever runs).
func (s *Server) SetScanProgress(p *scan.Progress) {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	s.scan = p
}

func (s *Server) SetScanWorkers(n int) { s.scanWorkers = n }

// SetFedResolver attaches the federation resolver used to proxy audio for
// tracks owned by other peers. Left nil when federation is off.
func (s *Server) SetFedResolver(r fed.Resolver) { s.fedResolver = r }

// SetFedPeers attaches a source of currently-online federation peer ids,
// surfaced via GET /api/config so the UI can grey out offline peers' tracks.
func (s *Server) SetFedPeers(fn func() []string) { s.fedPeers = fn }

func (s *Server) SetActiveFederation(settings store.FederationSettings) { s.activeFed = settings }

// SetMuteLocalOnCast records whether the frontend should mute local audio while
// casting; exposed via GET /api/config.
func (s *Server) SetMuteLocalOnCast(v bool) { s.muteLocalOnCast = v }

// SetInviteEmailer attaches the optional SMTP invite sender.
func (s *Server) SetInviteEmailer(fn func(to, link string)) { s.emailInvite = fn }

func (s *Server) SetPasswordResetEmailer(fn func(to, link string)) { s.emailPasswordReset = fn }

func (s *Server) SetVerificationEmailer(fn func(to, link string) error) { s.emailVerification = fn }

// SetSigningSecret records the HMAC secret used to sign Sonos media URLs.
// Loaded once at startup from the store (store.MediaSigningSecret).
func (s *Server) SetSigningSecret(secret []byte) { s.signingSecret = secret }

func (s *Server) SetMFAKey(key []byte) { s.mfaKey = append([]byte(nil), key...) }

// bootstrapStatus is what a presented bootstrap token entitles a caller to.
// Claimed is kept distinct from invalid so a caller holding the real token is
// told the bootstrap is gone rather than that its token is wrong.
type bootstrapStatus int

const (
	bootstrapInvalid bootstrapStatus = iota
	bootstrapValid
	bootstrapClaimed
)

// SetBootstrapToken arms the one-time first-admin bootstrap token and returns
// the URL an operator opens to claim it. The token itself is never persisted,
// so a restart before the first admin exists mints a fresh one.
func (s *Server) SetBootstrapToken(token string) string {
	s.bootstrapMu.Lock()
	s.bootstrapTokenHash = auth.HashToken(token)
	s.bootstrapClaimed = false
	s.bootstrapMu.Unlock()
	return s.publicBaseURL() + "/?bootstrap_token=" + url.QueryEscape(token)
}

// markBootstrapClaimed disarms bootstrap once the first admin exists, so
// deleting every account while the process is still running can't re-arm it
// with the old token.
func (s *Server) markBootstrapClaimed() {
	s.bootstrapMu.Lock()
	s.bootstrapTokenHash = ""
	s.bootstrapClaimed = true
	s.bootstrapMu.Unlock()
}

func (s *Server) bootstrapTokenStatus(token string) bootstrapStatus {
	s.bootstrapMu.RLock()
	hash, claimed := s.bootstrapTokenHash, s.bootstrapClaimed
	s.bootstrapMu.RUnlock()
	if claimed {
		return bootstrapClaimed
	}
	if hash == "" || token == "" {
		return bootstrapInvalid
	}
	if subtle.ConstantTimeCompare([]byte(hash), []byte(auth.HashToken(token))) == 1 {
		return bootstrapValid
	}
	return bootstrapInvalid
}

// Handler returns the routed mux. Handlers live in sibling files.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/artists", s.listArtists)
	mux.HandleFunc("GET /api/albums", s.listAlbums)
	mux.HandleFunc("GET /api/tracks", s.listTracks)
	// Stream CRUD. Listing is open; create/rename/delete are admin-gated on the
	// same terms as the other shared-stream controls (including the open-mode
	// escape hatch), because every one of them creates or destroys a shared stream.
	mux.HandleFunc("GET /api/streams", s.listStreams)
	mux.HandleFunc("POST /api/streams", s.requireAdminOrOpen(s.createStream))
	// Rename and delete take the shared-only gate: requireAdminShared lets a
	// private stream through ungated so a listener can drive their own queue,
	// and that must not extend to destroying a stream.
	mux.HandleFunc("PATCH /api/streams/{id}", s.personalStreamNoProvision(s.requireAdminOnSharedOnly(s.renameStream)))
	mux.HandleFunc("DELETE /api/streams/{id}", s.personalStreamNoProvision(s.requireAdminOnSharedOnly(s.deleteStream)))
	mux.HandleFunc("GET /api/streams/{id}", s.personalStream(s.getStream))
	// next/remove/clear/shuffle mutate the queue. requireAdminShared gates them on
	// the stream's kind, so every shared stream is admin-only; a caller's own
	// personal stream stays open so they can always drive their own queue.
	// resolvePersonalStream runs first on every {id} route: it turns the alias
	// into the caller's own id and refuses every other route into a private
	// stream, so the ungated fall-through below can only ever be the caller's.
	mux.HandleFunc("POST /api/streams/{id}/next", s.personalStream(s.requireAdminShared(s.nextTrack)))
	mux.HandleFunc("POST /api/streams/{id}/requests", s.personalStream(s.request))
	mux.HandleFunc("DELETE /api/streams/{id}/requests/{trackID}", s.personalStream(s.requireAdminShared(s.removeRequest)))
	mux.HandleFunc("DELETE /api/streams/{id}/requests", s.personalStream(s.requireAdminShared(s.clearRequests)))
	mux.HandleFunc("POST /api/streams/{id}/shuffle", s.personalStream(s.requireAdminShared(s.setShuffle)))
	mux.HandleFunc("GET /api/tracks/{id}/audio", s.trackAudio)
	mux.HandleFunc("GET /api/tracks/{id}/cover", s.trackCover)
	mux.HandleFunc("GET /api/albums/{id}/cover", s.albumCover)
	mux.HandleFunc("GET /api/albums/{id}/tracks", s.albumTracks)
	mux.HandleFunc("GET /stream/", s.streamAudioGuarded)
	mux.HandleFunc("GET /api/streams/{id}/events", s.personalStream(s.streamEvents))
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
	mux.HandleFunc("POST /api/auth/mfa/complete", s.mfaComplete)
	mux.HandleFunc("POST /api/auth/mfa/enroll/begin", s.mfaEnrollBegin)
	mux.HandleFunc("POST /api/auth/mfa/enroll/confirm", s.mfaEnrollConfirm)
	mux.HandleFunc("POST /api/auth/mfa/disable", s.mfaDisable)
	mux.HandleFunc("POST /api/auth/mfa/recovery/regenerate", s.mfaRecoveryRegenerate)
	mux.HandleFunc("GET /api/auth/profiles", s.listPasswordlessProfiles)
	mux.HandleFunc("POST /api/auth/profiles", s.createPasswordlessProfile)
	mux.HandleFunc("POST /api/auth/profiles/select", s.selectPasswordlessProfile)
	mux.HandleFunc("GET /api/auth/sso/{provider}/start", s.ssoStart)
	mux.HandleFunc("GET "+ssoCallbackPath, s.ssoCallback)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.HandleFunc("GET /api/auth/me", s.me)
	mux.HandleFunc("POST /api/auth/invite/accept", s.inviteAccept)
	mux.HandleFunc("POST /api/auth/verify-email", s.verifyEmail)
	mux.HandleFunc("POST /api/auth/password-reset/forgot", s.forgotPassword)
	mux.HandleFunc("POST /api/auth/password-reset/redeem", s.resetPassword)
	mux.HandleFunc("GET /api/discover/rediscover", s.discoverRediscover)
	mux.HandleFunc("GET /api/discover/recent", s.discoverRecent)
	mux.HandleFunc("GET /api/discover/genres", s.discoverGenres)
	mux.HandleFunc("GET /api/discover/recommended", s.discoverRecommended)
	mux.HandleFunc("POST /api/enrich", s.requireAdmin(s.enrichStart))
	mux.HandleFunc("GET /api/enrich", s.enrichStatus)
	mux.HandleFunc("GET /api/scan", s.scanStatus)
	mux.HandleFunc("GET /api/config", s.getConfig)
	mux.HandleFunc("GET /api/streams/{id}/station", s.personalStream(s.getStationHandler))
	mux.HandleFunc("POST /api/streams/{id}/station", s.personalStream(s.requireAdminShared(s.startStationHandler)))
	mux.HandleFunc("DELETE /api/streams/{id}/station", s.personalStream(s.requireAdminShared(s.stopStationHandler)))
	mux.HandleFunc("GET /api/admin/settings", s.requireAdmin(s.getAdminSettings))
	mux.HandleFunc("POST /api/admin/settings", s.requireAdmin(s.setAdminSettings))
	mux.HandleFunc("GET /api/admin/libraries", s.requireAdmin(s.getAdminLibraries))
	mux.HandleFunc("POST /api/admin/libraries", s.requireAdmin(s.setAdminLibraries))
	mux.HandleFunc("GET /api/admin/library-paths", s.requireAdmin(s.listLibraryPaths))
	mux.HandleFunc("GET /api/admin/federation/peers", s.requireAdmin(s.listFederationPeers))
	mux.HandleFunc("POST /api/admin/federation/peers", s.requireAdmin(s.addFederationPeer))
	mux.HandleFunc("POST /api/admin/federation/peers/{peerID}/approve", s.requireAdmin(s.approveFederationPeer))
	mux.HandleFunc("GET /api/admin/federation/groups", s.requireAdmin(s.listFederationGroups))
	mux.HandleFunc("POST /api/admin/federation/groups", s.requireAdmin(s.createFederationGroup))
	mux.HandleFunc("DELETE /api/admin/federation/groups/{id}", s.requireAdmin(s.deleteFederationGroup))
	mux.HandleFunc("POST /api/admin/federation/groups/{id}/members", s.requireAdmin(s.addFederationGroupMember))
	mux.HandleFunc("DELETE /api/admin/federation/groups/{id}/members/{peerID}", s.requireAdmin(s.removeFederationGroupMember))
	mux.HandleFunc("POST /api/admin/invites", s.requireAdmin(s.createInvite))
	mux.HandleFunc("GET /api/admin/invites", s.requireAdmin(s.listInvites))
	mux.HandleFunc("DELETE /api/admin/invites/{id}", s.requireAdmin(s.deleteInvite))
	mux.HandleFunc("GET /api/admin/users", s.requireAdmin(s.listUsers))
	mux.HandleFunc("POST /api/admin/users/{id}/password-reset", s.requireAdmin(s.createPasswordReset))
	mux.HandleFunc("POST /api/admin/users/{id}/email-verification", s.requireAdmin(s.createEmailVerification))
	mux.HandleFunc("DELETE /api/admin/users/{id}", s.requireAdmin(s.deleteUser))
	if s.ui != nil {
		mux.HandleFunc("GET /admin", s.serveUIIndex)
		mux.HandleFunc("GET /verify/", s.serveUIIndex)
		mux.HandleFunc("GET /invite/", s.serveUIIndex)
		mux.HandleFunc("GET /reset-password/", s.serveUIIndex)
		mux.Handle("GET /", http.FileServerFS(s.ui))
	}
	return mux
}

func (s *Server) serveUIIndex(w http.ResponseWriter, r *http.Request) {
	index, err := fs.ReadFile(s.ui, "index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}

	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(index))
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
