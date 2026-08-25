package fed

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/yamux"
)

// captureStdLog redirects the standard logger — which is where the resolver
// writes its transport line — for the duration of a test, and returns what was
// written. Writes are serialised because pion and the answerer log from their
// own goroutines while the capture is in place.
func captureStdLog(t *testing.T) func() string {
	t.Helper()
	buf := &syncBuffer{}
	out, flags := log.Writer(), log.Flags()
	log.SetOutput(buf)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(out); log.SetFlags(flags) })
	return buf.String
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// hubProcess stands in for a hub instance in the topology #158 is about: two
// peers with no session to each other, each holding only a session to the hub.
//
// It is faithful in the two ways that matter. Its Signaler never holds a
// mailbox, because a hub builds no WebRTC transport (main.go constructs one for
// role "peer" only), which is why the mount there answered 503 unconditionally
// before this change. And it serves one HubSessionHandler over every member
// session, tagged per session with that member's peer id — exactly what
// serveHubConn does — so the identity the relay stamps into From comes from the
// session rather than from anything the sender wrote.
type hubProcess struct {
	signaler *Signaler
	reg      *Registry
	handler  http.Handler

	mu sync.Mutex
	// signals counts /fed/signal/ requests arriving on each member's own
	// session, so a test can show which sessions actually carried the
	// negotiation rather than assuming it went through the hub.
	signals map[string]int
}

func newHubProcess(t *testing.T) *hubProcess {
	t.Helper()
	h := &hubProcess{signaler: NewSignaler(), reg: NewRegistry(), signals: make(map[string]int)}
	h.handler = HubSessionHandler(Capabilities{}, h.signaler, NewRelay(h.reg, nil))
	return h
}

// admit wires p in as a member of h in both directions, the way the token
// handshake does in production: the hub registers the member (serveHubConn ->
// acceptAndRegister) and serves HubHandler over that one session tagged with
// the member's id (serveHubConn -> WithSessionPeer), while the member registers
// the hub under "@hub" (runMember).
func (h *hubProcess) admit(t *testing.T, p *signalProcess) {
	t.Helper()
	h.reg.put(&Peer{ID: p.peerID, Client: p.srv.Client(), BaseURL: p.srv.URL, Caps: Capabilities{DirectWebRTC: true}})
	session := WithSessionPeer(p.peerID, h.handler)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/fed/signal/") {
			h.mu.Lock()
			h.signals[p.peerID]++
			h.mu.Unlock()
		}
		session.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	p.reg.put(&Peer{ID: hubPeerID, Client: srv.Client(), BaseURL: srv.URL})
}

func (h *hubProcess) signalsOn(peerID string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.signals[peerID]
}

// postThroughHub sends msg over sender's own hub session, the way
// WebRTCTransport.postSignal does once it falls back to the hub. It returns the
// status the hub answered with.
func postThroughHub(t *testing.T, sender *signalProcess, msg SignalMsg) int {
	t.Helper()
	hub := sender.reg.Get(hubPeerID)
	if hub == nil {
		t.Fatal("sender has no hub session")
	}
	body, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, hub.BaseURL+"/fed/signal/"+msg.To, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := hub.Client.Do(req)
	if err != nil {
		t.Fatalf("post through hub: %v", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

// noDirectSession asserts the condition the whole file is about. Without it a
// passing test could be explained by the direct peer-to-peer path #163 built.
func noDirectSession(t *testing.T, a, b *signalProcess) {
	t.Helper()
	if a.reg.Get(b.peerID) != nil || b.reg.Get(a.peerID) != nil {
		t.Fatal("the peers know each other directly; this would not exercise the hub relay")
	}
}

// TestWebRTCNegotiatesThroughHubWithNoDirectSession is #158's acceptance
// criterion: three separate Signalers — two peers and a hub — bridged only by
// HTTP, where the peers have no session to each other, negotiate a data channel.
//
// Before this change the tier bailed at the first signal: postSignal found no
// registry entry for the recipient and returned false, and even had it reached
// the hub, the hub hosts no mailbox for anyone so its relay answered 503.
func TestWebRTCNegotiatesThroughHubWithNoDirectSession(t *testing.T) {
	alice := newSignalProcess(t, "alice")
	bob := newSignalProcess(t, "bob")
	hub := newHubProcess(t)
	hub.admit(t, alice)
	hub.admit(t, bob)
	noDirectSession(t, alice, bob)

	if alice.signaler == bob.signaler || alice.signaler == hub.signaler {
		t.Fatal("the processes share a Signaler; this would prove nothing")
	}
	if hub.signaler.isRegistered("alice") || hub.signaler.isRegistered("bob") {
		t.Fatal("the hub hosts a signaling mailbox; a real hub builds no WebRTC transport")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	received := make(chan []byte, 1)
	bob.transport.Listen(ctx, func(c *dataChannelConn) {
		if c.peerID != "alice" {
			t.Errorf("bob's channel is talking to %q, want alice", c.peerID)
		}
		buf := make([]byte, 64)
		n, err := c.Read(buf)
		if err != nil {
			t.Errorf("bob read: %v", err)
			return
		}
		select {
		case received <- buf[:n]:
		default:
		}
	})

	start := time.Now()
	conn, err := alice.transport.Dial(ctx, "bob")
	if err != nil {
		t.Fatalf("alice dial bob through the hub: %v (after %v)", err, time.Since(start))
	}
	defer conn.Close()
	t.Logf("data channel opened through the hub in %v", time.Since(start))

	payload := []byte("hello through the hub")
	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("alice write: %v", err)
	}
	select {
	case got := <-received:
		if string(got) != string(payload) {
			t.Fatalf("bob received %q want %q", got, payload)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("bob did not receive the message")
	}

	// Every hop the negotiation had to take. alice's and bob's hub sessions
	// carried the two directions into the hub; each peer's own signal route
	// received what the hub forwarded on.
	for _, c := range []struct {
		what string
		n    int
	}{
		{"alice's hub session", hub.signalsOn("alice")},
		{"bob's hub session", hub.signalsOn("bob")},
		{"bob's own signal route", int(bob.signalsIn.Load())},
		{"alice's own signal route", int(alice.signalsIn.Load())},
	} {
		if c.n == 0 {
			t.Fatalf("%s carried no signals: the negotiation did not go through the hub", c.what)
		}
	}
}

// TestHubRelayedWebRTCStreamsAudio is the rest of #158's acceptance criterion:
// the same three-process topology streams audio with transport=webrtc.
//
// alice's resolver is given no hub relay tier, so tiers 2 and 3 cannot answer at
// all — alice has no session to bob, and ServeRemoteAudio's p == nil branch with
// a nil hub writes 503. TestHubRelayedAudioNeedsTheWebRTCTier is the positive
// control for that: the same request on the same topology with the WebRTC
// transport removed does answer 503. So the 206 below is attributable.
func TestHubRelayedWebRTCStreamsAudio(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tracks/42/audio" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if rng := r.Header.Get("Range"); rng != "bytes=2-5" {
			t.Errorf("range not forwarded: %q", rng)
		}
		w.Header().Set("Content-Range", "bytes 2-5/8")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Content-Length", "4")
		w.WriteHeader(http.StatusPartialContent)
		io.WriteString(w, "CDEF")
	}))
	defer backend.Close()

	alice := newSignalProcess(t, "alice")
	bob := newSignalProcess(t, "bob")
	hub := newHubProcess(t)
	hub.admit(t, alice)
	hub.admit(t, bob)
	noDirectSession(t, alice, bob)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bob.transport.Listen(ctx, func(c *dataChannelConn) {
		go func() {
			if err := ServeAudioOverConn(c, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, backend.URL+r.URL.Path, nil)
				if rng := r.Header.Get("Range"); rng != "" {
					req.Header.Set("Range", rng)
				}
				resp, err := http.DefaultTransport.RoundTrip(req)
				if err != nil {
					http.Error(w, "backend", http.StatusBadGateway)
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
			}), ""); err != nil {
				t.Errorf("bob serve: %v", err)
			}
		}()
	})

	logged := captureStdLog(t)
	resolver := &directResolver{reg: alice.reg, webrtc: alice.transport}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tracks/9/audio", nil)
	req.Header.Set("Range", "bytes=2-5")
	resolver.ServeRemoteAudio(rec, req, "bob", 42)

	// The line #158 names by hand, and the one an operator reads in the field —
	// #152 was found by transport=webrtc never appearing in a real log.
	if got := logged(); !strings.Contains(got, "fed audio bob/42 transport=webrtc") {
		t.Fatalf("no transport=webrtc line; the resolver logged: %q", got)
	}

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status = %d want %d (body %q)", rec.Code, http.StatusPartialContent, rec.Body.String())
	}
	if rec.Body.String() != "CDEF" {
		t.Fatalf("body = %q want CDEF", rec.Body.String())
	}
	for _, c := range []struct{ hdr, want string }{
		{"Content-Range", "bytes 2-5/8"},
		{"Accept-Ranges", "bytes"},
		{"Content-Type", "audio/mpeg"},
		{"Content-Length", "4"},
	} {
		if got := rec.Header().Get(c.hdr); got != c.want {
			t.Fatalf("%s = %q want %q", c.hdr, got, c.want)
		}
	}
	if hub.signalsOn("alice") == 0 || hub.signalsOn("bob") == 0 {
		t.Fatal("the stream did not negotiate through the hub")
	}
}

// TestHubRelayedAudioNeedsTheWebRTCTier is the positive control for the test
// above: with the WebRTC transport removed and everything else identical, the
// request answers 503 rather than 206. Without this, a 206 from a future tier 2
// or 3 would satisfy the assertions there and the acceptance criterion would
// pass without WebRTC ever engaging.
func TestHubRelayedAudioNeedsTheWebRTCTier(t *testing.T) {
	alice := newSignalProcess(t, "alice")
	bob := newSignalProcess(t, "bob")
	hub := newHubProcess(t)
	hub.admit(t, alice)
	hub.admit(t, bob)

	resolver := &directResolver{reg: alice.reg}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tracks/9/audio", nil)
	req.Header.Set("Range", "bytes=2-5")
	resolver.ServeRemoteAudio(rec, req, "bob", 42)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d want 503; the 206 in the acceptance test is not attributable to WebRTC", rec.Code)
	}
}

// TestHubStampsForwardedSenderFromTheSession is the authorization property the
// forwarding surface rests on. When a hub forwards, it is asserting who the
// message came from, so From must come from the session the request arrived on
// rather than from the body — SignalMsg.From is a claim, not a credential
// (#163's R1). Getting this wrong reintroduces the negotiation hijack one layer
// up: mallory would land an answer in alice's in-flight dial with bob.
//
// The negative and the positive are both asserted against the same live dial
// slot, so a slot that simply never receives anything cannot pass.
func TestHubStampsForwardedSenderFromTheSession(t *testing.T) {
	alice := newSignalProcess(t, "alice")
	bob := newSignalProcess(t, "bob")
	mallory := newSignalProcess(t, "mallory")
	hub := newHubProcess(t)
	hub.admit(t, alice)
	hub.admit(t, bob)
	hub.admit(t, mallory)

	alice.transport.ensureReader()
	const sid = "a-negotiation-alice-has-in-flight-with-bob"
	in, teardown, ok := alice.transport.registerDial(sid, "bob")
	if !ok {
		t.Fatal("could not claim the sid")
	}
	defer teardown()

	// mallory claims to be bob, over her own hub session, on bob's SID.
	if code := postThroughHub(t, mallory, SignalMsg{
		From: "bob", To: "alice", Type: "answer", SID: sid, SDP: "mallory's answer",
	}); code != http.StatusAccepted {
		t.Fatalf("hub refused mallory's post with %d; the test would pass without the stamp doing anything", code)
	}
	select {
	case msg := <-in:
		t.Fatalf("alice's negotiation with bob accepted %q from mallory (From=%q): the hub relayed her claim", msg.SDP, msg.From)
	case <-time.After(500 * time.Millisecond):
	}

	// The same message from bob's own session is accepted, so the refusal above
	// is the sender check and not a dead path.
	if code := postThroughHub(t, bob, SignalMsg{
		From: "bob", To: "alice", Type: "answer", SID: sid, SDP: "bob's answer",
	}); code != http.StatusAccepted {
		t.Fatalf("hub refused bob's post with %d", code)
	}
	select {
	case msg := <-in:
		if msg.From != "bob" || msg.SDP != "bob's answer" {
			t.Fatalf("alice received %+v, want bob's answer", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("alice's negotiation never received bob's answer")
	}
}

// TestHubRefusesToForwardWithoutSessionIdentity pins the other half of the
// stamp. The hub can only assert a sender it knows, so a relay request that
// carries no session identity is refused rather than forwarded with whatever
// From the body claimed.
func TestHubRefusesToForwardWithoutSessionIdentity(t *testing.T) {
	bob := newSignalProcess(t, "bob")
	hub := newHubProcess(t)
	hub.admit(t, bob)
	bob.transport.ensureReader()

	body, _ := json.Marshal(SignalMsg{From: "alice", To: "bob", Type: "offer", SID: "s", SDP: "x"})
	rec := httptest.NewRecorder()
	hub.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/fed/signal/bob", bytes.NewReader(body)))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a relay request with no session identity", rec.Code)
	}
	if n := bob.signalsIn.Load(); n != 0 {
		t.Fatalf("the hub forwarded %d unattributable signals to bob", n)
	}
}

// TestPeerSessionDoesNotForwardSignals pins the premise the absence of a
// forwarding loop rests on: only the hub composition forwards, so a message the
// hub relays onward cannot be relayed on again by whoever receives it.
//
// "other" genuinely hosts a mailbox and is genuinely reachable over HTTP, so a
// composition that did forward would succeed in reaching it — which is what
// makes the zero count below mean something rather than merely restating that
// these handlers were built without a registry to forward through.
func TestPeerSessionDoesNotForwardSignals(t *testing.T) {
	other := newSignalProcess(t, "other")
	other.transport.ensureReader()
	if !other.signaler.isRegistered("other") {
		t.Fatal("other hosts no mailbox; a forwarded signal would be refused at the far end anyway")
	}

	body, _ := json.Marshal(SignalMsg{From: "alice", To: "other", Type: "offer", SID: "s", SDP: "x"})
	for name, h := range map[string]http.Handler{
		"peer":   PeerSessionHandler(Capabilities{}, NewSignaler(), nil, nil),
		"member": MemberSessionHandler(Capabilities{}, nil),
	} {
		rec := httptest.NewRecorder()
		WithSessionPeer("alice", h).ServeHTTP(rec,
			httptest.NewRequest(http.MethodPost, "/fed/signal/other", bytes.NewReader(body)))
		if rec.Code == http.StatusAccepted {
			t.Errorf("%s session accepted a signal for a peer it does not host", name)
		}
	}
	if n := other.signalsIn.Load(); n != 0 {
		t.Fatalf("a non-hub session forwarded %d signals onward; hub relaying is no longer bounded to one hop", n)
	}
}

// TestHubSessionAttributesSignalsToTheHandshakePeer pins the production wiring
// rather than the harness above. The stamp is only as good as the id
// Manager.serveHubConn tags the session with, and serveHubConn is what the tests
// in this file model by hand — so without this, dropping WithSessionPeer there
// would leave every test green while the hub forwarded on the body's word.
//
// It runs a real handshake over a real yamux session: mallory connects claiming
// "mallory", then posts a signal for bob claiming to be alice.
func TestHubSessionAttributesSignalsToTheHandshakePeer(t *testing.T) {
	bob := newSignalProcess(t, "bob")
	bob.transport.ensureReader()
	seen := make(chan SignalMsg, 1)
	bob.onSignal = func(m SignalMsg) {
		select {
		case seen <- m:
		default:
		}
	}

	reg := NewRegistry()
	reg.put(&Peer{ID: "bob", Client: bob.srv.Client(), BaseURL: bob.srv.URL})
	m := &Manager{
		Role:       "hub",
		Token:      "tok",
		Registry:   reg,
		HubHandler: HubSessionHandler(Capabilities{}, NewSignaler(), NewRelay(reg, nil)),
	}

	cConn, sConn := net.Pipe()
	go m.serveHubConn(sConn)
	if err := dialHandshake(cConn, "tok", "mallory"); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	sess, err := yamux.Client(cConn, nil)
	if err != nil {
		t.Fatalf("yamux client: %v", err)
	}
	defer sess.Close()

	body, _ := json.Marshal(SignalMsg{From: "alice", To: "bob", Type: "offer", SID: "s", SDP: "x"})
	req, err := http.NewRequest(http.MethodPost, "http://hub/fed/signal/bob", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := SessionClient(sess).Do(req)
	if err != nil {
		t.Fatalf("post over the hub session: %v", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("hub answered %d, want 202", resp.StatusCode)
	}

	select {
	case msg := <-seen:
		if msg.From != "mallory" {
			t.Fatalf("bob was told the offer came from %q; the hub relayed the body's claim rather than the session", msg.From)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the offer never reached bob")
	}
}
