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

// TestSignalRelayHandlerAcceptsAndRelays verifies the HTTP handler mounted on
// the hub session: a POST with a JSON body is relayed to the recipient mailbox.
func TestSignalRelayHandlerAcceptsAndRelays(t *testing.T) {
	s := NewSignaler()
	mailbox := s.Register("bob")
	defer s.Unregister("bob", mailbox)

	h := WithSignalRelay(s, http.NewServeMux())
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

// TestSignalRelayHandlerRejectsOfflineRecipient verifies the handler returns
// 503 when the recipient peer is not registered.
func TestSignalRelayHandlerRejectsOfflineRecipient(t *testing.T) {
	s := NewSignaler()
	h := WithSignalRelay(s, http.NewServeMux())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/fed/signal/ghost", strings.NewReader(`{}`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d want 503", rec.Code)
	}
}
