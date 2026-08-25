package fed

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/webrtc/v4"
)

// WebRTC direct transport for federated audio (issue #124).
//
// Transport choice: the existing federation is hub-mediated, and the direct
// yamux-TCP path (directResolver) only reaches peers that are network-reachable
// (same LAN via mDNS, or a routable/port-forwarded address). Peers behind NAT
// without inbound access cannot be dialed. WebRTC data channels add a
// NAT-traversing transport: ICE gathers server-reflexive (STUN) and relay (TURN)
// candidates so two NAT'd peers can establish a direct data channel with no
// inbound firewall openings.
//
// Required settings: at least one STUN server (EXIT66_FED_STUN, default
// stun:stun.l.google.com:19302). A TURN server (EXIT66_FED_TURN as
// turn://user:pass@host:port) is required for symmetric NAT or restrictive
// firewalls where a host/srflx candidate pair cannot connect. Without a
// reachable candidate pair, negotiation fails and the resolver falls back to the
// yamux-direct then hub-relay tiers — playback is never broken.
//
// Signaling rides the peer's own federation session: SDP offer/answer and
// trickle ICE candidates are POSTed to the recipient's /fed/signal/{to} endpoint
// over the session established by the token handshake, and land in that
// process's Signaler mailbox (see sendSignal). An outbound signal is addressed
// through the Registry, so it can only travel over an existing session and never
// to an arbitrary host.
//
// What that does NOT establish is who is on the far end. The handshake checks a
// single federation-wide token and takes the peer id from the claim the dialer
// makes, so a token holder chooses the id it is registered under. Signals are
// therefore authenticated as "some token holder", not as a particular peer, and
// SignalMsg.From is a claim rather than a credential — which is why routeToDial
// matches the sender against the negotiation's own peer and why newSID is
// random. Correlation across an offer/answer/ICE exchange is by SID (see
// SignalMsg).
//
// Two peers that have no session to each other cannot signal at all; hub-relayed
// signaling for that case is not built (issue #158).
//
// Dispatch model: each transport owns one mailbox (under selfID) and one reader
// goroutine (startedReader). The reader routes inbound offers to handleOffer and
// routes answers/ICE to the SID-keyed callback of the in-flight Dial that minted
// that SID. This means a peer that only Dials (never accepts inbound offers)
// still receives its own answers because the reader is always running.

const webrtcSetupTimeout = 15 * time.Second

// WebRTCTransport establishes and caches direct WebRTC data channels to peers.
// The same instance serves both as offerer (Dial) and answerer (the reader
// dispatches inbound offers via onChannel set by Listen).
type WebRTCTransport struct {
	selfID   string
	config   webrtc.Configuration
	signaler *Signaler
	// reg resolves a peer id to its live federation session, which is how an
	// outbound signal reaches a peer in another process (see postSignal).
	reg *Registry
	log *log.Logger

	api *webrtc.API

	mu    sync.Mutex
	cache map[string]*dataChannelConn // peerID -> live channel

	// dial maps a negotiation's SID to the slot that receives its answers and ICE
	// candidates. The single mailbox reader routes into these.
	dialMu sync.Mutex
	dial   map[string]*dialSlot // sid -> inbound messages for this negotiation

	// onChannel is set by Listen and invoked when an inbound offer establishes a
	// data channel (the answerer side).
	onChMu    sync.RWMutex
	onChannel func(*dataChannelConn)

	// setupTimeout is how long a negotiation may stay half-open: it bounds Dial
	// and arms handleOffer's watchdog. It is webrtcSetupTimeout in production and
	// exists as a field so tests can drive the watchdog without waiting it out.
	setupTimeout time.Duration

	readerOnce sync.Once
	readerDone chan struct{}
	mailbox    chan SignalMsg // set by startedReader; the channel the mailbox maps to
}

// NewWebRTCTransport builds a transport. iceServers are the STUN/TURN servers
// from settings (STUN always; TURN when configured). reg is the peer registry
// outbound signaling is addressed through; a nil reg confines signaling to
// signaler's in-process mailboxes. A nil logger defaults to the standard logger.
func NewWebRTCTransport(selfID string, iceServers []webrtc.ICEServer, signaler *Signaler, reg *Registry, logger *log.Logger) *WebRTCTransport {
	if logger == nil {
		logger = log.Default()
	}
	// Detaching data channels yields a raw io.ReadWriteCloser, which the framing
	// protocol reads and writes. Detach must be enabled on the SettingEngine
	// before any PeerConnection is created.
	se := webrtc.SettingEngine{}
	se.DetachDataChannels()
	return &WebRTCTransport{
		selfID:       selfID,
		config:       webrtc.Configuration{ICEServers: iceServers},
		signaler:     signaler,
		reg:          reg,
		log:          logger,
		setupTimeout: webrtcSetupTimeout,
		api:          webrtc.NewAPI(webrtc.WithSettingEngine(se)),
		cache:        make(map[string]*dataChannelConn),
		dial:         make(map[string]*dialSlot),
		readerDone:   make(chan struct{}),
	}
}

// startedReader lazily registers this transport's mailbox and runs the single
// dispatch loop. It is started on first Dial/Listen so a transport that is never
// used does not register a mailbox. The loop runs for the transport's lifetime
// (until Close).
func (t *WebRTCTransport) startedReader() {
	t.mailbox = t.signaler.Register(t.selfID)
	go func() {
		defer close(t.readerDone)
		for msg := range t.mailbox {
			switch msg.Type {
			case "offer":
				onCh := t.onChannelSnapshot()
				if onCh == nil {
					continue // not listening for inbound channels; ignore
				}
				// Claim the SID here, on the reader goroutine, before dispatching.
				// The offerer begins gathering at its own SetLocalDescription,
				// which is before it sends the offer, so its first ICE can be
				// close behind — and over the wire the two are independent
				// requests with no ordering guarantee. Registering inside
				// handleOffer would leave a window in which routeToDial silently
				// drops those candidates. Doing it here means any ICE the reader
				// sees after the offer has a slot waiting.
				iceIn, iceDone, ok := t.registerDial(msg.SID, msg.From)
				if !ok {
					// Another negotiation already holds this SID. Starting a
					// second one would orphan the first's slot; see registerDial.
					// Logged rather than dropped quietly: a repeated SID is either
					// a retransmit or a peer probing for this leak.
					t.log.Printf("webrtc answer: %s reused in-flight sid %s; ignoring", msg.From, msg.SID)
					continue
				}
				go t.handleOffer(msg, onCh, iceIn, iceDone)
			case "answer", "ice":
				t.routeToDial(msg)
			}
		}
	}()
}

// ensureReader starts the reader exactly once.
func (t *WebRTCTransport) ensureReader() {
	t.readerOnce.Do(t.startedReader)
}

// onChannelSnapshot returns the current answerer callback under a read lock.
func (t *WebRTCTransport) onChannelSnapshot() func(*dataChannelConn) {
	t.onChMu.RLock()
	defer t.onChMu.RUnlock()
	return t.onChannel
}

// Listen enables the answerer role: inbound offers spawn an answerer and opened
// data channels are delivered to onChannel. It is safe to call once; a second
// call replaces the callback. The reader goroutine owns the mailbox lifetime.
func (t *WebRTCTransport) Listen(_ context.Context, onChannel func(*dataChannelConn)) {
	t.onChMu.Lock()
	t.onChannel = onChannel
	t.onChMu.Unlock()
	t.ensureReader()
}

// newSID returns an unpredictable correlation id for an outbound offer.
//
// It must be unguessable, not merely unique. A SID is the routing key for
// inbound answers and ICE, and signaling now arrives from the wire: an id of the
// form selfID-1, selfID-2, ... lets any peer that can reach this instance's
// /fed/signal/ endpoint spray the whole small keyspace and land a message in an
// in-flight Dial. Randomness is the second of the two guards on that, the first
// being routeToDial's check that the sender is the peer the Dial is for.
func (t *WebRTCTransport) newSID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand cannot fail on any supported platform; if it somehow does,
		// refuse to mint a guessable id — Dial fails and the resolver falls back
		// a tier rather than negotiating on a spoofable correlation key.
		return ""
	}
	return hex.EncodeToString(b[:])
}

// dialSlot is one in-flight negotiation's inbound queue. peerID is the only peer
// permitted to feed it: a SignalMsg claiming a different sender is refused even
// when it carries the right SID.
type dialSlot struct {
	peerID string
	ch     chan SignalMsg
}

// registerDial claims sid for a negotiation with peerID and returns that
// negotiation's inbound channel plus a teardown func. ok is false when sid is
// already claimed, and the caller must then not start the negotiation at all.
//
// Teardown closes the channel as well as unmapping it, so whoever is ranging
// over it returns. It used to only unmap, which left that goroutine — and, on
// the answerer side, the PeerConnection it feeds — alive for the process's
// lifetime. Inbound SIDs are attacker-chosen strings, so anything retained per
// negotiation is retained per request.
//
// Refusing a duplicate rather than replacing the incumbent is the same concern
// one step further on. Storing unconditionally left the previous slot unmapped
// and unclosed — its teardown is identity-guarded and so became a no-op — which
// is the identical leak reached by a cheaper route: repeat one SID and every
// offer after the first orphans a pump and a PeerConnection, while the dial map
// still drains to zero and looks healthy. Replacing would fix the leak but hand
// an attacker a way to tear down a negotiation already in flight by naming its
// SID. Refusing cannot disturb an existing negotiation, and costs nothing
// legitimate: outbound SIDs are 128 random bits, so a collision is never an
// accident, and an offerer whose duplicate is refused gives up on the same
// deadline the incumbent is torn down on and retries under a fresh SID.
//
// Sends happen under dialMu (see routeToDial) so the close can never race one.
func (t *WebRTCTransport) registerDial(sid, peerID string) (ch <-chan SignalMsg, teardown func(), ok bool) {
	slot := &dialSlot{peerID: peerID, ch: make(chan SignalMsg, 16)}
	t.dialMu.Lock()
	if _, taken := t.dial[sid]; taken {
		t.dialMu.Unlock()
		return nil, nil, false
	}
	t.dial[sid] = slot
	t.dialMu.Unlock()
	return slot.ch, func() {
		t.dialMu.Lock()
		if cur, exists := t.dial[sid]; exists && cur == slot {
			delete(t.dial, sid)
			close(slot.ch)
		}
		t.dialMu.Unlock()
	}, true
}

// routeToDial forwards an answer/ICE message to the negotiation that minted its
// SID, if the message came from that negotiation's peer.
//
// The sender check is load-bearing. SignalMsg.From is not authenticated — any
// peer that can reach this instance's /fed/signal/ endpoint sets it freely — so
// without this a third peer that guesses or observes an in-flight SID can inject
// its own answer. The first SetRemoteDescription to land wins and the loser's
// error is discarded, and the resulting channel is cached under the peer the
// Dial was for, not the peer that answered: the caller would then fetch audio
// from the injector believing it to be the intended peer. Matching against the
// slot's own peerID refuses that.
//
// The send is done under dialMu because teardown closes the channel under the
// same lock; taking the value out and sending outside the lock would race the
// close. The send is non-blocking, so holding the lock across it is bounded.
func (t *WebRTCTransport) routeToDial(msg SignalMsg) {
	t.dialMu.Lock()
	defer t.dialMu.Unlock()
	slot, ok := t.dial[msg.SID]
	if !ok || msg.From != slot.peerID {
		return
	}
	select {
	case slot.ch <- msg:
	default:
	}
}

// sendSignal delivers msg to its recipient, whichever process it lives in.
//
// A recipient with a mailbox in this process is served from it directly: that is
// the shared-Signaler arrangement the in-process tests use. In production the
// only locally-registered id is this transport's own selfID, so a peer id always
// takes the wire path below.
//
// isRegistered gates the local attempt rather than Signaler.Send's return value:
// Send also returns false after blocking for signalTimeout on a registered but
// full mailbox, and this runs on pion's ICE-gathering goroutine, which must not
// stall for five seconds before trying the wire.
func (t *WebRTCTransport) sendSignal(msg SignalMsg) bool {
	if t.signaler.isRegistered(msg.To) {
		return t.signaler.Send(msg)
	}
	return t.postSignal(msg)
}

// postSignal POSTs msg to the recipient's /fed/signal/{to} endpoint over the
// recipient's own federation session, where WithSignalRelay drops it into that
// process's mailbox. It returns false when the peer has no live session — the
// caller treats that as a negotiation failure and falls back a tier.
//
// Routing through reg is what keeps this off arbitrary hosts: reg holds peers
// that completed the token handshake and have a live session, and p.Client is a
// SessionClient bound to that session, so baseURL only names the peer — it
// never selects a destination. It is not proof of who the peer is; the handshake
// does not verify the id a token holder claims.
func (t *WebRTCTransport) postSignal(msg SignalMsg) bool {
	if t.reg == nil {
		return false
	}
	p := t.reg.Get(msg.To)
	if p == nil || p.Client == nil {
		return false
	}
	baseURL := p.BaseURL
	if baseURL == "" {
		baseURL = "http://" + msg.To
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), signalTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/fed/signal/"+url.PathEscape(msg.To), bytes.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.Client.Do(req)
	if err != nil {
		t.log.Printf("webrtc signal %s %s: %v", msg.Type, msg.To, err)
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusAccepted {
		t.log.Printf("webrtc signal %s %s: status %d", msg.Type, msg.To, resp.StatusCode)
		return false
	}
	return true
}

// Dial is the offerer side: create a PeerConnection + data channel, exchange
// offer/answer and ICE with the peer via sendSignal, and wait for the channel
// to open. Returns a cached or fresh *dataChannelConn. On any failure returns an
// error so the resolver falls back to the next transport tier.
func (t *WebRTCTransport) Dial(ctx context.Context, peerID string) (*dataChannelConn, error) {
	if existing := t.get(peerID); existing != nil {
		return existing, nil
	}
	t.ensureReader()
	ctx, cancel := context.WithTimeout(ctx, t.setupTimeout)
	defer cancel()

	sid := t.newSID()
	if sid == "" {
		return nil, fmt.Errorf("webrtc dial: no randomness for correlation id")
	}
	in, teardown, ok := t.registerDial(sid, peerID)
	if !ok {
		// Unreachable in practice: sid is 128 random bits, freshly minted.
		return nil, fmt.Errorf("webrtc dial: correlation id %s already in flight", sid)
	}
	defer teardown()

	pc, err := t.api.NewPeerConnection(t.config)
	if err != nil {
		return nil, fmt.Errorf("webrtc dial: new pc: %w", err)
	}
	// success flips to true only on the open-channel path; otherwise the deferred
	// teardown closes the half-open PeerConnection and evicts any cached entry.
	success := false
	defer func() {
		if !success {
			_ = pc.Close()
			t.evict(peerID)
		}
	}()

	dc, err := pc.CreateDataChannel("audio", nil)
	if err != nil {
		return nil, fmt.Errorf("webrtc dial: create dc: %w", err)
	}

	openCh := make(chan *dataChannelConn, 1)
	failCh := make(chan error, 1)
	dc.OnOpen(func() {
		rwc, derr := dc.Detach()
		if derr != nil {
			select {
			case failCh <- fmt.Errorf("webrtc dial: detach: %w", derr):
			default:
			}
			return
		}
		conn := newDataChannelConn(peerID, dc, rwc)
		t.put(conn)
		select {
		case openCh <- conn:
		default:
		}
	})

	// Trickle local ICE candidates to the peer.
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return // gathering complete
		}
		b, _ := json.Marshal(c.ToJSON())
		t.sendSignal(SignalMsg{From: t.selfID, To: peerID, Type: "ice", SID: sid, ICECandidate: string(b)})
	})

	iceFailed := make(chan struct{}, 1)
	pc.OnICEConnectionStateChange(func(s webrtc.ICEConnectionState) {
		if s == webrtc.ICEConnectionStateFailed {
			select {
			case iceFailed <- struct{}{}:
			default:
			}
		}
	})

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return nil, fmt.Errorf("webrtc dial: create offer: %w", err)
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		return nil, fmt.Errorf("webrtc dial: set local: %w", err)
	}
	if !t.sendSignal(SignalMsg{From: t.selfID, To: peerID, Type: "offer", SID: sid, SDP: offer.SDP}) {
		return nil, fmt.Errorf("webrtc dial: peer %s offline", peerID)
	}

	// Pump inbound answers/ICE from the reader to the PeerConnection until the
	// channel opens or the negotiation fails. Exits when the Dial's mailbox
	// channel is torn down.
	go func() {
		for msg := range in {
			switch msg.Type {
			case "answer":
				_ = pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: msg.SDP})
			case "ice":
				var init webrtc.ICECandidateInit
				if json.Unmarshal([]byte(msg.ICECandidate), &init) == nil {
					_ = pc.AddICECandidate(init)
				}
			}
		}
	}()

	select {
	case conn := <-openCh:
		success = true
		return conn, nil
	case derr := <-failCh:
		return nil, derr
	case <-iceFailed:
		return nil, fmt.Errorf("webrtc dial: ICE failed")
	case <-ctx.Done():
		return nil, fmt.Errorf("webrtc dial: timeout")
	}
}

// handleOffer builds the answering PeerConnection for an inbound offer.
//
// The caller registers the inbound-ICE slot and hands over iceIn/iceDone; every
// exit path here must release them along with the PeerConnection and the ICE
// pump goroutine. This runs once per inbound offer, and an offer is one POST to
// /fed/signal/{to} carrying an attacker-chosen SID, so anything left behind
// accumulates per request rather than per peer.
func (t *WebRTCTransport) handleOffer(offer SignalMsg, onChannel func(*dataChannelConn), iceIn <-chan SignalMsg, iceDone func()) {
	pc, err := t.api.NewPeerConnection(t.config)
	if err != nil {
		t.log.Printf("webrtc answer: new pc: %v", err)
		iceDone()
		return
	}

	// torn makes teardown idempotent and reentrant-safe: pion may invoke a
	// close/state callback synchronously from inside pc.Close().
	var torn atomic.Bool
	teardown := func() {
		if torn.Swap(true) {
			return
		}
		iceDone()
		_ = pc.Close()
	}
	// A negotiation that never opens a channel — a peer that vanishes, or an
	// offer sent purely to allocate one — is torn down on the same deadline the
	// offerer gives up on.
	watchdog := time.AfterFunc(t.setupTimeout, teardown)

	go func() {
		for msg := range iceIn {
			if msg.Type == "ice" {
				var init webrtc.ICECandidateInit
				if json.Unmarshal([]byte(msg.ICECandidate), &init) == nil {
					_ = pc.AddICECandidate(init)
				}
			}
		}
	}()

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		dc.OnOpen(func() {
			rwc, derr := dc.Detach()
			if derr != nil {
				t.log.Printf("webrtc answer: detach: %v", derr)
				teardown()
				return
			}
			// Established: the setup deadline no longer applies, because the
			// connection legitimately owns its slot for as long as it lives (ICE
			// can trickle throughout). Release now follows the conn instead.
			//
			// It has to be the conn and not dc.OnClose: this channel is detached,
			// and pion surfaces a remote close on a detached channel as an EOF to
			// the reader, not as an OnClose. Relying on OnClose here left every
			// established answerer negotiation — its PeerConnection included —
			// held until ICE eventually failed, or forever if the offerer kept its
			// PeerConnection up. Whoever consumes the conn closes it when done
			// (see Manager.Start), and that is what releases this.
			watchdog.Stop()
			conn := newDataChannelConn(offer.From, dc, rwc)
			conn.onClose = teardown
			onChannel(conn)
		})
	})
	// A peer that vanishes rather than closing tidily: ICE reports it on the
	// first, and DTLS/SCTP failures that leave ICE untouched on the second.
	pc.OnICEConnectionStateChange(func(st webrtc.ICEConnectionState) {
		if st == webrtc.ICEConnectionStateFailed || st == webrtc.ICEConnectionStateClosed {
			teardown()
		}
	})
	pc.OnConnectionStateChange(func(st webrtc.PeerConnectionState) {
		if st == webrtc.PeerConnectionStateFailed || st == webrtc.PeerConnectionStateClosed {
			teardown()
		}
	})
	// Trickle our candidates back to the offerer under the same SID.
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		b, _ := json.Marshal(c.ToJSON())
		t.sendSignal(SignalMsg{From: t.selfID, To: offer.From, Type: "ice", SID: offer.SID, ICECandidate: string(b)})
	})

	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offer.SDP}); err != nil {
		t.log.Printf("webrtc answer: set remote: %v", err)
		teardown()
		return
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		t.log.Printf("webrtc answer: create: %v", err)
		teardown()
		return
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		t.log.Printf("webrtc answer: set local: %v", err)
		teardown()
		return
	}
	if !t.sendSignal(SignalMsg{From: t.selfID, To: offer.From, Type: "answer", SID: offer.SID, SDP: answer.SDP}) {
		t.log.Printf("webrtc answer: %s unreachable", offer.From)
		teardown()
	}
}

// Close unregisters the mailbox, which closes it and so ends the reader loop.
// It does not wait for the reader to return, and it does not close cached data
// channels (callers Close them).
func (t *WebRTCTransport) Close() {
	t.signaler.Unregister(t.selfID, t.mailbox)
}

// get returns a cached, open channel for peerID, or nil.
func (t *WebRTCTransport) get(peerID string) *dataChannelConn {
	t.mu.Lock()
	defer t.mu.Unlock()
	if c, ok := t.cache[peerID]; ok {
		if c.open() {
			return c
		}
		delete(t.cache, peerID)
	}
	return nil
}

func (t *WebRTCTransport) put(c *dataChannelConn) {
	t.mu.Lock()
	t.cache[c.peerID] = c
	t.mu.Unlock()
}

func (t *WebRTCTransport) evict(peerID string) {
	t.mu.Lock()
	delete(t.cache, peerID)
	t.mu.Unlock()
}

// dataChannelConn wraps a detached WebRTC data channel as an io.ReadWriteCloser
// and tracks the owning peer for caching/eviction.
type dataChannelConn struct {
	peerID string
	dc     *webrtc.DataChannel
	rwc    io.ReadWriteCloser
	// onClose, when set, releases whatever owns this channel's negotiation. The
	// answerer sets it because a detached channel gives it no other reliable
	// signal that the far end is done (see handleOffer).
	onClose func()
}

func newDataChannelConn(peerID string, dc *webrtc.DataChannel, rwc io.ReadWriteCloser) *dataChannelConn {
	return &dataChannelConn{peerID: peerID, dc: dc, rwc: rwc}
}

func (c *dataChannelConn) Read(p []byte) (int, error)  { return c.rwc.Read(p) }
func (c *dataChannelConn) Write(p []byte) (int, error) { return c.rwc.Write(p) }
func (c *dataChannelConn) Close() error {
	if c.onClose != nil {
		c.onClose()
	}
	_ = c.dc.Close()
	return c.rwc.Close()
}

func (c *dataChannelConn) open() bool {
	return c.dc.ReadyState() == webrtc.DataChannelStateOpen
}
