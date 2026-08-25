package fed

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
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
// for a registered peer to drain its mailbox, and how long postSignal waits on
// the HTTP request to the next hop. A peer that is slow to consume, or a session
// that has gone quiet, should not stall the sender's negotiation beyond this —
// nor, on a forwarding hub, hold a goroutine per relayed message.
const signalTimeout = 5 * time.Second

// signalMailboxSize bounds per-peer buffered messages so a stalled peer cannot
// accumulate unbounded signaling traffic.
const signalMailboxSize = 64

// maxSignalBody bounds a relayed SignalMsg. The route is reachable by anything
// holding a federation session, and a signal is never large.
const maxSignalBody = 64 << 10

// hubPeerID is the id a member registers its hub under (Manager.runMember). It
// is not a peer id any instance can claim for itself: it is how the local
// process names "the session I have to my hub".
const hubPeerID = "@hub"

// sessionPeerKey types the request-context value carrying the peer id of the
// federation session a request arrived on.
type sessionPeerKey struct{}

// WithSessionPeer tags every request next serves with the id of the peer whose
// session it arrived on. It wraps the handler per session rather than being part
// of the shared composition, because the id is a property of the connection and
// one handler serves them all (see Manager.serveHubConn).
//
// This is what lets the hub's relay say who a forwarded signal came from. The id
// is the one that session's owner claimed at the token handshake, so it is only
// as strong as that handshake (#167) — but it is the peer the hub is actually
// talking to, which is strictly more than SignalMsg.From, a value the sender
// writes at will.
func WithSessionPeer(peerID string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionPeerKey{}, peerID)))
	})
}

// SessionPeer returns the id of the peer whose session r arrived on, or "" when
// the handler was not wrapped by WithSessionPeer.
func SessionPeer(r *http.Request) string {
	id, _ := r.Context().Value(sessionPeerKey{}).(string)
	return id
}

// SignalForwarder relays a SignalMsg onward to a recipient this process hosts no
// mailbox for, and reports whether it landed. Only the hub composition sets one
// (HubSessionHandler); a peer or member session leaves it nil and refuses such a
// message, which is what bounds relaying to a single hop.
type SignalForwarder func(ctx context.Context, msg SignalMsg) bool

// Signaler holds the WebRTC signaling mailboxes of this process. Every instance
// running the peer role has one; its WebRTC transport registers a single mailbox
// under the instance's own peer id (startedReader) and drains it. Messages
// arrive either in-process or, from another instance, through the
// WithSignalRelay endpoint on this instance's federation session.
//
// Send only ever delivers to a registered mailbox. A message for an id this
// process does not host is refused, except on a hub, whose relay hands it to a
// SignalForwarder instead (#158). Either way the destination comes from a live
// federation session — the Registry — so signaling never reaches an arbitrary
// host.
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
// (posted by WebRTCTransport.postSignal) crosses the process boundary, and fwd
// is nil there.
//
// A hub hosts no mailboxes at all — it creates no WebRTC transport — so every
// recipient it is asked about is remote. fwd is what it does with them: two
// peers that can each only reach the hub negotiate through it (#158). Before
// that the mount was inert on a hub and answered 503 unconditionally.
func WithSignalRelay(s *Signaler, fwd SignalForwarder, next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /fed/signal/{to}", s.signalRelayHandler(fwd))
	if next != nil {
		mux.Handle("/", next)
	}
	return mux
}

// signalRelayHandler returns an http.Handler that accepts a SignalMsg addressed
// to a peer (the path value "to") and either delivers it to that peer's local
// mailbox or, when fwd is set and no such mailbox exists, forwards it onward.
// It is served only over a federation session, so reaching it already required
// the token handshake.
//
// The two paths treat From differently, and the difference is the point.
//
// Local delivery leaves From as the sender wrote it (falling back to the "from"
// query value). It is a claim, not a credential, and always was: what refuses a
// forged one is routeToDial, which matches it against the peer the negotiation
// belongs to.
//
// Forwarding cannot leave it alone. A hub that relays is asserting to the
// recipient who the message came from, and the recipient has no other way to
// check — it arrives on the recipient's hub session, not on the sender's. So
// From is overwritten with the peer id of the session the request arrived on,
// and a request that carries no session identity is refused rather than
// forwarded on the body's word. Trusting the body here would hand any token
// holder the negotiation hijack #163 closed, one layer up: an answer stamped
// with someone else's id lands in that peer's in-flight Dial.
//
// What this does not establish is that the session's own id is genuine; the
// handshake verifies one federation-wide token and takes the id from the
// dialer's claim (#167). The guarantee is "the peer the hub is talking to on
// this session", which is what the recipient's routeToDial can act on.
func (s *Signaler) signalRelayHandler(fwd SignalForwarder) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		to := r.PathValue("to")
		if to == "" {
			http.Error(w, "missing recipient", http.StatusBadRequest)
			return
		}
		var msg SignalMsg
		// A SignalMsg is an SDP or a single ICE candidate; 64 KiB is well clear of
		// both. Unbounded decoding here would let one request hold a goroutine on
		// an arbitrarily long body, and Send can then park it for signalTimeout on
		// a full mailbox, so the two compound.
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSignalBody)).Decode(&msg); err != nil {
			http.Error(w, "bad signal", http.StatusBadRequest)
			return
		}
		// The path is the routing key: it is what the mux matched, what
		// isRegistered is checked against, and what the forward re-addresses. A
		// body naming a different recipient is refused rather than quietly
		// overwritten, so there is never a second source of truth for the same
		// decision that a later change could come to read instead.
		if msg.To != "" && msg.To != to {
			http.Error(w, "recipient mismatch", http.StatusBadRequest)
			return
		}
		msg.To = to
		if msg.From == "" {
			msg.From = r.URL.Query().Get("from")
		}
		if s.isRegistered(to) {
			if !s.Send(msg) {
				http.Error(w, "signal dropped", http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if fwd == nil {
			http.Error(w, "recipient offline", http.StatusServiceUnavailable)
			return
		}
		from := SessionPeer(r)
		if from == "" {
			http.Error(w, "unattributable signal", http.StatusForbidden)
			return
		}
		msg.From = from
		if !fwd(r.Context(), msg) {
			http.Error(w, "recipient offline", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})
}

// hubSignalForwarder returns the SignalForwarder a hub's session relay uses. It
// hands the message to the recipient's own session, where that process's relay
// delivers it to the local mailbox.
//
// reg is what keeps this off arbitrary hosts, the same property postSignal
// relies on: it holds only peers with a live session here, and each Peer.Client
// is bound to that session, so the recipient's id selects a session rather than
// a destination.
//
// Relaying stops after this one hop. What receives the forwarded message is a
// peer or member session, and neither composition carries a forwarder
// (WithSignalRelay is called with nil there), so a recipient that does not host
// the mailbox refuses instead of passing it on. Nothing in the repo serves
// HubSessionHandler to another hub, so no cycle of forwarders can form.
func hubSignalForwarder(reg *Registry, logger *log.Logger) SignalForwarder {
	if logger == nil {
		logger = log.Default()
	}
	return func(ctx context.Context, msg SignalMsg) bool {
		p := reg.Get(msg.To)
		if p == nil || p.Client == nil {
			return false
		}
		return postSignal(ctx, p, msg, logger)
	}
}

// postSignal POSTs msg to /fed/signal/{msg.To} over p's federation session,
// where that process's WithSignalRelay takes it from there. The path names the
// final recipient and the session names the next hop, so the two agree when p is
// the recipient (WebRTCTransport.postSignal) and differ when p is the hub
// relaying on the recipient's behalf (hubSignalForwarder).
//
// ctx bounds the request. It is derived from the caller's — for a forwarding hub
// that is the inbound request's context, so a caller that gives up does not
// leave the hub holding a goroutine and a yamux stream for signalTimeout.
func postSignal(ctx context.Context, p *Peer, msg SignalMsg, logger *log.Logger) bool {
	baseURL := p.BaseURL
	if baseURL == "" {
		baseURL = "http://" + p.ID
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, signalTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/fed/signal/"+url.PathEscape(msg.To), bytes.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.Client.Do(req)
	if err != nil {
		logger.Printf("webrtc signal %s %s via %s: %v", msg.Type, msg.To, p.ID, err)
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusAccepted {
		logger.Printf("webrtc signal %s %s via %s: status %d", msg.Type, msg.To, p.ID, resp.StatusCode)
		return false
	}
	return true
}
