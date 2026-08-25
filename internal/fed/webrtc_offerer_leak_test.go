package fed

import (
	"errors"
	"io"
	"runtime"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

// TestOutboundDialsDoNotAccumulatePeerConnections is the leak in issue #168.
//
// Dial's deferred teardown closes the PeerConnection on every FAILURE path and
// deliberately not on the success path, because the conn has to outlive Dial.
// Nothing closed it afterwards: dataChannelConn.Close closed the data channel
// and the rwc, and onClose — the seam that releases a negotiation — was set only
// by the answerer. A dial that SUCCEEDED therefore left its PeerConnection and
// ICE agent running for the process's lifetime.
//
// The far end deliberately holds its conn open. Closing it would tear down the
// answerer's PeerConnection, and that alone winds the offerer's down too, which
// hides the leak entirely: the offerer never has to close anything of its own.
// With the far end up, only the offerer's own close can release it.
//
// The assertion is on the goroutine count, following the leak tests from #163:
// the dial map and the conn cache drain either way, so what separates a released
// PeerConnection from a leaked one is whether its goroutines return. The count
// covers both ends — the offerer's close cascades to the answerer's teardown —
// and that is the point: an outbound close must release everything the dial set
// in motion. Measured before the fix: +145 goroutines per dial, none released.
func TestOutboundDialsDoNotAccumulatePeerConnections(t *testing.T) {
	signaler := NewSignaler()
	alice := NewWebRTCTransport("alice", nil, signaler, nil, testLogger(t))
	bob := NewWebRTCTransport("bob", nil, signaler, nil, testLogger(t))
	defer alice.Close()
	defer bob.Close()

	answered := make(chan struct{}, 8)
	bob.Listen(t.Context(), func(*dataChannelConn) {
		select {
		case answered <- struct{}{}:
		default:
		}
	})

	settle(t)
	before := runtime.NumGoroutine()

	// An instance that redials, which is the shape the issue calls out: each
	// cycle negotiates a fresh PeerConnection and closes the conn it got.
	const dials = 4
	for i := 0; i < dials; i++ {
		conn, err := alice.Dial(t.Context(), "bob")
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		select {
		case <-answered:
		case <-time.After(10 * time.Second):
			t.Fatalf("dial %d: answerer never saw the channel", i)
		}
		if err := conn.Close(); err != nil {
			t.Fatalf("dial %d close: %v", i, err)
		}
	}

	if !waitForGoroutines(t, before, 20, 30*time.Second) {
		t.Fatalf("goroutines grew by %d after %d successful dials that were closed; "+
			"each dial leaked its PeerConnection and ICE agent",
			runtime.NumGoroutine()-before, dials)
	}
}

// TestClosingAnOutboundConnDropsItFromTheCache is the same fix stated without a
// timing window: the conn Dial cached is gone once it is closed, so no later
// request is handed a conn whose PeerConnection has been torn down.
func TestClosingAnOutboundConnDropsItFromTheCache(t *testing.T) {
	signaler := NewSignaler()
	alice := NewWebRTCTransport("alice", nil, signaler, nil, testLogger(t))
	bob := NewWebRTCTransport("bob", nil, signaler, nil, testLogger(t))
	defer alice.Close()
	defer bob.Close()

	answered := make(chan struct{}, 1)
	bob.Listen(t.Context(), func(*dataChannelConn) {
		select {
		case answered <- struct{}{}:
		default:
		}
	})

	conn, err := alice.Dial(t.Context(), "bob")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	select {
	case <-answered:
	case <-time.After(10 * time.Second):
		t.Fatal("answerer never saw the channel")
	}

	alice.mu.Lock()
	cached := alice.cache["bob"] == conn
	alice.mu.Unlock()
	if !cached {
		t.Fatal("a successful dial did not cache its conn; the rest of this test proves nothing")
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	alice.mu.Lock()
	n := len(alice.cache)
	alice.mu.Unlock()
	if n != 0 {
		t.Fatalf("cache holds %d entries after the conn was closed, want 0", n)
	}
}

// TestEvictConnLeavesAReplacementAlone pins the identity guard, which is the
// answer to the second question the issue raises: a conn's close evicts, but
// only its own entry. An unguarded delete would drop a conn a later dial had
// already cached — that dial's caller keeps using it, so the next request
// renegotiates a second PeerConnection to the same peer and the replacement is
// never closed. Evicting the wrong entry converts one leak into another.
func TestEvictConnLeavesAReplacementAlone(t *testing.T) {
	tr := NewWebRTCTransport("alice", nil, NewSignaler(), nil, testLogger(t))

	first := &dataChannelConn{peerID: "bob"}
	second := &dataChannelConn{peerID: "bob"}
	tr.put(first)
	tr.put(second)

	tr.evictConn(first)

	tr.mu.Lock()
	got := tr.cache["bob"]
	tr.mu.Unlock()
	if got != second {
		t.Fatalf("evicting a superseded conn removed the live entry (got %p want %p)", got, second)
	}

	tr.evictConn(second)
	tr.mu.Lock()
	n := len(tr.cache)
	tr.mu.Unlock()
	if n != 0 {
		t.Fatalf("cache holds %d entries after the cached conn was evicted, want 0", n)
	}
}

// TestStaleCachedConnIsClosedNotJustDropped covers the path that reaches the
// leak with no caller involved at all: nothing in production closes an outbound
// conn (the resolver caches and reuses it), so the way one is discarded is get()
// pruning it on the next dial. Dropping it there left the last reference to its
// PeerConnection on the floor.
func TestStaleCachedConnIsClosedNotJustDropped(t *testing.T) {
	tr := NewWebRTCTransport("alice", nil, NewSignaler(), nil, testLogger(t))

	// A real data channel that never opened: ReadyState is "connecting", so
	// open() is false and get() must treat the entry as stale.
	pc, err := tr.api.NewPeerConnection(tr.config)
	if err != nil {
		t.Fatalf("new pc: %v", err)
	}
	defer pc.Close()
	dc, err := pc.CreateDataChannel("audio", nil)
	if err != nil {
		t.Fatalf("create dc: %v", err)
	}
	if dc.ReadyState() == webrtc.DataChannelStateOpen {
		t.Fatal("the fixture channel is open; get() would hand it back and this test would prove nothing")
	}

	rwc := &recordingRWC{}
	conn := newDataChannelConn("bob", dc, rwc)
	released := false
	conn.onClose = func() { released = true }
	tr.put(conn)

	if got := tr.get("bob"); got != nil {
		t.Fatal("get returned a channel that is not open")
	}
	if !released {
		t.Fatal("the stale conn was dropped from the cache without being closed; its PeerConnection is stranded")
	}
	if !rwc.closed {
		t.Fatal("the stale conn's reader/writer was not closed")
	}
	tr.mu.Lock()
	n := len(tr.cache)
	tr.mu.Unlock()
	if n != 0 {
		t.Fatalf("cache holds %d entries after pruning a stale conn, want 0", n)
	}
}

type recordingRWC struct{ closed bool }

func (r *recordingRWC) Read([]byte) (int, error)    { return 0, io.EOF }
func (r *recordingRWC) Write(p []byte) (int, error) { return len(p), nil }
func (r *recordingRWC) Close() error {
	if r.closed {
		return errors.New("closed twice")
	}
	r.closed = true
	return nil
}
