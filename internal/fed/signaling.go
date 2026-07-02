package fed

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// SignalMsg is one WebRTC negotiation message relayed between two peers through
// the signaling channel. Type is "offer", "answer", or "ice".
type SignalMsg struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"` // "offer" | "answer" | "ice"
	// SID correlates the messages of a single negotiation. The offerer mints it,
	// the answer echoes it on the answer and ICE so the offerer routes the reply
	// to the right in-flight Dial.
	SID string `json:"sid,omitempty"`
	// SDP carries the session description for offer/answer messages.
	SDP string `json:"sdp,omitempty"`
	// ICECandidate carries a single trickle ICE candidate (JSON) for type=="ice".
	ICECandidate string `json:"ice,omitempty"`
}

// signalTimeout is how long Send waits for a peer to drain its mailbox. A peer
// that is registered but slow to consume should not stall the sender's
// negotiation beyond this.
const signalTimeout = 5 * time.Second

// signalMailboxSize bounds per-peer buffered messages so a stalled peer cannot
// accumulate unbounded signaling traffic.
const signalMailboxSize = 64

// Signaler relays WebRTC signaling messages between authenticated peers. The
// hub runs one Signaler and routes messages from a connected peer to another
// connected peer's mailbox; each peer's WebRTC transport drains its mailbox.
// Messages are only delivered to peers that have been registered (via
// Register), which for the hub means peers with a live authenticated session —
// preserving the property that signaling never reaches an arbitrary host.
type Signaler struct {
	mu        sync.Mutex
	mailboxes map[string]chan SignalMsg
}

// NewSignaler returns an empty Signaler.
func NewSignaler() *Signaler {
	return &Signaler{mailboxes: make(map[string]chan SignalMsg)}
}

// Register creates a mailbox for peerID and returns the channel that delivers
// inbound messages to it. Calling Register twice replaces the mailbox.
func (s *Signaler) Register(peerID string) chan SignalMsg {
	ch := make(chan SignalMsg, signalMailboxSize)
	s.mu.Lock()
	old := s.mailboxes[peerID]
	s.mailboxes[peerID] = ch
	s.mu.Unlock()
	if old != nil {
		close(old)
	}
	return ch
}

// Unregister removes peerID's mailbox. The ch parameter guards against removing
// a newer mailbox registered under the same id (same identity-guard pattern as
// Registry.remove).
func (s *Signaler) Unregister(peerID string, ch chan SignalMsg) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cur, ok := s.mailboxes[peerID]; ok && cur == ch {
		close(cur)
		delete(s.mailboxes, peerID)
	}
}

// isRegistered reports whether peerID has an active mailbox. Held under s.mu.
func (s *Signaler) isRegistered(peerID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.mailboxes[peerID]
	return ok
}

// Send delivers msg to its recipient's mailbox. It returns false (without
// blocking long) if the recipient is unknown or its mailbox is full/unreachable
// — callers treat that as a negotiation failure and fall back to the next
// transport tier.
func (s *Signaler) Send(msg SignalMsg) bool {
	s.mu.Lock()
	ch, ok := s.mailboxes[msg.To]
	s.mu.Unlock()
	if !ok {
		return false
	}
	timer := time.NewTimer(signalTimeout)
	defer timer.Stop()
	select {
	case ch <- msg:
		return true
	case <-timer.C:
		return false
	}
}

// WithSignalRelay mounts the WebRTC signaling relay endpoint onto next and
// returns the composed handler. The same *Signaler must be the one the Manager
// uses (so the recipient's mailbox, registered by the local transport, is the
// one this handler delivers into). The route accepts a SignalMsg addressed via
// the path value "to" and relays it to that peer's mailbox.
func WithSignalRelay(s *Signaler, next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /fed/signal/{to}", s.signalRelayHandler())
	if next != nil {
		mux.Handle("/", next)
	}
	return mux
}

// signalRelayHandler returns an http.Handler that accepts a SignalMsg addressed
// to a peer (the path value "to") and relays it through s. The sender's id is
// taken from the "from" query value (set by the hub relay after it has
// authenticated the session); the body overrides it. This handler is mounted on
// the hub session handler so any connected peer can signal any other.
func (s *Signaler) signalRelayHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		to := r.PathValue("to")
		if to == "" {
			http.Error(w, "missing recipient", http.StatusBadRequest)
			return
		}
		var msg SignalMsg
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			http.Error(w, "bad signal", http.StatusBadRequest)
			return
		}
		msg.To = to
		if msg.From == "" {
			msg.From = r.URL.Query().Get("from")
		}
		if !s.isRegistered(to) {
			http.Error(w, "recipient offline", http.StatusServiceUnavailable)
			return
		}
		if !s.Send(msg) {
			http.Error(w, "signal dropped", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})
}

// signalPollHandler lets a peer pull its inbound messages one at a time over
// HTTP. Each call blocks until a message arrives or the context is done. This
// is the hubless fallback path (a peer role with no hub still needs to exchange
// signaling over a direct yamux session; the long-poll fits that model).
func (s *Signaler) signalPollHandler(peerID string) http.Handler {
	ch := s.Register(peerID)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case msg, ok := <-ch:
			if !ok {
				http.Error(w, "mailbox closed", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(msg)
		case <-r.Context().Done():
			http.Error(w, "timeout", http.StatusServiceUnavailable)
		}
	})
}
