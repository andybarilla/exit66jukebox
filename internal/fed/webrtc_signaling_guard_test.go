package fed

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

// TestRouteToDialRefusesMessageFromAnotherPeer is the unit-level guard for the
// negotiation hijack: SignalMsg.From is a claim, not a credential, so a message
// carrying the right SID but the wrong sender must not reach the negotiation.
func TestRouteToDialRefusesMessageFromAnotherPeer(t *testing.T) {
	tr := NewWebRTCTransport("alice", nil, NewSignaler(), nil, testLogger(t))
	in, teardown, ok := tr.registerDial("sid-1", "bob")
	if !ok {
		t.Fatal("registerDial refused a fresh sid")
	}
	defer teardown()

	tr.routeToDial(SignalMsg{SID: "sid-1", From: "mallory", Type: "answer", SDP: "hijack"})
	select {
	case msg := <-in:
		t.Fatalf("accepted a message from mallory into bob's negotiation: %+v", msg)
	default:
	}

	// Positive control: the same message from the right peer is delivered, so the
	// assertion above is refusing the sender rather than dropping everything.
	tr.routeToDial(SignalMsg{SID: "sid-1", From: "bob", Type: "answer", SDP: "real"})
	select {
	case msg := <-in:
		if msg.SDP != "real" {
			t.Fatalf("delivered %q want %q", msg.SDP, "real")
		}
	default:
		t.Fatal("dropped the legitimate peer's message too")
	}
}

// TestNewSIDIsNotGuessable guards the second half of the hijack fix. The SID was
// selfID + "-" + an incrementing counter, so an attacker who knew the peer id —
// which every federated peer does — could spray the whole live keyspace with a
// few dozen requests.
func TestNewSIDIsNotGuessable(t *testing.T) {
	tr := NewWebRTCTransport("alice", nil, NewSignaler(), nil, testLogger(t))

	const draws = 1000
	seen := make(map[string]bool, draws)
	for i := 0; i < draws; i++ {
		sid := tr.newSID()
		if sid == "" {
			t.Fatal("newSID returned empty")
		}
		if len(sid) < 32 {
			t.Fatalf("sid %q is %d chars; too small a keyspace to resist a spray", sid, len(sid))
		}
		if strings.Contains(sid, tr.selfID) {
			t.Fatalf("sid %q embeds the peer id, which every peer knows", sid)
		}
		if seen[sid] {
			t.Fatalf("sid %q repeated within %d draws", sid, draws)
		}
		seen[sid] = true
	}

	// The old scheme's ids were exactly this. Nothing in the sequence may match a
	// pattern an attacker can enumerate.
	for i := 1; i <= draws; i++ {
		guess := tr.selfID + "-" + itoa(i)
		if seen[guess] {
			t.Fatalf("sid %q is guessable from the peer id and a counter", guess)
		}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// TestRegisterDialTeardownClosesTheChannel guards the leak fix. Teardown used to
// only unmap the slot, so whoever was ranging over the channel — handleOffer's
// ICE pump, and Dial's answer pump — blocked on it for the process's lifetime.
func TestRegisterDialTeardownClosesTheChannel(t *testing.T) {
	tr := NewWebRTCTransport("alice", nil, NewSignaler(), nil, testLogger(t))
	in, teardown, ok := tr.registerDial("sid-1", "bob")
	if !ok {
		t.Fatal("registerDial refused a fresh sid")
	}

	drained := make(chan struct{})
	go func() {
		for range in { // returns only when the channel is closed
		}
		close(drained)
	}()

	teardown()
	select {
	case <-drained:
	case <-time.After(5 * time.Second):
		t.Fatal("the range over the dial channel never returned: teardown did not close it")
	}

	// Idempotent: a second teardown (the answerer calls it from several paths)
	// must not panic on a double close.
	teardown()

	tr.dialMu.Lock()
	n := len(tr.dial)
	tr.dialMu.Unlock()
	if n != 0 {
		t.Fatalf("dial map holds %d entries after teardown, want 0", n)
	}
}

// TestInboundOffersDoNotAccumulate is the wire-reachable leak: an inbound offer
// is one POST carrying an attacker-chosen SID, so anything handleOffer retains
// is retained per request. Every one of these offers fails at
// SetRemoteDescription, and none may leave a dial entry or a goroutine behind.
func TestInboundOffersDoNotAccumulate(t *testing.T) {
	signaler := NewSignaler()
	bob := NewWebRTCTransport("bob", nil, signaler, nil, testLogger(t))
	defer bob.Close()
	bob.Listen(t.Context(), func(*dataChannelConn) {})

	settle(t)
	before := runtime.NumGoroutine()

	const offers = 50
	for i := 0; i < offers; i++ {
		signaler.Send(SignalMsg{
			From: "mallory", To: "bob", Type: "offer",
			SID: "sprayed-" + itoa(i), SDP: "not a session description",
		})
	}

	deadline := time.Now().Add(20 * time.Second)
	for {
		bob.dialMu.Lock()
		n := len(bob.dial)
		bob.dialMu.Unlock()
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d of %d sprayed offers still hold a dial entry", n, offers)
		}
		time.Sleep(20 * time.Millisecond)
	}

	settle(t)
	if grew := runtime.NumGoroutine() - before; grew > offers/2 {
		t.Fatalf("goroutines grew by %d after %d failed offers; the ICE pumps are not exiting", grew, offers)
	}
}

// settle gives finished goroutines a chance to be reaped before a count is read.
func settle(t *testing.T) {
	t.Helper()
	for i := 0; i < 20; i++ {
		runtime.GC()
		time.Sleep(20 * time.Millisecond)
	}
}

// TestReaderClaimsOfferSIDBeforeHandlingTheNextMessage pins the ordering fix.
// The offerer starts gathering before it sends its offer, so its ICE can be
// right behind; routeToDial drops an unknown SID silently. Registering the slot
// on the reader goroutine — before dispatching handleOffer — means anything the
// reader sees after an offer has somewhere to go.
func TestReaderClaimsOfferSIDBeforeHandlingTheNextMessage(t *testing.T) {
	signaler := NewSignaler()
	bob := NewWebRTCTransport("bob", nil, signaler, nil, testLogger(t))
	defer bob.Close()
	bob.Listen(t.Context(), func(*dataChannelConn) {})

	// A slot the test owns, used purely as a sentinel: when a message addressed
	// to it comes out, the reader has necessarily finished with the offer that
	// went in ahead of it.
	sentinel, done, ok := bob.registerDial("sentinel", "mallory")
	if !ok {
		t.Fatal("registerDial refused a fresh sid")
	}
	defer done()

	signaler.Send(SignalMsg{From: "mallory", To: "bob", Type: "offer", SID: "offer-sid", SDP: "not a session description"})
	signaler.Send(SignalMsg{From: "mallory", To: "bob", Type: "answer", SID: "sentinel"})

	select {
	case <-sentinel:
	case <-time.After(10 * time.Second):
		t.Fatal("reader never reached the sentinel")
	}

	bob.dialMu.Lock()
	_, claimed := bob.dial["offer-sid"]
	bob.dialMu.Unlock()
	if !claimed {
		t.Fatal("the offer's SID was not claimed before the reader moved on; ICE arriving now would be dropped")
	}
}

// TestHalfOpenOffersAreTornDownByTheWatchdog covers the leak's other half. The
// tests above spray malformed offers, which fail at SetRemoteDescription and
// take handleOffer's explicit error path. This is the shape that actually
// threatens the process: well-formed offers, answered normally, from a peer that
// then never completes the negotiation. Nothing on the error path fires, so only
// the watchdog releases them.
func TestHalfOpenOffersAreTornDownByTheWatchdog(t *testing.T) {
	signaler := NewSignaler()
	bob := NewWebRTCTransport("bob", nil, signaler, nil, testLogger(t))
	bob.setupTimeout = 300 * time.Millisecond
	defer bob.Close()
	bob.Listen(t.Context(), func(*dataChannelConn) {})

	// Bob's answers must be deliverable, or the send-failure path would tear the
	// negotiation down and the watchdog would never be what released it. Give
	// mallory a mailbox and drain it into the void — mallory never replies.
	mallorybox := signaler.Register("mallory")
	go func() {
		for range mallorybox {
		}
	}()

	offerSDP := wellFormedOffer(t)
	const offers = 10
	for i := 0; i < offers; i++ {
		signaler.Send(SignalMsg{From: "mallory", To: "bob", Type: "offer", SID: "halfopen-" + itoa(i), SDP: offerSDP})
	}

	// They must be live first, or a drained map would prove nothing.
	if !waitForDialCount(t, bob, func(n int) bool { return n > 0 }, 5*time.Second) {
		t.Fatal("no negotiation was ever registered; the spray did not reach handleOffer")
	}
	if !waitForDialCount(t, bob, func(n int) bool { return n == 0 }, 15*time.Second) {
		bob.dialMu.Lock()
		n := len(bob.dial)
		bob.dialMu.Unlock()
		t.Fatalf("%d of %d half-open negotiations survived the watchdog", n, offers)
	}
}

// TestAnswererReleasesTheNegotiationWhenTheConnIsClosed covers the path the
// watchdog hands off to once a channel is established, and the reason that path
// is not dc.OnClose.
//
// The channel is detached, so pion reports the far end going away as an EOF to
// the reader — OnClose does not fire. An earlier version of this fix relied on
// OnClose and left every established answerer negotiation held indefinitely;
// this test is what caught it. Release now follows the conn, exactly as
// Manager.Start drives it: serve until the peer is done, then Close.
func TestAnswererReleasesTheNegotiationWhenTheConnIsClosed(t *testing.T) {
	alice := newSignalProcess(t, "alice")
	bob := newSignalProcess(t, "bob")
	alice.knows(bob)
	bob.knows(alice)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Mirror Manager.Start: read until the peer goes away, then close.
	served := make(chan struct{}, 1)
	bob.transport.Listen(ctx, func(c *dataChannelConn) {
		go func() {
			defer func() {
				c.Close()
				select {
				case served <- struct{}{}:
				default:
				}
			}()
			buf := make([]byte, 64)
			for {
				if _, err := c.Read(buf); err != nil {
					return
				}
			}
		}()
	})

	conn, err := alice.transport.Dial(ctx, "bob")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if !waitForDialCount(t, bob.transport, func(n int) bool { return n > 0 }, 5*time.Second) {
		t.Fatal("bob holds no negotiation for an open channel")
	}

	conn.Close()
	select {
	case <-served:
	case <-time.After(15 * time.Second):
		t.Fatal("bob's reader never saw the channel close")
	}
	if !waitForDialCount(t, bob.transport, func(n int) bool { return n == 0 }, 15*time.Second) {
		t.Fatal("bob still holds the negotiation after its conn was closed")
	}
}

// wellFormedOffer returns a real SDP offer, so handleOffer gets past
// SetRemoteDescription instead of bailing on its error path.
func wellFormedOffer(t *testing.T) string {
	t.Helper()
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("offer pc: %v", err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	if _, err := pc.CreateDataChannel("audio", nil); err != nil {
		t.Fatalf("offer dc: %v", err)
	}
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		t.Fatalf("create offer: %v", err)
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		t.Fatalf("set local: %v", err)
	}
	return offer.SDP
}

// waitForDialCount polls the in-flight negotiation count until want is satisfied.
func waitForDialCount(t *testing.T, tr *WebRTCTransport, want func(int) bool, within time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		tr.dialMu.Lock()
		n := len(tr.dial)
		tr.dialMu.Unlock()
		if want(n) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestDuplicateOfferSIDsDoNotOrphanNegotiations covers the leak that survives a
// correct teardown: registering a SID that is already claimed.
//
// Storing unconditionally left the incumbent slot unmapped and unclosed — its
// teardown is identity-guarded, so it became a no-op — and its ICE pump and
// PeerConnection then lived for the process's lifetime. Inbound SIDs are
// attacker-chosen, so it cost one POST per orphan.
//
// This asserts on the GOROUTINE COUNT, deliberately. The dial map still drains
// to zero in the broken case, so len(t.dial) shows nothing: the orphans are
// exactly the slots no longer in the map. Both existing leak tests use unique
// SIDs and are blind to this.
func TestDuplicateOfferSIDsDoNotOrphanNegotiations(t *testing.T) {
	signaler := NewSignaler()
	bob := NewWebRTCTransport("bob", nil, signaler, nil, testLogger(t))
	bob.setupTimeout = 500 * time.Millisecond
	defer bob.Close()
	bob.Listen(t.Context(), func(*dataChannelConn) {})

	// Answers must be deliverable, or the send-failure path would tear each
	// negotiation down and hide the orphaning.
	mallorybox := signaler.Register("mallory")
	go func() {
		for range mallorybox {
		}
	}()
	offerSDP := wellFormedOffer(t)

	settle(t)
	before := runtime.NumGoroutine()

	const offers = 30
	for i := 0; i < offers; i++ {
		signaler.Send(SignalMsg{From: "mallory", To: "bob", Type: "offer", SID: "same", SDP: offerSDP})
	}

	// The map drains either way; that is the point. What separates the two is
	// whether the goroutines those offers started ever come back.
	if !waitForDialCount(t, bob, func(n int) bool { return n == 0 }, 20*time.Second) {
		t.Fatal("negotiations never released their slots")
	}
	if !waitForGoroutines(t, before, 10, 30*time.Second) {
		t.Fatalf("goroutines grew by %d after %d offers sharing one SID; "+
			"each duplicate orphaned a pump and a PeerConnection",
			runtime.NumGoroutine()-before, offers)
	}
}

// TestRegisterDialRefusesAClaimedSID is the unit-level statement of the same
// rule, and of the choice to refuse rather than replace: replacing would let a
// peer tear down a negotiation already in flight by naming its SID.
func TestRegisterDialRefusesAClaimedSID(t *testing.T) {
	tr := NewWebRTCTransport("alice", nil, NewSignaler(), nil, testLogger(t))

	first, teardown, ok := tr.registerDial("sid-1", "bob")
	if !ok {
		t.Fatal("a fresh sid was refused")
	}
	if _, _, ok := tr.registerDial("sid-1", "mallory"); ok {
		t.Fatal("a claimed sid was handed out twice; the first slot is now orphaned")
	}

	// The incumbent is untouched: still mapped, still open, still bob's.
	tr.routeToDial(SignalMsg{SID: "sid-1", From: "bob", Type: "answer", SDP: "real"})
	select {
	case msg := <-first:
		if msg.SDP != "real" {
			t.Fatalf("incumbent received %q", msg.SDP)
		}
	default:
		t.Fatal("the refused duplicate disturbed the negotiation already in flight")
	}

	// And the sid is reusable once the incumbent is done.
	teardown()
	if _, _, ok := tr.registerDial("sid-1", "mallory"); !ok {
		t.Fatal("sid stayed claimed after teardown")
	}
}

// waitForGoroutines polls until the count is within tolerance of base. Teardown
// unwinds a PeerConnection's goroutines asynchronously, so this converges rather
// than sampling once.
func waitForGoroutines(t *testing.T, base, tolerance int, within time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		settle(t)
		if runtime.NumGoroutine()-base <= tolerance {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
	}
}
