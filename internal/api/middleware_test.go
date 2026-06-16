package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/auth"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
}

func TestMiddlewareBlocksAnonAPI(t *testing.T) {
	s, _ := newTestServer(t)
	h := s.RequireAuthMiddleware(okHandler())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/tracks", nil)
	req.RemoteAddr = "203.0.113.9:1" // public, not loopback
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anon /api/tracks: want 401, got %d", rec.Code)
	}
}

func TestMiddlewareAllowsOpenPaths(t *testing.T) {
	s, _ := newTestServer(t)
	h := s.RequireAuthMiddleware(okHandler())
	for _, p := range []string{"/api/config", "/api/auth/login", "/api/auth/me"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", p, nil)
		req.RemoteAddr = "203.0.113.9:1"
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("open path %s: want 200, got %d", p, rec.Code)
		}
	}
}

func TestMiddlewareAllowsStaticAndStream(t *testing.T) {
	s, _ := newTestServer(t)
	h := s.RequireAuthMiddleware(okHandler())
	for _, p := range []string{"/", "/assets/app.js", "/stream/house.mp3"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", p, nil)
		req.RemoteAddr = "203.0.113.9:1"
		h.ServeHTTP(rec, req)
		if rec.Code != 200 { // non-/api paths pass the middleware (their own handlers/guards apply downstream)
			t.Fatalf("path %s: want pass-through 200, got %d", p, rec.Code)
		}
	}
}

func TestMiddlewareAllowsSessionAndLoopback(t *testing.T) {
	s, db := newTestServer(t)
	uid, _ := store.CreateUser(db, "a@b.com", "A", "h", false)
	raw, _ := auth.GenerateToken()
	store.CreateSession(db, auth.HashToken(raw), uid, 4_000_000_000)
	h := s.RequireAuthMiddleware(okHandler())

	// session cookie
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/tracks", nil)
	req.RemoteAddr = "203.0.113.9:1"
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: raw})
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("session: want 200, got %d", rec.Code)
	}

	// loopback (ffmpeg house source), no cookie
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "/api/tracks/5/audio", nil)
	req2.RemoteAddr = "127.0.0.1:5555"
	h.ServeHTTP(rec2, req2)
	if rec2.Code != 200 {
		t.Fatalf("loopback: want 200, got %d", rec2.Code)
	}
}
