package fed

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
)

// Relay is the hub-side handler. It reverse-proxies GET /fed/audio/{peer}/{id}
// to the owning peer's /api/tracks/{id}/audio over that peer's session
// (forwarding Range, copying 206 + body back). db is the hub's own database —
// the hub is a peer too, so received catalogs are applied to it (a later task).
// db may be nil in tests that exercise only audio relay.
type Relay struct {
	reg *Registry
	db  *sql.DB
}

func NewRelay(reg *Registry, db *sql.DB) *Relay { return &Relay{reg: reg, db: db} }

func (h *Relay) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	peerID := r.PathValue("peer")
	remoteID := r.PathValue("id")
	if peerID == "" || remoteID == "" {
		http.Error(w, "bad fed audio path", http.StatusBadRequest)
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

// Routes returns the hub's federation mux (served over the session to members).
func (h *Relay) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /fed/audio/{peer}/{id}", h)
	return mux
}
