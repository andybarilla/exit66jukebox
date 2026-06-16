package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/auth"
)

func TestStreamAudioUnknownStreamIs404(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/stream/nope.mp3", nil)
	req.RemoteAddr = "127.0.0.1:1234" // loopback passes auth guard
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 for unknown stream, got %d", rec.Code)
	}
}

func TestEventsUnknownStreamIs404(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/streams/nope/events", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 for unknown stream events, got %d", rec.Code)
	}
}

func TestStreamRequiresAuth(t *testing.T) {
	s, _ := newTestServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/stream/house.mp3", nil)
	req.RemoteAddr = "203.0.113.5:1234" // public, not loopback
	s.streamAudioGuarded(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anon: want 401, got %d", rec.Code)
	}
}

func TestStreamAllowsSignedURL(t *testing.T) {
	s, _ := newTestServer(t) // signing secret "test-secret"
	tok := auth.SignPath([]byte("test-secret"), "/stream/house.mp3", 4_000_000_000)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/stream/house.mp3?sig="+tok, nil)
	req.RemoteAddr = "203.0.113.5:1234"
	s.streamAudioGuarded(rec, req)
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("signed token: should pass auth, got 401")
	}
}

func TestStreamAllowsLoopback(t *testing.T) {
	s, _ := newTestServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/stream/house.mp3", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	s.streamAudioGuarded(rec, req)
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("loopback: should pass auth, got 401")
	}
}
