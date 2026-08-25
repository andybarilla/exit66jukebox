package fed

import (
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/hashicorp/yamux"
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

// startTestHub accepts peer connections and registers each one, reporting the
// registration error (nil on success) on the returned channel. It never calls
// reg.remove, so an entry for a peer whose session has ended stays in the
// registry — the stale-entry case a reconnecting peer has to get past.
func startTestHub(t *testing.T, reg *Registry) (addr string, results <-chan error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	out := make(chan error, 4)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				p, err := acceptAndRegister(conn, "tok", reg)
				out <- err
				if err != nil {
					return
				}
				http.Serve(p.Session, nil)
			}()
		}
	}()
	return ln.Addr().String(), out
}

// dialTestMember is the member side: handshake as peerID, then serve /who
// returning name over the session. The caller closes the returned session.
func dialTestMember(t *testing.T, addr, peerID, name string) *yamux.Session {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	if err := dialHandshake(conn, "tok", peerID); err != nil {
		t.Fatalf("handshake as %s: %v", peerID, err)
	}
	sess, err := yamux.Client(conn, nil)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/who", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, name) })
	go http.Serve(sess, mux)
	return sess
}

// readPeerWho calls /who over the peer's session, proving the session is not
// merely present in the registry but still usable.
func readPeerWho(t *testing.T, p *Peer) string {
	t.Helper()
	resp, err := p.Client.Get(p.BaseURL + "/who")
	if err != nil {
		t.Fatalf("request over peer session: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body)
}

func TestRegisterRefusesIDHoldingLiveSession(t *testing.T) {
	reg := NewRegistry()
	addr, results := startTestHub(t, reg)

	incumbentSess := dialTestMember(t, addr, "home", "incumbent")
	defer incumbentSess.Close()
	if err := <-results; err != nil {
		t.Fatalf("first registration: %v", err)
	}
	incumbent := reg.Get("home")
	if incumbent == nil {
		t.Fatal("incumbent never registered")
	}

	usurperSess := dialTestMember(t, addr, "home", "usurper")
	defer usurperSess.Close()
	if err := <-results; err == nil {
		t.Fatal("registering an id that holds a live session must be refused")
	}

	if got := reg.Get("home"); got != incumbent {
		t.Fatalf("registry peer = %p, want the incumbent %p", got, incumbent)
	}
	if incumbent.Session.IsClosed() {
		t.Fatal("incumbent session was closed by the refused registration")
	}
	if who := readPeerWho(t, incumbent); who != "incumbent" {
		t.Fatalf("/who over incumbent session = %q, want %q", who, "incumbent")
	}
}

func TestRegisterReclaimsIDAfterSessionEnds(t *testing.T) {
	reg := NewRegistry()
	addr, results := startTestHub(t, reg)

	firstSess := dialTestMember(t, addr, "home", "first")
	if err := <-results; err != nil {
		t.Fatalf("first registration: %v", err)
	}
	first := reg.Get("home")
	if first == nil {
		t.Fatal("first peer never registered")
	}

	firstSess.Close()
	deadline := time.Now().Add(2 * time.Second)
	for !first.Session.IsClosed() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !first.Session.IsClosed() {
		t.Fatal("hub side never saw the first session close")
	}

	secondSess := dialTestMember(t, addr, "home", "second")
	defer secondSess.Close()
	if err := <-results; err != nil {
		t.Fatalf("re-registering after the session ended must succeed: %v", err)
	}
	second := reg.Get("home")
	if second == first {
		t.Fatal("registry still holds the dead session")
	}
	if who := readPeerWho(t, second); who != "second" {
		t.Fatalf("/who over reconnected session = %q, want %q", who, "second")
	}
}
