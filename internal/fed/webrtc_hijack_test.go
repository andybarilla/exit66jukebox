package fed

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

// TestDialRefusesAnswerInjectedByAThirdPeer is the end-to-end form of the
// negotiation hijack, through the real wire path.
//
// Signaling arrives over HTTP from anything holding a federation session, and
// SignalMsg.From is set by the sender. Whoever lands an answer on an in-flight
// Dial first supplies the remote description — the loser's error is discarded —
// and the channel is then cached under the peer the Dial was FOR, not the peer
// that answered. So a successful injection either substitutes the audio source
// or, as here, denies the negotiation outright.
//
// This test gives the attacker more than the real one has: mallory is handed the
// true SID and alice's real offer SDP, rather than having to guess a SID.
// newSID's randomness is what makes guessing infeasible (see
// TestNewSIDIsNotGuessable); this test isolates the other guard, routeToDial's
// check that the sender is the peer the negotiation belongs to.
func TestDialRefusesAnswerInjectedByAThirdPeer(t *testing.T) {
	alice := newSignalProcess(t, "alice")
	bob := newSignalProcess(t, "bob")
	alice.knows(bob)
	bob.knows(alice)

	// Hold bob's offer so the negotiation stays in flight with no answer yet —
	// the window an injector needs. The offer is captured on its way in.
	bob.holdOffers = make(chan struct{})
	captured := make(chan SignalMsg, 4)
	bob.onSignal = func(msg SignalMsg) {
		if msg.Type == "offer" {
			select {
			case captured <- msg:
			default:
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	// Bob answers honestly and identifies himself on the channel.
	bob.transport.Listen(ctx, func(c *dataChannelConn) {
		go func() { _, _ = c.Write([]byte("BOB")) }()
	})

	dialed := make(chan error, 1)
	conns := make(chan *dataChannelConn, 1)
	go func() {
		c, err := alice.transport.Dial(ctx, "bob")
		if err != nil {
			dialed <- err
			return
		}
		conns <- c
		dialed <- nil
	}()

	var offer SignalMsg
	select {
	case offer = <-captured:
	case <-time.After(10 * time.Second):
		t.Fatal("alice's offer never reached bob")
	}
	if offer.SID == "" || offer.SDP == "" {
		t.Fatalf("captured offer is not usable: %+v", offer)
	}

	// Mallory answers alice's offer for real, under alice's true SID, claiming
	// its own identity. Non-trickle: gathering completes before the answer is
	// sent, so the SDP stands alone.
	injected := mallorysAnswerTo(t, offer.SDP)
	body, err := json.Marshal(SignalMsg{From: "mallory", To: "alice", Type: "answer", SID: offer.SID, SDP: injected})
	if err != nil {
		t.Fatalf("marshal injection: %v", err)
	}
	resp, err := http.Post(alice.srv.URL+"/fed/signal/alice", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("mallory could not reach alice's signal endpoint: %v", err)
	}
	resp.Body.Close()
	// The relay accepts it into alice's mailbox — the refusal happens at
	// routeToDial, which is the layer that knows which negotiation it is for.
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("injection was not even delivered to the mailbox (status %d); "+
			"this test would then prove nothing about routeToDial", resp.StatusCode)
	}

	// Let it sit ahead of bob's answer in alice's queue, then release bob.
	time.Sleep(300 * time.Millisecond)
	close(bob.holdOffers)

	select {
	case err := <-dialed:
		if err != nil {
			t.Fatalf("mallory's injected answer broke a negotiation between alice and bob: %v", err)
		}
	case <-time.After(25 * time.Second):
		t.Fatal("dial never returned after the injection")
	}

	conn := <-conns
	defer conn.Close()
	buf := make([]byte, 16)
	readDone := make(chan int, 1)
	go func() {
		n, rerr := conn.Read(buf)
		if rerr != nil {
			readDone <- 0
			return
		}
		readDone <- n
	}()
	select {
	case n := <-readDone:
		if got := string(buf[:n]); got != "BOB" {
			t.Fatalf("channel is talking to %q, want BOB: alice connected to the injector", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no data on the channel")
	}
}

// mallorysAnswerTo builds a genuine SDP answer to offerSDP, with ICE gathering
// completed so the answer is self-contained. A malformed answer would be
// rejected by SetRemoteDescription and discarded, which would make this test
// pass whether or not the sender check exists.
func mallorysAnswerTo(t *testing.T, offerSDP string) string {
	t.Helper()
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("mallory pc: %v", err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offerSDP}); err != nil {
		t.Fatalf("mallory set remote: %v", err)
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("mallory create answer: %v", err)
	}
	gathered := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		t.Fatalf("mallory set local: %v", err)
	}
	select {
	case <-gathered:
	case <-time.After(10 * time.Second):
		t.Fatal("mallory's ICE gathering did not complete")
	}
	return pc.LocalDescription().SDP
}
