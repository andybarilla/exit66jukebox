package fed

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSignalerRelaysBetweenRegisteredPeers verifies the hub-style relay path:
// a message addressed to a registered peer is delivered to its mailbox, and the
// POST /fed/signal/{to} handler accepts and relays a JSON SignalMsg.
func TestSignalerRelaysBetweenRegisteredPeers(t *testing.T) {
	s := NewSignaler()
	mailbox := s.Register("bob")
	defer s.Unregister("bob", mailbox)

	if !s.Send(SignalMsg{From: "alice", To: "bob", Type: "offer", SDP: "sdp-offer"}) {
		t.Fatal("Send to registered peer returned false")
	}
	select {
	case msg := <-mailbox:
		if msg.From != "alice" || msg.To != "bob" || msg.Type != "offer" || msg.SDP != "sdp-offer" {
			t.Fatalf("relayed message = %#v", msg)
		}
	default:
		t.Fatal("message not delivered to mailbox")
	}
}

// TestSignalerRejectsUnregisteredRecipient verifies Send returns false (so the
// caller treats negotiation as failed and falls back) when the recipient has no
// mailbox — preserving the authenticated-peers-only property.
func TestSignalerRejectsUnregisteredRecipient(t *testing.T) {
	s := NewSignaler()
	if s.Send(SignalMsg{From: "alice", To: "nobody", Type: "offer"}) {
		t.Fatal("Send to unknown peer should return false")
	}
}

// TestSignalRelayHandlerAcceptsAndRelays verifies local delivery: a POST with a
// JSON body lands in the recipient's mailbox in this process. The nil forwarder
// is the peer-session shape; the hub's onward half is in
// webrtc_hub_relay_test.go.
func TestSignalRelayHandlerAcceptsAndRelays(t *testing.T) {
	s := NewSignaler()
	mailbox := s.Register("bob")
	defer s.Unregister("bob", mailbox)

	h := WithSignalRelay(s, nil, http.NewServeMux())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/fed/signal/bob", strings.NewReader(
		`{"from":"alice","to":"bob","type":"ice","ice":"candidate:..."}`))
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d want 202", rec.Code)
	}
	select {
	case msg := <-mailbox:
		if msg.Type != "ice" || msg.ICECandidate != "candidate:..." {
			t.Fatalf("relayed msg = %#v", msg)
		}
	default:
		t.Fatal("handler did not deliver to mailbox")
	}
}

// TestSignalRelayHandlerRejectsOfflineRecipient verifies a relay with no
// forwarder returns 503 when the recipient peer is not registered here. That is
// every composition but the hub's.
func TestSignalRelayHandlerRejectsOfflineRecipient(t *testing.T) {
	s := NewSignaler()
	h := WithSignalRelay(s, nil, http.NewServeMux())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/fed/signal/ghost", strings.NewReader(`{}`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d want 503", rec.Code)
	}
}

// TestSignalRelayRejectsOversizedBody bounds the decode. The route is reachable
// by anything holding a federation session, and an unbounded body would let one
// request hold a goroutine for as long as it cared to keep sending.
func TestSignalRelayRejectsOversizedBody(t *testing.T) {
	s := NewSignaler()
	ch := s.Register("bob")
	h := WithSignalRelay(s, nil, http.NewServeMux())

	body := `{"type":"offer","sdp":"` + strings.Repeat("A", maxSignalBody+1) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/fed/signal/bob", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an oversized signal", rec.Code)
	}
	select {
	case msg := <-ch:
		t.Fatalf("an oversized body was relayed anyway: %d bytes of SDP", len(msg.SDP))
	default:
	}

	// Positive control: a normal signal on the same route still gets through, so
	// the 400 above is the size limit rather than a broken handler.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/fed/signal/bob",
		strings.NewReader(`{"type":"offer","sdp":"v=0"}`)))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("normal signal status = %d, want 202", rec.Code)
	}
	if msg := <-ch; msg.SDP != "v=0" {
		t.Fatalf("relayed SDP = %q", msg.SDP)
	}
}
