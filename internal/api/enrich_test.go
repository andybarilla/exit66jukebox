package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/enrich"
	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

func TestEnrichEndpoint503WithoutRunner(t *testing.T) {
	srv := newTestServer(t)
	for _, method := range []string{http.MethodPost, http.MethodGet} {
		req := httptest.NewRequest(method, "/api/enrich", nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s /api/enrich without runner: got %d, want 503", method, rec.Code)
		}
	}
}

func TestEnrichStartReturnsStatusAndFlipsRunning(t *testing.T) {
	srv := newTestServer(t)
	// A runner over an empty DB: Start flips running and the pass finishes with
	// nothing to do. mb/ca are never called (no targets), so nil is safe.
	srv.SetEnrichRunner(enrich.NewRunner(srv.db, nil, nil, t.TempDir()))

	req := httptest.NewRequest(http.MethodPost, "/api/enrich", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/enrich: got %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"running":true`) {
		t.Fatalf("expected running:true in body, got %s", rec.Body.String())
	}

	// GET reports a status snapshot as JSON.
	rec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/enrich", nil))
	if rec2.Code != http.StatusOK {
		t.Fatalf("GET /api/enrich: got %d, want 200", rec2.Code)
	}
	if !strings.Contains(rec2.Body.String(), `"processed"`) {
		t.Fatalf("expected status fields in body, got %s", rec2.Body.String())
	}
}

func TestServeCoverFallsBackToCachedCover(t *testing.T) {
	srv := newTestServer(t)
	// Track file does not exist => no embedded art => fall back to album cover.
	id, _ := store.UpsertTrack(srv.db, model.Track{Path: "/no/such/file.mp3", Title: "X"}, "A", "B")
	tr, _, _ := store.GetTrack(srv.db, id)

	cover := filepath.Join(t.TempDir(), "cover.png")
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0}
	if err := os.WriteFile(cover, png, 0o644); err != nil {
		t.Fatalf("write cover: %v", err)
	}
	if err := store.SetAlbumCover(srv.db, tr.AlbumID, cover); err != nil {
		t.Fatalf("SetAlbumCover: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/tracks/"+strconv.FormatInt(id, 10)+"/cover", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 from cached cover fallback, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "image/") {
		t.Errorf("content-type = %q, want image/*", ct)
	}
	if rec.Body.Len() != len(png) {
		t.Errorf("served %d bytes, want %d", rec.Body.Len(), len(png))
	}
}
