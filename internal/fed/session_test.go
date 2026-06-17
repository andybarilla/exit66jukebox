package fed

import (
	"net"
	"testing"
	"time"
)

func TestHandshakeAcceptsValidToken(t *testing.T) {
	reg := NewRegistry()
	cConn, sConn := net.Pipe()

	go func() { _ = acceptPeer(sConn, "good-token", reg) }()

	if err := dialHandshake(cConn, "good-token", "home"); err != nil {
		t.Fatalf("handshake failed: %v", err)
	}
	// Registry sees the peer within a moment.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if reg.Get("home") != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("peer 'home' never registered")
}

func TestHandshakeRejectsBadToken(t *testing.T) {
	reg := NewRegistry()
	cConn, sConn := net.Pipe()
	go func() { _ = acceptPeer(sConn, "good-token", reg) }()
	if err := dialHandshake(cConn, "wrong", "home"); err == nil {
		t.Fatal("expected rejection on bad token")
	}
	if reg.Get("home") != nil {
		t.Fatal("bad-token peer must not register")
	}
}

func TestRegistryRemovePeerKeepsNewerSession(t *testing.T) {
	reg := NewRegistry()
	oldPeer := &Peer{ID: "peer-a"}
	newPeer := &Peer{ID: "peer-a"}
	reg.put(oldPeer)
	reg.put(newPeer)

	reg.remove("peer-a", oldPeer)

	if got := reg.Get("peer-a"); got != newPeer {
		t.Fatalf("registry peer = %p, want newer peer %p", got, newPeer)
	}
}
