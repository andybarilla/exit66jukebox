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

func TestStreamURL(t *testing.T) {
	if got := streamURL("192.168.1.10", ":8066"); got != "http://192.168.1.10:8066/stream/house.mp3" {
		t.Fatalf("streamURL = %q", got)
	}
	// Empty/invalid listen addr falls back to the default port.
	if got := streamURL("192.168.1.10", ""); got != "http://192.168.1.10:8066/stream/house.mp3" {
		t.Fatalf("streamURL fallback = %q", got)
	}
}

func TestPrivateIPv4(t *testing.T) {
	for _, ip := range []string{"10.0.0.5", "172.16.3.4", "192.168.1.50"} {
		if !privateIPv4(ip) {
			t.Errorf("privateIPv4(%q) = false, want true", ip)
		}
	}
	// loopback, link-local/metadata, public, multicast, non-IP, IPv6 all rejected.
	for _, ip := range []string{"", "nope", "127.0.0.1", "169.254.169.254", "8.8.8.8", "224.0.0.1", "::1", "fe80::1"} {
		if privateIPv4(ip) {
			t.Errorf("privateIPv4(%q) = true, want false", ip)
		}
	}
}

func TestSonosCastRejectsUndiscoveredIP(t *testing.T) {
	srv := newTestServer(t)
	// A valid private IP that was never discovered must be rejected (SSRF guard:
	// the server only casts to IPs that announced themselves via discovery).
	req := httptest.NewRequest(http.MethodPost, "/api/sonos/cast", strings.NewReader("ip=192.168.1.99"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for undiscovered ip, got %d", rec.Code)
	}
}
