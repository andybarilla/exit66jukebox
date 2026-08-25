package fed

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestWebRTCDialerAndAnswererExchangeBytes wires two WebRTCTransports against a
// shared in-memory Signaler (no real network, no STUN — pion connects the two
// in-process PeerConnections directly) and verifies a data channel carries
// bytes from the offerer to the answerer. This de-risks the resolver integration
// before framing is layered on.
func TestWebRTCDialerAndAnswererExchangeBytes(t *testing.T) {
	signaler := NewSignaler()

	alice := NewWebRTCTransport("alice", nil, signaler, nil, testLogger(t))
	bob := NewWebRTCTransport("bob", nil, signaler, nil, testLogger(t))
	defer alice.Close()
	defer bob.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Bob is the answerer: when a channel opens, read one message and report it.
	bobReceived := make(chan []byte, 1)
	bob.Listen(ctx, func(c *dataChannelConn) {
		buf := make([]byte, 32)
		n, err := c.Read(buf)
		if err != nil {
			t.Errorf("bob read: %v", err)
			return
		}
		select {
		case bobReceived <- buf[:n]:
		default:
		}
	})

	// Alice dials Bob; the data channel opens on both sides.
	conn, err := alice.Dial(ctx, "bob")
	if err != nil {
		t.Fatalf("alice dial: %v", err)
	}
	defer conn.Close()

	payload := []byte("hello webrtc")
	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("alice write: %v", err)
	}

	select {
	case got := <-bobReceived:
		if string(got) != string(payload) {
			t.Fatalf("bob received %q want %q", got, payload)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("bob did not receive the message")
	}
}

// TestWebRTCDialTimesOutWhenPeerAbsent verifies the transport returns an error
// (so the resolver can fall back) when the target peer is not listening.
func TestWebRTCDialTimesOutWhenPeerAbsent(t *testing.T) {
	signaler := NewSignaler()
	alice := NewWebRTCTransport("alice", nil, signaler, nil, testLogger(t))
	defer alice.Close()

	// Register no mailbox for "bob" → offer Send fails immediately.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := alice.Dial(ctx, "bob"); err == nil {
		t.Fatal("expected dial to fail when peer absent")
	}
}

// TestWebRTCBothPeersCanDial verifies symmetric reachability: each side can act
// as offerer to the other, with both readers running and both registering
// callbacks. Alice dials Bob, then (independently) Bob dials Alice.
func TestWebRTCBothPeersCanDial(t *testing.T) {
	signaler := NewSignaler()
	alice := NewWebRTCTransport("alice", nil, signaler, nil, testLogger(t))
	bob := NewWebRTCTransport("bob", nil, signaler, nil, testLogger(t))
	defer alice.Close()
	defer bob.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	aliceReceived := make(chan []byte, 1)
	bobReceived := make(chan []byte, 1)

	alice.Listen(ctx, func(c *dataChannelConn) {
		buf := make([]byte, 16)
		if n, err := c.Read(buf); err == nil {
			select {
			case aliceReceived <- buf[:n]:
			default:
			}
		}
	})
	bob.Listen(ctx, func(c *dataChannelConn) {
		buf := make([]byte, 16)
		if n, err := c.Read(buf); err == nil {
			select {
			case bobReceived <- buf[:n]:
			default:
			}
		}
	})

	// Alice → Bob.
	a2b, err := alice.Dial(ctx, "bob")
	if err != nil {
		t.Fatalf("alice dial bob: %v", err)
	}
	defer a2b.Close()
	if _, err := a2b.Write([]byte("a2b")); err != nil {
		t.Fatalf("a2b write: %v", err)
	}
	select {
	case got := <-bobReceived:
		if string(got) != "a2b" {
			t.Fatalf("bob got %q want a2b", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("bob did not receive a2b")
	}

	// Bob → Alice.
	b2a, err := bob.Dial(ctx, "alice")
	if err != nil {
		t.Fatalf("bob dial alice: %v", err)
	}
	defer b2a.Close()
	if _, err := b2a.Write([]byte("b2a")); err != nil {
		t.Fatalf("b2a write: %v", err)
	}
	select {
	case got := <-aliceReceived:
		if string(got) != "b2a" {
			t.Fatalf("alice got %q want b2a", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("alice did not receive b2a")
	}
}

// TestWebRTCResolverStreamsAudioWithRange exercises the full cascade: a
// directResolver configured with a WebRTC transport streams a remote track
// directly over a WebRTC data channel, preserving Range semantics and the four
// audio headers. This is the core acceptance test for issue #124.
func TestWebRTCResolverStreamsAudioWithRange(t *testing.T) {
	// Bob's local audio backend: serves a track with Range support, like the real
	// trackAudio → http.ServeFile path.
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

	signaler := NewSignaler()
	alice := NewWebRTCTransport("alice", nil, signaler, nil, testLogger(t))
	bob := NewWebRTCTransport("bob", nil, signaler, nil, testLogger(t))
	defer alice.Close()
	defer bob.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Bob serves inbound WebRTC data channels against the backend handler.
	bob.Listen(ctx, func(c *dataChannelConn) {
		go func() {
			if err := ServeAudioOverConn(c, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Re-dispatch to the real backend so Range/headers are real.
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

	// Alice's resolver: the peer is registered with DirectWebRTC capability so
	// the WebRTC tier is attempted. No yamux Client/Hub is needed because the
	// WebRTC tier should succeed.
	reg := NewRegistry()
	reg.put(&Peer{ID: "bob", Caps: Capabilities{DirectWebRTC: true}})
	resolver := &directResolver{reg: reg, webrtc: alice}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tracks/9/audio", nil)
	req.Header.Set("Range", "bytes=2-5")
	resolver.ServeRemoteAudio(rec, req, "bob", 42)

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status = %d want %d", rec.Code, http.StatusPartialContent)
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
}

// TestWebRTCFallsBackToYamuxDirectWhenDialFails verifies that when the WebRTC
// tier cannot connect, the resolver falls through to the existing yamux-direct
// path (tier 2). The peer is registered with a working Client but WebRTC dial
// fails because no answerer is listening.
func TestWebRTCFallsBackToYamuxDirectWhenDialFails(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, "DIRECT")
	}))
	defer backend.Close()

	signaler := NewSignaler()
	// Alice has a transport, but no bob is listening on the signaler, so WebRTC
	// dial fails and the resolver must fall back to the yamux-direct Client.
	alice := NewWebRTCTransport("alice", nil, signaler, nil, testLogger(t))
	defer alice.Close()

	reg := NewRegistry()
	reg.put(&Peer{ID: "bob", Client: backend.Client(), BaseURL: backend.URL, Caps: Capabilities{DirectWebRTC: true}})
	resolver := &directResolver{reg: reg, webrtc: alice}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tracks/9/audio", nil)
	resolver.ServeRemoteAudio(rec, req, "bob", 42)

	if rec.Code != http.StatusOK || rec.Body.String() != "DIRECT" {
		t.Fatalf("fallback = code %d body %q", rec.Code, rec.Body.String())
	}
}

// TestWebRTCFallsBackToHubWhenPeerOffline verifies the WebRTC-disabled /
// peer-offline case still routes to the hub relay (tier 3). This mirrors the
// existing TestPeerResolverFallsBackToHubWhenDirectPeerOffline but with a
// WebRTC-capable resolver configured.
func TestWebRTCFallsBackToHubWhenPeerOffline(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fed/audio/bob/42" {
			t.Errorf("unexpected hub path: %s", r.URL.Path)
		}
		io.WriteString(w, "RELAY")
	}))
	defer hub.Close()

	signaler := NewSignaler()
	alice := NewWebRTCTransport("alice", nil, signaler, nil, testLogger(t))
	defer alice.Close()

	reg := NewRegistry()
	reg.put(&Peer{ID: "@hub", Client: hub.Client(), BaseURL: hub.URL})
	// bob is not registered at all → both WebRTC and direct fail → hub relay.
	mgr := &Manager{Role: "peer", Registry: reg, WebRTC: alice}
	resolver := &directResolver{reg: reg, hub: &memberResolver{m: mgr}, webrtc: alice}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tracks/9/audio", nil)
	resolver.ServeRemoteAudio(rec, req, "bob", 42)

	if rec.Code != http.StatusOK || rec.Body.String() != "RELAY" {
		t.Fatalf("hub fallback = code %d body %q", rec.Code, rec.Body.String())
	}
}

// TestWebRTCDisabledSkipsWebRTCTier verifies that when direct P2P is disabled
// (webrtc transport is nil), the resolver skips the WebRTC tier and uses the
// yamux-direct path directly — hub relay behavior is unchanged.
func TestWebRTCDisabledSkipsWebRTCTier(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, "DIRECT")
	}))
	defer backend.Close()

	reg := NewRegistry()
	// Caps.DirectWebRTC is true but webrtc transport is nil (disabled) → skip.
	reg.put(&Peer{ID: "bob", Client: backend.Client(), BaseURL: backend.URL, Caps: Capabilities{DirectWebRTC: true}})
	resolver := &directResolver{reg: reg, webrtc: nil}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tracks/9/audio", nil)
	resolver.ServeRemoteAudio(rec, req, "bob", 42)

	if rec.Code != http.StatusOK || rec.Body.String() != "DIRECT" {
		t.Fatalf("disabled-webrtc = code %d body %q", rec.Code, rec.Body.String())
	}
}
