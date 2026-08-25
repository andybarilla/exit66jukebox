package fed

import (
	"database/sql"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/store"
	"github.com/hashicorp/yamux"
)

func TestManagerMemberServesHubRequests(t *testing.T) {
	reg := NewRegistry()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	// Hub: accept one peer.
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		_ = acceptPeer(conn, "tok", reg)
	}()

	// Member: dial hub, serve a handler over the session.
	mux := http.NewServeMux()
	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "from-member")
	})
	m := &Manager{Token: "tok", PeerID: "home", HubAddr: ln.Addr().String(), MemberHandler: mux, Registry: NewRegistry()}
	go m.runMember()

	// Wait for registration, then the hub calls the member.
	var peer *Peer
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if peer = reg.Get("home"); peer != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if peer == nil {
		t.Fatal("member never registered with hub")
	}
	resp, err := peer.Client.Get("http://home/hello")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "from-member" {
		t.Fatalf("expected from-member, got %q", body)
	}
}

func TestPeerDialerUsesAcceptedPeerStorageAddress(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	db := openPeerDialerTestDB(t)
	if err := store.SaveFederationPeer(db, store.FederationPeer{PeerID: "peer-a", Address: listener.Addr().String(), Status: store.PeerStatusAccepted}); err != nil {
		t.Fatal(err)
	}

	connected := make(chan struct{})
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		conn.Close()
		close(connected)
	}()

	manager := &Manager{DB: db, Registry: NewRegistry()}
	manager.runPeerDialerOnce()

	select {
	case <-connected:
	case <-time.After(2 * time.Second):
		t.Fatal("peer dialer did not dial accepted peer address")
	}
}

func TestPeerModeFallsBackThroughConfiguredHub(t *testing.T) {
	hubRegistry := NewRegistry()
	hubListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer hubListener.Close()

	hubRelay := NewRelay(hubRegistry, nil)
	hub := &Manager{Role: "hub", Token: "tok", HubListen: hubListener.Addr().String(), HubHandler: hubRelay.Routes(), Registry: hubRegistry}
	go func() {
		for {
			conn, err := hubListener.Accept()
			if err != nil {
				return
			}
			go hub.serveHubConn(conn)
		}
	}()

	peer := &Manager{Role: "peer", Token: "tok", PeerID: "peer-a", HubAddr: hubListener.Addr().String(), HubListen: "127.0.0.1:0", Registry: NewRegistry()}
	peer.Start()

	ownerMux := http.NewServeMux()
	ownerMux.HandleFunc("/api/tracks/{id}/audio", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "from-owner")
	})
	owner := &Manager{Role: "member", Token: "tok", PeerID: "owner", HubAddr: hubListener.Addr().String(), MemberHandler: ownerMux, Registry: NewRegistry()}
	go owner.runMember()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if peer.Registry.Get("@hub") != nil && hubRegistry.Get("owner") != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if peer.Registry.Get("@hub") == nil {
		t.Fatal("peer mode never registered configured hub")
	}
	if hubRegistry.Get("owner") == nil {
		t.Fatal("owner never registered with hub")
	}

	resolver := NewResolverFor(peer)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/audio", nil)
	resolver.ServeRemoteAudio(rec, req, "owner", 42)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "from-owner" {
		t.Fatalf("expected hub relay body, got %q", rec.Body.String())
	}
}

func openPeerDialerTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// startDialTarget stands in for the far end of a peer dial: it accepts the peer
// handshake and serves the session, reporting each accepted peer. Registration
// goes into its own registry, so the dialer's registry is the only one under
// test.
func startDialTarget(t *testing.T) (addr string, accepted <-chan *Peer) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	reg := NewRegistry()
	out := make(chan *Peer, 4)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				p, err := acceptAndRegister(conn, "tok", reg)
				if err != nil {
					return
				}
				out <- p
				_ = http.Serve(p.Session, nil)
			}()
		}
	}()
	return ln.Addr().String(), out
}

// waitSessionClosed polls until sess reports closed, which is how the far end
// observes the dialer tearing its redundant session down.
func waitSessionClosed(t *testing.T, sess *yamux.Session) bool {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if sess.IsClosed() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// In peer role both ends dial each other, so a dial that began while the id was
// free can finish after an inbound session for the same peer was accepted. The
// incumbent must survive and the redundant session must be dropped (#185).
func TestDialDoesNotEvictLiveInboundSession(t *testing.T) {
	reg := NewRegistry()
	incumbent := &Peer{ID: "peer-a", Session: liveTestSession(t)}
	if err := reg.putIfFree(incumbent); err != nil {
		t.Fatalf("registering the incumbent: %v", err)
	}

	addr, accepted := startDialTarget(t)
	m := &Manager{Role: "peer", Token: "tok", PeerID: "self", Registry: reg}

	done := make(chan struct{})
	go func() { m.dialPeer("peer-a", addr); close(done) }()

	var far *Peer
	select {
	case far = <-accepted:
	case <-time.After(2 * time.Second):
		t.Fatal("dial never completed its handshake")
	}

	// Settle on whichever comes first: dialPeer dropping its session, or the
	// incumbent being displaced. Waiting only on done would report a hang for a
	// dial that evicted and carried on serving.
	dropped := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if reg.Get("peer-a") != incumbent {
			break
		}
		select {
		case <-done:
			dropped = true
		default:
		}
		if dropped {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got := reg.Get("peer-a"); got != incumbent {
		t.Fatalf("registry peer = %p, want the incumbent %p", got, incumbent)
	}
	if incumbent.Session.IsClosed() {
		t.Fatal("the completed dial closed the live incumbent session")
	}
	if !dropped {
		t.Fatal("dialPeer kept the redundant session instead of dropping it")
	}
	if !waitSessionClosed(t, far.Session) {
		t.Fatal("the redundant dialled session was left open")
	}
}

// The reconnect path #171 had to protect: an entry whose session has ended is
// reaped by its serving goroutine, so a dial can arrive while the dead entry is
// still present and must not be refused on it.
func TestDialRegistersOverEndedSession(t *testing.T) {
	reg := NewRegistry()
	stale := &Peer{ID: "peer-a", Session: liveTestSession(t)}
	if err := reg.putIfFree(stale); err != nil {
		t.Fatalf("registering the stale peer: %v", err)
	}
	stale.Session.Close()

	addr, accepted := startDialTarget(t)
	m := &Manager{Role: "peer", Token: "tok", PeerID: "self", Registry: reg}
	go m.dialPeer("peer-a", addr)

	var far *Peer
	select {
	case far = <-accepted:
	case <-time.After(2 * time.Second):
		t.Fatal("dial never completed its handshake")
	}
	t.Cleanup(func() { far.Session.Close() })

	var got *Peer
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got = reg.Get("peer-a"); got != nil && got != stale {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got == nil || got == stale {
		t.Fatal("a dial to a peer whose session ended was refused on the dead entry")
	}
	if got.Session.IsClosed() {
		t.Fatal("the registered session is not live")
	}
}
