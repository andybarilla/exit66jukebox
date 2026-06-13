package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/sonos"
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

func formPost(srv *Server, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func TestManualRejectsNonPrivateIP(t *testing.T) {
	srv := newTestServer(t)
	verified := false
	srv.manualVerify = func(string) (string, bool) { verified = true; return "X", true }
	rec := formPost(srv, "/api/sonos/manual", "ip=8.8.8.8")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for public ip, got %d", rec.Code)
	}
	if verified {
		t.Fatalf("Verify must not run for a non-private ip (SSRF guard)")
	}
}

func TestManualRejectsUnverifiableIP(t *testing.T) {
	srv := newTestServer(t)
	srv.manualVerify = func(string) (string, bool) { return "", false }
	rec := formPost(srv, "/api/sonos/manual", "ip=192.168.1.77")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("want 502 for unverifiable ip, got %d", rec.Code)
	}
}

func TestManualAddsVerifiedIP(t *testing.T) {
	srv := newTestServer(t)
	srv.manualVerify = func(ip string) (string, bool) { return "Kitchen", true }
	rec := formPost(srv, "/api/sonos/manual", "ip=192.168.1.77")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var got map[string]string
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got["name"] != "Kitchen" || got["ip"] != "192.168.1.77" {
		t.Fatalf("response = %v, want Kitchen/192.168.1.77", got)
	}
	// Manual IP is now on the allowlist and survives a rediscovery that doesn't
	// include it.
	if !srv.allowedSonos("192.168.1.77") {
		t.Fatalf("manual ip should be allowed after add")
	}
	srv.rememberDevices([]sonos.Device{{Name: "Den", IP: "192.168.1.5"}})
	if !srv.allowedSonos("192.168.1.77") {
		t.Fatalf("manual ip should survive rediscovery")
	}
	if !srv.allowedSonos("192.168.1.5") {
		t.Fatalf("discovered ip should be allowed")
	}
}

func TestDeviceListMergesManual(t *testing.T) {
	srv := newTestServer(t)
	srv.sonosManual["192.168.1.77"] = "Kitchen"
	list := srv.deviceList([]sonos.Device{{Name: "Den", IP: "192.168.1.5"}})
	names := map[string]string{}
	for _, d := range list {
		names[d.IP] = d.Name
	}
	if names["192.168.1.5"] != "Den" || names["192.168.1.77"] != "Kitchen" {
		t.Fatalf("deviceList = %v, want both discovered and manual", list)
	}
}

func TestVolumeRejectsUndiscoveredIP(t *testing.T) {
	srv := newTestServer(t)
	get := httptest.NewRequest(http.MethodGet, "/api/sonos/volume?ip=192.168.1.99", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, get)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("GET volume: want 400 for undiscovered ip, got %d", rec.Code)
	}
	rec = formPost(srv, "/api/sonos/volume", "ip=192.168.1.99&volume=50")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST volume: want 400 for undiscovered ip, got %d", rec.Code)
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
