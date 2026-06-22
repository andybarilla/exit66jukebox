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
