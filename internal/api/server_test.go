package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/jukebox"
	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	jb := jukebox.New(db, jukebox.Config{HistoryWindow: 5})
	return NewServer(db, jb, nil)
}

func TestArtistsEndpointReturnsJSON(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/artists", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type: want application/json, got %q", ct)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
		t.Fatalf("want empty JSON array, got %q", body)
	}
}

func TestRequestThenNextRoundTrip(t *testing.T) {
	srv := newTestServer(t)
	id, _ := store.UpsertTrack(srv.db, model.Track{Path: "/m/a.mp3", Title: "Hello"}, "Band", "Album")

	form := url.Values{"kind": {"track"}, "id": {strconv.FormatInt(id, 10)}}
	req := httptest.NewRequest(http.MethodPost, "/api/streams/sess/requests",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("request status: %d", rec.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/streams/sess/next", nil)
	rec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("next status: %d", rec2.Code)
	}
	if !strings.Contains(rec2.Body.String(), "\"ok\":true") {
		t.Fatalf("expected ok:true, got %s", rec2.Body.String())
	}
}
