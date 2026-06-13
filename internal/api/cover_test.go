package api

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

func TestTrackCoverMissingArtIs404(t *testing.T) {
	srv := newTestServer(t)
	id, _ := store.UpsertTrack(srv.db, model.Track{Path: "/no/such/file.mp3", Title: "X"}, "A", "", "B")
	req := httptest.NewRequest(http.MethodGet, "/api/tracks/"+strconv.FormatInt(id, 10)+"/cover", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 when no cover, got %d", rec.Code)
	}
}

func TestTrackCoverUnknownIdIs404(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/tracks/9999/cover", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 for unknown track, got %d", rec.Code)
	}
}

func TestTrackCoverServesEmbeddedArt(t *testing.T) {
	srv := newTestServer(t)
	// testdata/art.mp3 carries an embedded MJPEG cover (path relative to package dir).
	id, _ := store.UpsertTrack(srv.db, model.Track{Path: "testdata/art.mp3", Title: "Art"}, "AA", "", "AL")
	req := httptest.NewRequest(http.MethodGet, "/api/tracks/"+strconv.FormatInt(id, 10)+"/cover", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 with embedded art, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "image/") {
		t.Fatalf("expected an image/* content-type, got %q", ct)
	}
	if rec.Body.Len() == 0 {
		t.Fatalf("expected image bytes, got none")
	}
}
