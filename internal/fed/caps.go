package fed

import (
	"bufio"
	"encoding/json"
	"net/http"
)

// Capabilities advertise which optional transports a peer supports. They are
// exchanged only over an already token-authenticated session (see
// fetchCaps) and never broadcast, preserving the hub-relay SSRF-safety
// property that the federation only ever talks to registered peers.
type Capabilities struct {
	// DirectWebRTC is true when the peer can establish direct WebRTC data
	// channels for audio, enabling NAT traversal beyond the yamux TCP direct
	// path.
	DirectWebRTC bool `json:"direct_webrtc"`
	// STUNServers are the STUN URLs the peer will use for ICE; exchanged for
	// diagnostics/compatibility. A working pair needs at least one shared
	// reachable STUN/TURN server.
	STUNServers []string `json:"stun_servers,omitempty"`
}

// capsRoute is the path served over every peer/hub session to fetch the
// remote's capabilities. It returns a single JSON object so the exchange
// mirrors the catalog sync message style.
const capsRoute = "/fed/caps"

// writeCaps replies to a caps request with this instance's capabilities JSON.
func writeCaps(w http.ResponseWriter, c Capabilities) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(c)
}

// WithCapsRoute returns a handler that serves this instance's capabilities at
// capsRoute and delegates everything else to next. It is layered onto every
// federation session handler so a peer can discover the remote's transports
// right after the token-authenticated handshake.
func WithCapsRoute(caps Capabilities, next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+capsRoute, func(w http.ResponseWriter, _ *http.Request) {
		writeCaps(w, caps)
	})
	if next != nil {
		mux.Handle("/", next)
	}
	return mux
}

// fetchCaps retrieves the remote peer's capabilities over an established
// session client. It is best-effort: a transport error or malformed body yields
// a zero Capabilities (meaning "relay only"), so callers fail safe to the
// existing transport cascade.
func fetchCaps(client *http.Client, baseURL string) Capabilities {
	if client == nil || baseURL == "" {
		return Capabilities{}
	}
	resp, err := client.Get(baseURL + capsRoute)
	if err != nil {
		return Capabilities{}
	}
	defer resp.Body.Close()
	var c Capabilities
	// Buffered read so a partial/empty body does not error noisily.
	br := bufio.NewReader(resp.Body)
	if err := json.NewDecoder(br).Decode(&c); err != nil {
		return Capabilities{}
	}
	return c
}
