package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/recommend"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

func TestDiscoverRediscoverEndpoint(t *testing.T) {
	srv := newTestServer(t)
	store.UpsertTrack(srv.db, model.Track{Path: "/m/a.mp3", Title: "A", Genre: "Rock"}, "B", "X")

	req := httptest.NewRequest(http.MethodGet, "/api/discover/rediscover?genre=Rock", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d", rec.Code)
	}
	var got []model.Track
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	if len(got) != 1 || got[0].Title != "A" {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestDiscoverGenresEndpoint(t *testing.T) {
	srv := newTestServer(t)
	store.UpsertTrack(srv.db, model.Track{Path: "/m/a.mp3", Title: "A", Genre: "Rock"}, "B", "X")

	req := httptest.NewRequest(http.MethodGet, "/api/discover/genres", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Rock") {
		t.Fatalf("unexpected genres response: %d %s", rec.Code, rec.Body.String())
	}
}

func TestStationStartGetStopEndpoints(t *testing.T) {
	srv := newTestServer(t)
	for _, p := range []string{"/m/1.mp3", "/m/2.mp3", "/m/3.mp3"} {
		store.UpsertTrack(srv.db, model.Track{Path: p, Title: p, Genre: "Rock"}, "B", "X")
	}

	// Start
	body := strings.NewReader(`{"genre":"Rock"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/streams/s/station", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("start status: %d body=%s", rec.Code, rec.Body.String())
	}

	// Get
	req2 := httptest.NewRequest(http.MethodGet, "/api/streams/s/station", nil)
	rec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK || !strings.Contains(rec2.Body.String(), "Rock") {
		t.Fatalf("get station: %d %s", rec2.Code, rec2.Body.String())
	}

	// Queue should have been filled immediately.
	n, _ := store.QueueLen(srv.db, "s")
	if n == 0 {
		t.Fatalf("expected immediate fill, queue empty")
	}

	// Stop
	req3 := httptest.NewRequest(http.MethodDelete, "/api/streams/s/station", nil)
	rec3 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("stop status: %d", rec3.Code)
	}
	if _, ok := store.GetStation(srv.db, "s"); ok {
		t.Fatalf("expected station removed after stop")
	}
}

func TestDiscoverRecommendedNoRunnerReturnsEmptyArray(t *testing.T) {
	srv := newTestServer(t) // no recommend runner wired
	req := httptest.NewRequest(http.MethodGet, "/api/discover/recommended", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (not 503)", rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("body = %q, want []", rec.Body.String())
	}
}

func TestDiscoverRecommendedServesRunnerCache(t *testing.T) {
	srv := newTestServer(t)
	// A runner with no configured sources serves an empty (non-null) array.
	srv.SetRecommendRunner(recommend.NewRunner(srv.db, nil, nil))
	req := httptest.NewRequest(http.MethodGet, "/api/discover/recommended", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []model.EnrichedTrack
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	if len(got) != 0 {
		t.Fatalf("got %d tracks, want 0", len(got))
	}
}
