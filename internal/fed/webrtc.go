package fed

import (
	"bytes"
	"context"
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
// Signaling rides the peer's own authenticated federation session: SDP
// offer/answer and trickle ICE candidates are POSTed to the recipient's
// /fed/signal/{to} endpoint over the session established by the token
// handshake, and land in that process's Signaler mailbox (see sendSignal). Only
// peers that are in the registry — i.e. token-authenticated and on the accepted
// federation_peer list — can be signalled, preserving the federation's
// SSRF-safety property. Correlation across an offer/answer/ICE exchange is by
// SID (see SignalMsg).
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

	// sidSeq mints per-negotiation correlation ids for outbound offers.
	sidSeq uint64

	// dial maps an in-flight Dial's SID to the channel that receives answers and
	// ICE candidates for it. The single mailbox reader routes into these.
	dialMu sync.Mutex
	dial   map[string]chan SignalMsg // sid -> inbound messages for this Dial

	// onChannel is set by Listen and invoked when an inbound offer establishes a
	// data channel (the answerer side).
	onChMu    sync.RWMutex
	onChannel func(*dataChannelConn)

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
		selfID:     selfID,
		config:     webrtc.Configuration{ICEServers: iceServers},
		signaler:   signaler,
		reg:        reg,
		log:        logger,
		api:        webrtc.NewAPI(webrtc.WithSettingEngine(se)),
		cache:      make(map[string]*dataChannelConn),
		dial:       make(map[string]chan SignalMsg),
		readerDone: make(chan struct{}),
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
				go t.handleOffer(msg, onCh)
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

// newSID returns a unique correlation id for an outbound offer.
func (t *WebRTCTransport) newSID() string {
	return fmt.Sprintf("%s-%d", t.selfID, atomic.AddUint64(&t.sidSeq, 1))
}

// registerDial creates an inbound channel for messages matching sid and returns
// it plus a teardown func.
func (t *WebRTCTransport) registerDial(sid string) (<-chan SignalMsg, func()) {
	ch := make(chan SignalMsg, 16)
	t.dialMu.Lock()
	t.dial[sid] = ch
	t.dialMu.Unlock()
	return ch, func() {
		t.dialMu.Lock()
		if cur, ok := t.dial[sid]; ok && cur == ch {
			delete(t.dial, sid)
		}
		t.dialMu.Unlock()
	}
}

// routeToDial forwards an answer/ICE message to the waiting Dial (if any).
func (t *WebRTCTransport) routeToDial(msg SignalMsg) {
	t.dialMu.Lock()
	ch, ok := t.dial[msg.SID]
	t.dialMu.Unlock()
	if !ok {
		return
	}
	select {
	case ch <- msg:
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
// The session is the authorization: reg only holds peers that completed the
// token handshake and are on the accepted federation_peer list, so this cannot
// address an arbitrary host.
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
// offer/answer and ICE with the peer via the signaler, and wait for the channel
// to open. Returns a cached or fresh *dataChannelConn. On any failure returns an
// error so the resolver falls back to the next transport tier.
func (t *WebRTCTransport) Dial(ctx context.Context, peerID string) (*dataChannelConn, error) {
	if existing := t.get(peerID); existing != nil {
		return existing, nil
	}
	t.ensureReader()
	ctx, cancel := context.WithTimeout(ctx, webrtcSetupTimeout)
	defer cancel()

	sid := t.newSID()
	in, teardown := t.registerDial(sid)
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
func (t *WebRTCTransport) handleOffer(offer SignalMsg, onChannel func(*dataChannelConn)) {
	pc, err := t.api.NewPeerConnection(t.config)
	if err != nil {
		t.log.Printf("webrtc answer: new pc: %v", err)
		return
	}
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		dc.OnOpen(func() {
			rwc, derr := dc.Detach()
			if derr != nil {
				t.log.Printf("webrtc answer: detach: %v", derr)
				return
			}
			onChannel(newDataChannelConn(offer.From, dc, rwc))
		})
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
		_ = pc.Close()
		return
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		t.log.Printf("webrtc answer: create: %v", err)
		_ = pc.Close()
		return
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		t.log.Printf("webrtc answer: set local: %v", err)
		_ = pc.Close()
		return
	}
	// Receive the offerer's trickled ICE under the same SID: register a dial slot
	// keyed by offer.SID so the reader routes them here.
	iceIn, iceDone := t.registerDial(offer.SID)
	go func() {
		defer iceDone()
		for msg := range iceIn {
			if msg.Type == "ice" {
				var init webrtc.ICECandidateInit
				if json.Unmarshal([]byte(msg.ICECandidate), &init) == nil {
					_ = pc.AddICECandidate(init)
				}
			}
		}
	}()
	t.sendSignal(SignalMsg{From: t.selfID, To: offer.From, Type: "answer", SID: offer.SID, SDP: answer.SDP})
}

// Close unregisters the mailbox and waits for the reader to exit, releasing the
// mailbox name. It does not close cached data channels (callers Close them).
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
}

func newDataChannelConn(peerID string, dc *webrtc.DataChannel, rwc io.ReadWriteCloser) *dataChannelConn {
	return &dataChannelConn{peerID: peerID, dc: dc, rwc: rwc}
}

func (c *dataChannelConn) Read(p []byte) (int, error)  { return c.rwc.Read(p) }
func (c *dataChannelConn) Write(p []byte) (int, error) { return c.rwc.Write(p) }
func (c *dataChannelConn) Close() error {
	_ = c.dc.Close()
	return c.rwc.Close()
}

func (c *dataChannelConn) open() bool {
	return c.dc.ReadyState() == webrtc.DataChannelStateOpen
}
