package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/scan"
)

func TestScanEndpoint503WithoutProgress(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/scan", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /api/scan without progress: got %d, want 503", rec.Code)
	}
}

func TestScanEndpointReportsSnapshot(t *testing.T) {
	srv := newTestServer(t)
	var p scan.Progress
	p.SetRunning(true)
	srv.SetScanProgress(&p)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/scan", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/scan: got %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type: want application/json, got %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"running":true`) {
		t.Fatalf("expected running:true in body, got %s", body)
	}
	for _, field := range []string{`"added"`, `"updated"`, `"skipped"`, `"failed"`} {
		if !strings.Contains(body, field) {
			t.Fatalf("expected %s in body, got %s", field, body)
		}
	}
}
