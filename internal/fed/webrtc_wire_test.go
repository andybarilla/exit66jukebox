package fed

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// signalProcess stands in for one running instance: its own Signaler (mailboxes
// are per-process), its own Registry, and the session handler main.go serves to
// a remote peer. The transports of two signalProcesses share no memory, so the
// only way a SignalMsg gets from one to the other is over HTTP — which is
// exactly the property #152 was about.
type signalProcess struct {
	peerID    string
	signaler  *Signaler
	reg       *Registry
	srv       *httptest.Server
	transport *WebRTCTransport
	// signalsIn counts requests that reached this process's /fed/signal/{to}
	// route, so a test can assert the negotiation really crossed the wire rather
	// than taking a shortcut through a shared Signaler.
	signalsIn atomic.Int64
}

// newSignalProcess builds one instance. The session handler is composed exactly
// as main.go composes PeerHandler, so the test fails if the signal route stops
// surviving that chain of muxes.
func newSignalProcess(t *testing.T, peerID string) *signalProcess {
	t.Helper()
	p := &signalProcess{peerID: peerID, signaler: NewSignaler(), reg: NewRegistry()}
	session := WithCapsRoute(
		Capabilities{DirectWebRTC: true},
		WithSignalRelay(p.signaler, PeerRoutes(nil, nil)),
	)
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/fed/signal/") {
			p.signalsIn.Add(1)
		}
		session.ServeHTTP(w, r)
	}))
	t.Cleanup(p.srv.Close)
	p.transport = NewWebRTCTransport(peerID, nil, p.signaler, p.reg, testLogger(t))
	t.Cleanup(p.transport.Close)
	return p
}

// knows registers other as a live, WebRTC-capable federation session, the way
// servePeerConn/dialPeer register a peer after the token handshake.
func (p *signalProcess) knows(other *signalProcess) {
	p.reg.put(&Peer{
		ID:      other.peerID,
		Client:  other.srv.Client(),
		BaseURL: other.srv.URL,
		Caps:    Capabilities{DirectWebRTC: true},
	})
}

// TestWebRTCSignalingCrossesProcessBoundary is the regression test for #152.
// Before the fix, WebRTCTransport signalled only through its own in-process
// Signaler, so two instances never negotiated: Dial failed at the first Send
// with "peer offline" in single-digit milliseconds, far short of the 15s
// webrtcSetupTimeout an ICE failure costs. Here the two transports hold
// separate Signalers bridged only by HTTP, and a data channel must still open.
func TestWebRTCSignalingCrossesProcessBoundary(t *testing.T) {
	alice := newSignalProcess(t, "alice")
	bob := newSignalProcess(t, "bob")
	alice.knows(bob)
	bob.knows(alice)

	if alice.signaler == bob.signaler {
		t.Fatal("the two processes share a Signaler; this would prove nothing")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	received := make(chan []byte, 1)
	bob.transport.Listen(ctx, func(c *dataChannelConn) {
		buf := make([]byte, 32)
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
		t.Fatalf("alice dial bob across processes: %v (after %v)", err, time.Since(start))
	}
	defer conn.Close()
	t.Logf("data channel opened across processes in %v", time.Since(start))

	payload := []byte("hello across processes")
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

	// Both halves of the wiring must have carried traffic: alice's offer/ICE into
	// bob's endpoint, and bob's answer/ICE back into alice's.
	if n := bob.signalsIn.Load(); n == 0 {
		t.Fatal("bob's /fed/signal/ route was never called: the offer did not cross the wire")
	}
	if n := alice.signalsIn.Load(); n == 0 {
		t.Fatal("alice's /fed/signal/ route was never called: the answer did not cross the wire")
	}
}

// TestWebRTCResolverStreamsAudioAcrossProcesses is #136's WebRTC acceptance
// criterion under the condition that actually holds in production: the two
// peers are separate processes. It mirrors TestWebRTCResolverStreamsAudioWithRange,
// which shares one Signaler between the transports and therefore passed
// throughout the outage #152 describes.
func TestWebRTCResolverStreamsAudioAcrossProcesses(t *testing.T) {
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
	alice.knows(bob)
	bob.knows(alice)

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

	resolver := &directResolver{reg: alice.reg, webrtc: alice.transport}

	// bob's session handler mounts no application (PeerRoutes(nil, nil)), so
	// tier 2 — a plain GET for the track over that same session — answers 404.
	// That is what makes the 206 below attributable: only the WebRTC tier, whose
	// answerer serves against the backend handler, can produce it.
	tier2 := httptest.NewRecorder()
	tier2Req := httptest.NewRequest(http.MethodGet, bob.srv.URL+"/api/tracks/42/audio", nil)
	bob.srv.Config.Handler.ServeHTTP(tier2, tier2Req)
	if tier2.Code != http.StatusNotFound {
		t.Fatalf("tier 2 over bob's session = %d, want 404; a 206 below would be ambiguous", tier2.Code)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tracks/9/audio", nil)
	req.Header.Set("Range", "bytes=2-5")
	resolver.ServeRemoteAudio(rec, req, "bob", 42)

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
	if n := bob.signalsIn.Load(); n == 0 {
		t.Fatal("bob's /fed/signal/ route was never called: the stream did not negotiate over the wire")
	}
}

// TestWebRTCDialFailsWhenPeerHasNoSession pins the other half of postSignal: a
// peer that is not in the registry has no session to signal over, so Dial fails
// and the resolver falls back rather than hanging until webrtcSetupTimeout.
// This is the hub-only-NAT case that #158 covers.
func TestWebRTCDialFailsWhenPeerHasNoSession(t *testing.T) {
	alice := newSignalProcess(t, "alice")
	bob := newSignalProcess(t, "bob")
	bob.transport.Listen(context.Background(), func(*dataChannelConn) {})
	// alice deliberately does not know bob.

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := alice.transport.Dial(ctx, "bob"); err == nil {
		t.Fatal("dial succeeded with no session to bob")
	} else if !strings.Contains(err.Error(), "offline") {
		t.Fatalf("dial failed for an unexpected reason: %v", err)
	}
	if n := bob.signalsIn.Load(); n != 0 {
		t.Fatalf("bob's signal route was called %d times with no session registered", n)
	}
}
