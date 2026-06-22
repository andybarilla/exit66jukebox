package fed

import (
	"database/sql"
	"io"
	"net"
	"net/http"
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

func openPeerDialerTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
