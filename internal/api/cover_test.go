package api

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

func TestTrackCoverMissingArtIs404(t *testing.T) {
	srv := newTestServer(t)
	id, _ := store.UpsertTrack(srv.db, model.Track{Path: "/no/such/file.mp3", Title: "X"}, "A", "B")
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
