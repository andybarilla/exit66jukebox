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

// signalTimeout bounds both ways a single signal can stall: how long Send waits
// for a registered peer to drain its mailbox, and how long
// WebRTCTransport.postSignal waits on the HTTP request to a remote peer. A peer
// that is slow to consume, or a session that has gone quiet, should not stall
// the sender's negotiation beyond this.
const signalTimeout = 5 * time.Second

// signalMailboxSize bounds per-peer buffered messages so a stalled peer cannot
// accumulate unbounded signaling traffic.
const signalMailboxSize = 64

// Signaler holds the WebRTC signaling mailboxes of this process. Every instance
// running the peer role has one; its WebRTC transport registers a single mailbox
// under the instance's own peer id (startedReader) and drains it. Messages
// arrive either in-process or, from another instance, through the
// WithSignalRelay endpoint on this instance's federation session.
//
// Send only ever delivers to a registered mailbox, so a message for an id this
// process does not host is refused rather than forwarded — signaling never
// reaches an arbitrary host.
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

// Send delivers msg to its recipient's mailbox, and reports whether it landed.
// An unknown recipient is refused immediately; a known but full mailbox blocks
// for up to signalTimeout before Send gives up. Callers treat false as a
// negotiation failure and fall back to the next transport tier — which is why
// WebRTCTransport.sendSignal checks isRegistered first rather than reading this
// return value, so an absent recipient goes to the wire without waiting out a
// full one.
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
// returns the composed handler. It is layered onto the session handler of both
// the hub and the peer role, and s must be the same *Signaler the Manager uses,
// so the recipient's mailbox — registered by this process's transport — is the
// one this handler delivers into. The route accepts a SignalMsg addressed via
// the path value "to" and relays it to that mailbox.
//
// On a peer session the only mailbox that exists is the peer's own id, so this
// is in practice "deliver to me": it is how a remote peer's offer/answer/ICE
// (posted by WebRTCTransport.postSignal) crosses the process boundary. A hub
// hosts no mailboxes at all — it creates no WebRTC transport — so the mount
// there currently answers 503 for every recipient (issue #158).
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
// taken from the body, falling back to the "from" query value. It is served only
// over a federation session, so reaching it already required the token
// handshake; From is not otherwise verified, and a peer may claim another peer's
// id in it (see the PR for #152).
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
