package fed

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestRouteToDialRefusesMessageFromAnotherPeer is the unit-level guard for the
// negotiation hijack: SignalMsg.From is a claim, not a credential, so a message
// carrying the right SID but the wrong sender must not reach the negotiation.
func TestRouteToDialRefusesMessageFromAnotherPeer(t *testing.T) {
	tr := NewWebRTCTransport("alice", nil, NewSignaler(), nil, testLogger(t))
	in, teardown := tr.registerDial("sid-1", "bob")
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
	in, teardown := tr.registerDial("sid-1", "bob")

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
	sentinel, done := bob.registerDial("sentinel", "mallory")
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
