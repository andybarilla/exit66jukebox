package fed

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestWithCapsRouteAdvertisesCapabilities verifies the caps handler served on
// every federation session returns this instance's capabilities as JSON.
func TestWithCapsRouteAdvertisesCapabilities(t *testing.T) {
	caps := Capabilities{DirectWebRTC: true, STUNServers: []string{"stun:example:3478"}}
	h := WithCapsRoute(caps, http.NotFoundHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, capsRoute, nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	var got Capabilities
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.DirectWebRTC || len(got.STUNServers) != 1 || got.STUNServers[0] != "stun:example:3478" {
		t.Fatalf("caps = %#v", got)
	}
}

// TestWithCapsRouteDelegatesOtherPaths verifies the caps wrapper falls through
// to the wrapped handler for non-caps paths.
func TestWithCapsRouteDelegatesOtherPaths(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	})
	h := WithCapsRoute(Capabilities{}, next)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/anything-else", nil)
	h.ServeHTTP(rec, req)

	if !called {
		t.Fatal("wrapped handler not called")
	}
	if rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d want teapot", rec.Code)
	}
}

// TestFetchCapsReturnsRemoteCapabilities verifies the offerer side: fetching a
// peer's caps over its session client returns the advertised capabilities.
func TestFetchCapsReturnsRemoteCapabilities(t *testing.T) {
	want := Capabilities{DirectWebRTC: true}
	srv := httptest.NewServer(WithCapsRoute(want, http.NotFoundHandler()))
	defer srv.Close()

	got := fetchCaps(srv.Client(), srv.URL)
	if !got.DirectWebRTC {
		t.Fatalf("fetchCaps = %#v want DirectWebRTC=true", got)
	}
}

// TestFetchCapsFailsSafeOnTransportError verifies a fetch failure yields a zero
// Capabilities (relay-only), so the resolver falls back safely.
func TestFetchCapsFailsSafeOnTransportError(t *testing.T) {
	got := fetchCaps(http.DefaultClient, "http://127.0.0.1:1")
	if got.DirectWebRTC {
		t.Fatalf("expected zero caps on error, got %#v", got)
	}
	got = fetchCaps(nil, "")
	if got.DirectWebRTC {
		t.Fatalf("expected zero caps for nil client, got %#v", got)
	}
}

// TestLearnCapsPopulatesPeer verifies the manager helper records fetched caps on
// the Peer struct so the resolver can pick a transport.
func TestLearnCapsPopulatesPeer(t *testing.T) {
	srv := httptest.NewServer(WithCapsRoute(Capabilities{DirectWebRTC: true}, http.NotFoundHandler()))
	defer srv.Close()

	m := &Manager{}
	p := &Peer{ID: "bob", Client: srv.Client(), BaseURL: srv.URL}
	m.learnCaps(p)
	if !p.Caps.DirectWebRTC {
		t.Fatalf("learnCaps did not populate Peer.Caps: %#v", p.Caps)
	}

	// Best-effort: a nil peer is a no-op (not a panic).
	m.learnCaps(nil)
}
