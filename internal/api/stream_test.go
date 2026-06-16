package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/auth"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

func TestStreamAudioUnknownStreamIs404(t *testing.T) {
	srv, _ := newTestServer(t)
	// A valid signed token passes the auth guard so we reach the unknown-stream
	// 404 rather than the 401 gate.
	tok := auth.SignPath([]byte("test-secret"), "/stream/nope.mp3", 4_000_000_000)
	req := httptest.NewRequest(http.MethodGet, "/stream/nope.mp3?sig="+tok, nil)
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

func TestStreamAllowsSession(t *testing.T) {
	s, db := newTestServer(t)
	uid, _ := store.CreateUser(db, "a@b.com", "A", "h", false)
	raw, _ := auth.GenerateToken()
	store.CreateSession(db, auth.HashToken(raw), uid, 4_000_000_000)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/stream/house.mp3", nil)
	req.RemoteAddr = "203.0.113.5:1234" // not loopback — only the cookie authorizes
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: raw})
	s.streamAudioGuarded(rec, req)
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("session: should pass auth, got 401")
	}
}
