package fed

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMemberResolverReachesHubRelay(t *testing.T) {
	reg := NewRegistry()
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()

	// Hub: accept members, serve the relay over their sessions.
	relay := NewRelay(reg, nil)
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
				http.Serve(p.Session, relay.Routes())
			}()
		}
	}()

	// Owner member "home" serves audio.
	ownerMux := http.NewServeMux()
	ownerMux.HandleFunc("/api/tracks/{id}/audio", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "AUDIO")
	})
	home := &Manager{Role: "member", Token: "tok", PeerID: "home", HubAddr: ln.Addr().String(), MemberHandler: ownerMux, Registry: NewRegistry()}
	home.Start()

	// Playing member "vps" connects too; its resolver fetches home's track.
	vps := &Manager{Role: "member", Token: "tok", PeerID: "vps", HubAddr: ln.Addr().String(), Registry: NewRegistry()}
	vps.Start()

	for i := 0; i < 300 && (reg.Get("home") == nil || vps.HubClient() == nil); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	res := NewResolverFor(vps)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/tracks/1/audio", nil)
	res.ServeRemoteAudio(rec, req, "home", 99)

	if rec.Body.String() != "AUDIO" {
		t.Fatalf("expected AUDIO via relay, got %q (code %d)", rec.Body.String(), rec.Code)
	}
}
