package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSonosCastRequiresIP(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/sonos/cast", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 when ip missing, got %d", rec.Code)
	}
}

func TestHouseStreamURLFromHost(t *testing.T) {
	got := houseStreamURL("192.168.1.10:8066")
	want := "http://192.168.1.10:8066/stream/house.mp3"
	if got != want {
		t.Fatalf("houseStreamURL = %q, want %q", got, want)
	}
}
