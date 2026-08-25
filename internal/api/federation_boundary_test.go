package api

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/fed"
	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

// fedRefusedRoutes are application routes that must not be reachable over a
// federation session. Each is protected on the public listener by
// RequireAuthMiddleware (or by an admin check that reads the session cookie),
// neither of which runs on a peer session.
var fedRefusedRoutes = []struct{ method, path string }{
	{http.MethodGet, "/api/streams/house"},
	{http.MethodPost, "/api/streams/house/requests"},
	{http.MethodDelete, "/api/streams/house/requests"},
	{http.MethodPost, "/api/streams/house/next"},
	{http.MethodPost, "/api/streams/house/shuffle"},
	{http.MethodPost, "/api/streams/house/station"},
	{http.MethodGet, "/api/tracks"},
	{http.MethodGet, "/api/admin/settings"},
	{http.MethodGet, "/api/admin/federation/peers"},
	{http.MethodPost, "/api/admin/invites"},
}

// fedHandlers returns the two handlers main.go serves over a federation
// session, built the same way it builds them.
func fedHandlers(srv *Server, db *sql.DB) map[string]http.Handler {
	caps := fed.Capabilities{DirectWebRTC: true}
	return map[string]http.Handler{
		"member": fed.WithCapsRoute(caps, fed.AppRoutes(srv.Handler())),
		"peer":   fed.WithCapsRoute(caps, fed.PeerRoutes(db, srv.Handler())),
	}
}

// TestFederationSessionRefusesApplicationRoutes is the #136 boundary: a peer
// that has completed a federation session reaches the federation routes and
// track audio, and nothing else.
func TestFederationSessionRefusesApplicationRoutes(t *testing.T) {
	srv, db := newTestServer(t)
	for name, h := range fedHandlers(srv, db) {
		for _, r := range fedRefusedRoutes {
			t.Run(name+" "+r.method+" "+r.path, func(t *testing.T) {
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, httptest.NewRequest(r.method, r.path, nil))
				if rec.Code != http.StatusNotFound {
					t.Fatalf("status = %d, want 404 (body %q)", rec.Code, strings.TrimSpace(rec.Body.String()))
				}
			})
		}
	}
}

// TestFederationRefusedRoutesExistOnPublicHandler is the positive control for
// the test above: every path it asserts 404 on is a real route on the public
// handler, so a 404 there means the allowlist refused it rather than that the
// path was misspelled.
func TestFederationRefusedRoutesExistOnPublicHandler(t *testing.T) {
	srv, _ := newTestServer(t)
	for _, r := range fedRefusedRoutes {
		t.Run(r.method+" "+r.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, httptest.NewRequest(r.method, r.path, nil))
			if rec.Code == http.StatusNotFound {
				t.Fatalf("route is not registered on the public handler; the 404 in the boundary test proves nothing")
			}
		})
	}
}

// TestFederationSessionServesTrackAudio covers the one allowlisted application
// route, including the Range request <audio> seeking depends on.
func TestFederationSessionServesTrackAudio(t *testing.T) {
	srv, db := newTestServer(t)
	body := strings.Repeat("a", 1000)
	path := filepath.Join(t.TempDir(), "song.mp3")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write track file: %v", err)
	}
	id, err := store.UpsertTrack(db, model.Track{Path: path, Title: "Song"}, "Artist", "Artist", "Album")
	if err != nil {
		t.Fatalf("upsert track: %v", err)
	}
	audioPath := "/api/tracks/" + strconv.FormatInt(id, 10) + "/audio"

	for name, h := range fedHandlers(srv, db) {
		t.Run(name+" whole file", func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, audioPath, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %q)", rec.Code, strings.TrimSpace(rec.Body.String()))
			}
			if rec.Body.String() != body {
				t.Fatalf("body = %d bytes, want %d", rec.Body.Len(), len(body))
			}
		})
		t.Run(name+" range", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, audioPath, nil)
			req.Header.Set("Range", "bytes=100-199")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusPartialContent {
				t.Fatalf("status = %d, want 206 (body %q)", rec.Code, strings.TrimSpace(rec.Body.String()))
			}
			if got := rec.Header().Get("Content-Range"); got != "bytes 100-199/1000" {
				t.Fatalf("Content-Range = %q, want %q", got, "bytes 100-199/1000")
			}
			if rec.Body.Len() != 100 {
				t.Fatalf("body = %d bytes, want 100", rec.Body.Len())
			}
		})
	}
}

// TestFederationSessionKeepsFedRoutes guards the /fed/* routes the allowlist
// sits alongside: they are served by federation itself and must be unchanged.
func TestFederationSessionKeepsFedRoutes(t *testing.T) {
	srv, db := newTestServer(t)
	h := fedHandlers(srv, db)["peer"]

	for _, path := range []string{"/fed/caps", "/fed/catalog"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200 (body %q)", path, rec.Code, strings.TrimSpace(rec.Body.String()))
		}
	}
}
