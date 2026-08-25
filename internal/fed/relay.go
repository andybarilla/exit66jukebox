package fed

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

// Relay is the hub-side handler. It reverse-proxies GET /fed/audio/{peer}/{id}
// to the owning peer's /api/tracks/{id}/audio over that peer's session
// (forwarding Range, copying 206 + body back). db is the hub's own database —
// the hub is a peer too, so received catalogs are applied to it (a later task).
// db may be nil in tests that exercise only audio relay.
type Relay struct {
	reg      *Registry
	db       *sql.DB
	selfID   string
	mu       sync.Mutex
	catalogs map[string][]store.CatalogRow // peer -> its rows (for fan-out)
}

func NewRelay(reg *Registry, db *sql.DB) *Relay {
	return &Relay{reg: reg, db: db, catalogs: make(map[string][]store.CatalogRow)}
}

// SetSelf records the hub's own peer id so it publishes its local library to
// members and serves its own tracks directly — the hub is a peer too, but it is
// not present in its own registry.
func (h *Relay) SetSelf(peerID string) { h.selfID = peerID }

func (h *Relay) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	peerID := r.PathValue("peer")
	remoteID := r.PathValue("id")
	if peerID == "" || remoteID == "" {
		http.Error(w, "bad fed audio path", http.StatusBadRequest)
		return
	}
	// The hub's own tracks aren't served over a session (the hub isn't in its
	// registry); serve them from local disk, mirroring api.trackAudio's local path.
	if h.selfID != "" && peerID == h.selfID {
		id, err := strconv.ParseInt(remoteID, 10, 64)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		t, path, ok := store.GetTrack(h.db, id)
		if !ok || t.SourcePeer != "" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.ServeFile(w, r, path)
		return
	}
	peer := h.reg.Get(peerID)
	if peer == nil {
		http.Error(w, "peer offline", http.StatusServiceUnavailable)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet,
		fmt.Sprintf("http://%s/api/tracks/%s/audio", peerID, remoteID), nil)
	if err != nil {
		http.Error(w, "build request", http.StatusInternalServerError)
		return
	}
	if rng := r.Header.Get("Range"); rng != "" {
		req.Header.Set("Range", rng)
	}
	resp, err := peer.Client.Do(req)
	if err != nil {
		http.Error(w, "peer fetch failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for _, k := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
		if v := resp.Header.Get(k); v != "" {
			w.Header().Set(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// memberResolver implements Resolver on the member side by proxying to the hub's
// relay over the hub session.
type memberResolver struct{ m *Manager }

func (mr *memberResolver) ServeRemoteAudio(w http.ResponseWriter, r *http.Request, peer string, remoteID int64) {
	hubPeer := mr.m.Registry.Get("@hub")
	if hubPeer == nil || hubPeer.Client == nil {
		http.Error(w, "hub offline", http.StatusServiceUnavailable)
		return
	}
	baseURL := hubPeer.BaseURL
	if baseURL == "" {
		baseURL = "http://@hub"
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet,
		fmt.Sprintf("%s/fed/audio/%s/%d", baseURL, peer, remoteID), nil)
	if err != nil {
		http.Error(w, "build request", http.StatusInternalServerError)
		return
	}
	if rng := r.Header.Get("Range"); rng != "" {
		req.Header.Set("Range", rng)
	}
	resp, err := hubPeer.Client.Do(req)
	if err != nil {
		http.Error(w, "hub fetch failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for _, k := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
		if v := resp.Header.Get(k); v != "" {
			w.Header().Set(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
	log.Printf("fed audio %s/%d transport=relay", peer, remoteID)
}

// hubResolver implements Resolver on the hub side — the hub IS the relay, so it
// calls it directly rather than through a session.
type hubResolver struct{ relay *Relay }

func (hr *hubResolver) ServeRemoteAudio(w http.ResponseWriter, r *http.Request, peer string, remoteID int64) {
	req := r.Clone(r.Context())
	req.SetPathValue("peer", peer)
	req.SetPathValue("id", fmt.Sprintf("%d", remoteID))
	hr.relay.ServeHTTP(w, req)
}

type directResolver struct {
	reg    *Registry
	hub    *memberResolver
	webrtc *WebRTCTransport // nil when direct P2P is disabled or unsupported
}

// triedWebRTC reports whether the WebRTC transport should be attempted for peer:
// it must be configured and both this peer and the remote must advertise direct
// WebRTC capability.
func (dr *directResolver) triedWebRTC(p *Peer) bool {
	return dr.webrtc != nil && p != nil && p.Caps.DirectWebRTC
}

// serveWebRTC attempts the WebRTC direct transport tier. Returns ok=true if it
// wrote a response (success or a hard error); ok=false means the caller should
// fall through to the next tier. On a successful stream the response is fully
// written; on a setup failure ok=false signals fallback without writing.
func (dr *directResolver) serveWebRTC(w http.ResponseWriter, r *http.Request, peer string, remoteID int64) bool {
	conn, err := dr.webrtc.Dial(r.Context(), peer)
	if err != nil {
		// Setup failed: fall back silently to the next tier (do not write).
		return false
	}
	if err := serveAudioRequest(conn, w, remoteID, r.Header.Get("Range")); err != nil {
		// A mid-stream failure after headers/body started cannot be retried on
		// another tier; the client sees a truncated response. Log and stop.
		log.Printf("fed audio %s/%d transport=webrtc stream error: %v", peer, remoteID, err)
		return true
	}
	log.Printf("fed audio %s/%d transport=webrtc", peer, remoteID)
	return true
}

func (dr *directResolver) ServeRemoteAudio(w http.ResponseWriter, r *http.Request, peer string, remoteID int64) {
	p := dr.reg.Get(peer)
	// Tier 1: WebRTC direct (NAT-traversing). Attempted only when both peers
	// advertise support and direct P2P is enabled.
	if dr.triedWebRTC(p) {
		if dr.serveWebRTC(w, r, peer, remoteID) {
			return
		}
	}
	if p == nil || p.Client == nil {
		if dr.hub != nil {
			dr.hub.ServeRemoteAudio(w, r, peer, remoteID)
			return
		}
		http.Error(w, "peer offline", http.StatusServiceUnavailable)
		return
	}
	// Tier 2: yamux TCP direct path (existing) — works on LAN / routable peers.
	baseURL := p.BaseURL
	if baseURL == "" {
		baseURL = "http://" + peer
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet,
		fmt.Sprintf("%s/api/tracks/%d/audio", baseURL, remoteID), nil)
	if err != nil {
		http.Error(w, "build request", http.StatusInternalServerError)
		return
	}
	if rng := r.Header.Get("Range"); rng != "" {
		req.Header.Set("Range", rng)
	}
	resp, err := p.Client.Do(req)
	if err != nil {
		// Tier 3: hub relay fallback.
		if dr.hub != nil {
			dr.hub.ServeRemoteAudio(w, r, peer, remoteID)
			return
		}
		http.Error(w, "peer fetch failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for _, k := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
		if v := resp.Header.Get(k); v != "" {
			w.Header().Set(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
	log.Printf("fed audio %s/%d transport=direct", peer, remoteID)
}

// NewResolverFor returns the Resolver appropriate to the manager's role. The hub
// reuses its single Relay instance (m.Relay); a member proxies over its hub session.
func NewResolverFor(m *Manager) Resolver {
	if m.Role == "hub" {
		return &hubResolver{relay: m.Relay}
	}
	if m.Role == "peer" {
		return &directResolver{reg: m.Registry, hub: &memberResolver{m: m}, webrtc: m.WebRTC}
	}
	return &memberResolver{m: m}
}

func (h *Relay) receiveCatalog(w http.ResponseWriter, r *http.Request) {
	peer := r.PathValue("peer")
	var rows []store.CatalogRow
	if err := json.NewDecoder(r.Body).Decode(&rows); err != nil {
		http.Error(w, "bad catalog", http.StatusBadRequest)
		return
	}
	h.mu.Lock()
	h.catalogs[peer] = rows
	h.mu.Unlock()
	// The hub is a peer too: apply to its own DB so its browse shows remote
	// tracks — but only for a peer it shares a listening group with. The rows
	// still go into h.catalogs above, because the hub fans them out to members
	// whose own membership is decided separately in serveMerged.
	if h.db != nil {
		if !catalogVisible(h.db, peer, h.selfID) {
			// Applying nothing would leave rows cached while the group still
			// permitted them; ApplyCatalog with no rows deletes them instead.
			rows = nil
		}
		if err := ApplyCatalog(h.db, peer, rows); err != nil {
			http.Error(w, "apply catalog", http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// MergedCatalog is the fan-out payload: every peer's rows except the requester's
// own, plus the ids of all currently-online peers so members can grey out the
// offline ones (a member's local registry only knows the hub, not its siblings).
type MergedCatalog struct {
	Catalogs map[string][]store.CatalogRow `json:"catalogs"`
	Online   []string                      `json:"online"`
}

// serveMerged fans the hub's cached catalogs out to one member, scoped to the
// listening groups the HUB stores: in a hub topology the hub is the authority on
// who discovers whom, since members never talk to each other directly.
//
// A peer the hub holds no rows for is omitted, not emptied — the cache is
// in-memory, so after a hub restart every peer is absent until it re-pushes and
// a member that deleted on absence would wipe its whole remote library. Denial
// is the one case that must be named explicitly (with an empty catalog), so
// revoking membership deletes what a peer already cached.
func (h *Relay) serveMerged(w http.ResponseWriter, r *http.Request) {
	// The session's handshake id wins over the one in the path: both are only
	// claimed (#167), but the path is chosen per request, so scoping on it would
	// let any member read another group's catalogs by asking for them by name.
	exclude := r.PathValue("peer")
	if sessionPeer := RequestPeerID(r); sessionPeer != "" {
		exclude = sessionPeer
	}
	h.mu.Lock()
	cached := make(map[string][]store.CatalogRow, len(h.catalogs))
	for peer, rows := range h.catalogs {
		cached[peer] = rows
	}
	h.mu.Unlock()
	online := h.reg.IDs()
	if h.selfID != "" {
		online = append(online, h.selfID)
		if h.db != nil {
			if rows, err := store.ExportCatalog(h.db); err == nil && len(rows) > 0 {
				cached[h.selfID] = rows
			}
		}
	}
	cats := make(map[string][]store.CatalogRow, len(cached))
	for peer, rows := range cached {
		if peer == exclude {
			continue
		}
		if catalogVisible(h.db, peer, exclude) {
			cats[peer] = rows
			continue
		}
		cats[peer] = []store.CatalogRow{}
	}
	// A denied peer the hub holds no rows for (offline since the hub restarted)
	// would otherwise be omitted, leaving the member's cached rows in place.
	for _, peer := range h.deniedAcceptedPeers(exclude) {
		if _, named := cats[peer]; !named {
			cats[peer] = []store.CatalogRow{}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(MergedCatalog{Catalogs: cats, Online: online})
}

// deniedAcceptedPeers returns the accepted peers viewer shares no group with.
// It is only a backstop for peers absent from the in-memory cache; peers that
// are online repopulate it on their next push and are denied by the main loop.
func (h *Relay) deniedAcceptedPeers(viewer string) []string {
	if h.db == nil {
		return nil
	}
	peers, err := store.ListFederationPeers(h.db, store.PeerStatusAccepted)
	if err != nil {
		return nil
	}
	var denied []string
	for _, p := range peers {
		if p.PeerID == viewer {
			continue
		}
		if !catalogVisible(h.db, p.PeerID, viewer) {
			denied = append(denied, p.PeerID)
		}
	}
	return denied
}

// Routes returns the hub's federation mux (served over the session to members).
func (h *Relay) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /fed/audio/{peer}/{id}", h)
	mux.HandleFunc("POST /fed/catalog/{peer}", h.receiveCatalog)
	mux.HandleFunc("GET /fed/catalog/{peer}/merged", h.serveMerged)
	return mux
}
