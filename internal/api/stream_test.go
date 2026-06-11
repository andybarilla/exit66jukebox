package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStreamAudioUnknownStreamIs404(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/stream/nope.mp3", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 for unknown stream, got %d", rec.Code)
	}
}

func TestEventsUnknownStreamIs404(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/streams/nope/events", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 for unknown stream events, got %d", rec.Code)
	}
}
