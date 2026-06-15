package fed

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHubRelayForwardsToOwner(t *testing.T) {
	reg := NewRegistry()

	// The hub listens; the owner member "home" dials in and serves its audio
	// endpoint over the session. The relay runs on the hub side against reg.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		p, err := acceptAndRegister(conn, "tok", reg)
		if err != nil {
			return
		}
		http.Serve(p.Session, nil) // member-initiated requests unused in this test
	}()

	ownerMux := http.NewServeMux()
	ownerMux.HandleFunc("/api/tracks/{id}/audio", func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "x.mp3", time.Time{}, strings.NewReader("0123456789"))
	})
	home := &Manager{Role: "member", Token: "tok", PeerID: "home",
		HubAddr: ln.Addr().String(), MemberHandler: ownerMux, Registry: NewRegistry()}
	home.Start()

	// Wait for home to register on the hub, then relay a ranged request to it.
	for i := 0; i < 300 && reg.Get("home") == nil; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if reg.Get("home") == nil {
		t.Fatal("owner 'home' never registered")
	}

	relay := NewRelay(reg, nil) // nil db: this test exercises audio relay only
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/fed/audio/home/42", nil)
	req.SetPathValue("peer", "home")
	req.SetPathValue("id", "42")
	req.Header.Set("Range", "bytes=2-5")
	relay.ServeHTTP(rec, req)

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("expected 206, got %d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if string(body) != "2345" {
		t.Fatalf("expected range bytes 2345, got %q", body)
	}
}
