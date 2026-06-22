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

func TestPeerResolverStreamsDirectAudioWithRange(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tracks/42/audio" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Range") != "bytes=2-5" {
			t.Fatalf("range header was not forwarded: %q", r.Header.Get("Range"))
		}
		w.Header().Set("Content-Range", "bytes 2-5/8")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Content-Length", "4")
		w.WriteHeader(http.StatusPartialContent)
		io.WriteString(w, "CDEF")
	}))
	defer backend.Close()

	reg := NewRegistry()
	reg.put(&Peer{ID: "peer-a", Client: backend.Client(), BaseURL: backend.URL})
	resolver := NewResolverFor(&Manager{Role: "peer", Registry: reg})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tracks/9/audio", nil)
	req.Header.Set("Range", "bytes=2-5")

	resolver.ServeRemoteAudio(rec, req, "peer-a", 42)

	if rec.Code != http.StatusPartialContent || rec.Body.String() != "CDEF" {
		t.Fatalf("direct audio = code %d body %q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Range") != "bytes 2-5/8" {
		t.Fatalf("content-range not preserved: %q", rec.Header().Get("Content-Range"))
	}
	if rec.Header().Get("Accept-Ranges") != "bytes" {
		t.Fatalf("accept-ranges not preserved: %q", rec.Header().Get("Accept-Ranges"))
	}
	if rec.Header().Get("Content-Type") != "audio/mpeg" {
		t.Fatalf("content-type not preserved: %q", rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("Content-Length") != "4" {
		t.Fatalf("content-length not preserved: %q", rec.Header().Get("Content-Length"))
	}
}

func TestPeerResolverFallsBackToHubWhenDirectPeerOffline(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fed/audio/peer-a/42" {
			t.Fatalf("unexpected hub path: %s", r.URL.Path)
		}
		io.WriteString(w, "RELAY")
	}))
	defer hub.Close()

	reg := NewRegistry()
	reg.put(&Peer{ID: "@hub", Client: hub.Client(), BaseURL: hub.URL})
	resolver := NewResolverFor(&Manager{Role: "peer", Registry: reg})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tracks/9/audio", nil)

	resolver.ServeRemoteAudio(rec, req, "peer-a", 42)

	if rec.Code != http.StatusOK || rec.Body.String() != "RELAY" {
		t.Fatalf("fallback audio = code %d body %q", rec.Code, rec.Body.String())
	}
}

func TestPeerResolverFallsBackToHubWhenDirectFetchFails(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "RELAY")
	}))
	defer hub.Close()

	reg := NewRegistry()
	reg.put(&Peer{ID: "peer-a", Client: http.DefaultClient, BaseURL: "http://127.0.0.1:1"})
	reg.put(&Peer{ID: "@hub", Client: hub.Client(), BaseURL: hub.URL})
	resolver := NewResolverFor(&Manager{Role: "peer", Registry: reg})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tracks/9/audio", nil)

	resolver.ServeRemoteAudio(rec, req, "peer-a", 42)

	if rec.Code != http.StatusOK || rec.Body.String() != "RELAY" {
		t.Fatalf("fallback audio = code %d body %q", rec.Code, rec.Body.String())
	}
}
