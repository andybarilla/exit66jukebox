package fed

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
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
	hc := mr.m.HubClient()
	if hc == nil {
		http.Error(w, "hub offline", http.StatusServiceUnavailable)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet,
		fmt.Sprintf("http://@hub/fed/audio/%s/%d", peer, remoteID), nil)
	if err != nil {
		http.Error(w, "build request", http.StatusInternalServerError)
		return
	}
	if rng := r.Header.Get("Range"); rng != "" {
		req.Header.Set("Range", rng)
	}
	resp, err := hc.Do(req)
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

// NewResolverFor returns the Resolver appropriate to the manager's role. The hub
// reuses its single Relay instance (m.Relay); a member proxies over its hub session.
func NewResolverFor(m *Manager) Resolver {
	if m.Role == "hub" {
		return &hubResolver{relay: m.Relay}
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
	// The hub is a peer too: apply to its own DB so its browse shows remote tracks.
	if h.db != nil {
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

func (h *Relay) serveMerged(w http.ResponseWriter, r *http.Request) {
	exclude := r.PathValue("peer")
	h.mu.Lock()
	cats := make(map[string][]store.CatalogRow)
	for peer, rows := range h.catalogs {
		if peer != exclude {
			cats[peer] = rows
		}
	}
	h.mu.Unlock()
	online := h.reg.IDs()
	if h.selfID != "" {
		online = append(online, h.selfID)
		if h.selfID != exclude && h.db != nil {
			if rows, err := store.ExportCatalog(h.db); err == nil && len(rows) > 0 {
				cats[h.selfID] = rows
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(MergedCatalog{Catalogs: cats, Online: online})
}

// Routes returns the hub's federation mux (served over the session to members).
func (h *Relay) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /fed/audio/{peer}/{id}", h)
	mux.HandleFunc("POST /fed/catalog/{peer}", h.receiveCatalog)
	mux.HandleFunc("GET /fed/catalog/{peer}/merged", h.serveMerged)
	return mux
}
