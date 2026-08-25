package fed

import (
	"context"
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
	// A far end that holds its conn open never reacts to the channel close, so
	// the release falls to the linger. Production waits webrtcCloseLinger for
	// that; this test does not need to. The setup deadline is left alone —
	// shortening it would bound the dials themselves.
	alice.closeLinger = 2 * time.Second
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

	// Converge rather than snapshot: the release is asynchronous by design, and
	// pion unwinds a PeerConnection's goroutines asynchronously on top of that.
	// The deadline is generous because a slow machine must not make this fail;
	// what it cannot mask is the leak itself, which never converges at all.
	if !waitForGoroutines(t, before, 20, 90*time.Second) {
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

	// The eviction is synchronous inside Close, but this converges rather than
	// snapshots: a count read at one instant is a test that can fail for reasons
	// that have nothing to do with the code, and the leak it guards does not
	// converge at all, so a deadline cannot mask it.
	deadline := time.Now().Add(10 * time.Second)
	for {
		alice.mu.Lock()
		n := len(alice.cache)
		alice.mu.Unlock()
		if n == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("cache holds %d entries after the conn was closed, want 0", n)
		}
		time.Sleep(20 * time.Millisecond)
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
	conn := newDataChannelConn("bob", pc, dc, rwc)
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

// TestFailedDialLeavesAConcurrentDialsConnCached covers the hazard the doc on
// evictConn describes, on the one path that used to bypass it: Dial's deferred
// failure teardown deleted by peer id.
//
// serveWebRTC dials per HTTP request and nothing serializes dials to one peer,
// so a failing dial's teardown ran while a successful dial's conn was cached
// under that same id. The unscoped delete unmapped it. Nothing then closes it —
// serveWebRTC does not, and get() only prunes entries it still finds in the map
// — so its PeerConnection strands for the process's lifetime. The defer only
// runs when the dial FAILED, so it could never delete anything but another
// dial's entry.
func TestFailedDialLeavesAConcurrentDialsConnCached(t *testing.T) {
	signaler := NewSignaler()
	alice := NewWebRTCTransport("alice", nil, signaler, nil, testLogger(t))
	alice.setupTimeout = time.Second
	defer alice.Close()

	// Bob has a mailbox but never answers, so this dial fails on the setup
	// deadline. Reading the offer out of that mailbox is the ordering guarantee
	// the test needs: once it arrives, the dial is past its own cache lookup and
	// is waiting, which is exactly the window a concurrent dial completes in.
	bobbox := signaler.Register("bob")

	failed := make(chan error, 1)
	go func() {
		_, err := alice.Dial(context.Background(), "bob")
		failed <- err
	}()

	select {
	case msg := <-bobbox:
		if msg.Type != "offer" {
			t.Fatalf("first signal was %q, want an offer", msg.Type)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the dial never sent an offer")
	}

	// Stand in for the conn a concurrent, successful dial to the same peer just
	// cached. Which entry the teardown deletes is decided by identity alone, so
	// the conn needs no channel behind it — nothing reads its state, and the
	// failing dial has already done its lookup.
	live := &dataChannelConn{peerID: "bob"}
	alice.put(live)

	select {
	case err := <-failed:
		if err == nil {
			t.Fatal("the dial succeeded; this test needs the failure path")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the dial never gave up")
	}

	alice.mu.Lock()
	got := alice.cache["bob"]
	alice.mu.Unlock()
	if got != live {
		t.Fatalf("a failed dial evicted a concurrent dial's live conn (got %p want %p); "+
			"nothing prunes an unmapped conn, so its PeerConnection is stranded", got, live)
	}
}

// TestClosingAnOutboundConnLetsTheFarEndSeeTheCloseFirst is the guard on HOW the
// PeerConnection is released, which is as load-bearing as that it is released.
//
// The far end of a detached channel learns we are done only from the SCTP stream
// reset Close sends, and that reset rides this PeerConnection. Closing it inline
// with the conn denies the far end its EOF — and #163 built the answerer's whole
// release on that EOF, so doing it that way traded our leak for theirs. Measured
// under load: TestAnswererReleasesTheNegotiationWhenTheConnIsClosed failed 6 of
// 10 runs with the release inline, and 0 of 10 with it armed.
//
// That test is the end-to-end statement of it but it needs load to fail. This is
// the same rule with no timing in it: the PeerConnection is still up when Close
// returns, and it goes down on its own afterwards.
func TestClosingAnOutboundConnLetsTheFarEndSeeTheCloseFirst(t *testing.T) {
	signaler := NewSignaler()
	alice := NewWebRTCTransport("alice", nil, signaler, nil, testLogger(t))
	bob := NewWebRTCTransport("bob", nil, signaler, nil, testLogger(t))
	alice.closeLinger = 2 * time.Second
	defer alice.Close()
	defer bob.Close()

	// Bob reads until the channel goes away, which is what Manager.Start does.
	sawEOF := make(chan struct{}, 1)
	bob.Listen(t.Context(), func(c *dataChannelConn) {
		go func() {
			buf := make([]byte, 64)
			for {
				if _, err := c.Read(buf); err != nil {
					select {
					case sawEOF <- struct{}{}:
					default:
					}
					return
				}
			}
		}()
	})

	conn, err := alice.Dial(t.Context(), "bob")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if conn.pc == nil {
		t.Fatal("the conn does not know its PeerConnection; the rest of this test proves nothing")
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// pc.Close is synchronous, so an inline release is visible the instant Close
	// returns — no sleep and no load needed to catch it.
	if st := conn.pc.ConnectionState(); st == webrtc.PeerConnectionStateClosed {
		t.Fatal("the PeerConnection was closed inline with the conn; the stream reset " +
			"the far end reads as EOF cannot survive its own transport being torn down")
	}

	select {
	case <-sawEOF:
	case <-time.After(20 * time.Second):
		t.Fatal("the far end never saw the close")
	}

	// Armed, not abandoned: the linger releases it even though this far end never
	// closed its own conn in response.
	deadline := time.Now().Add(30 * time.Second)
	for conn.pc.ConnectionState() != webrtc.PeerConnectionStateClosed {
		if time.Now().After(deadline) {
			t.Fatalf("PeerConnection is %s well past the %v linger; the release was never armed",
				conn.pc.ConnectionState(), alice.closeLinger)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
