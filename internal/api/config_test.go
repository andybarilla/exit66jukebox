package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

func TestConfigIncludesFedPeers(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	srv := NewServer(db, nil, nil)
	srv.SetFedPeers(func() []string { return []string{"home", "vps"} })

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/api/config", nil))
	body := rec.Body.String()
	if !strings.Contains(body, `"fed_peers"`) || !strings.Contains(body, `"home"`) {
		t.Fatalf("expected fed_peers with home, got %s", body)
	}
}

func TestConfigEndpointReflectsMuteFlag(t *testing.T) {
	for _, tc := range []struct {
		mute bool
		want string
	}{
		{true, `"mute_local_on_cast":true`},
		{false, `"mute_local_on_cast":false`},
	} {
		srv := newTestServer(t)
		srv.SetMuteLocalOnCast(tc.mute)

		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("mute=%v: status %d", tc.mute, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Fatalf("mute=%v: content-type %q", tc.mute, ct)
		}
		if body := rec.Body.String(); !strings.Contains(body, tc.want) {
			t.Fatalf("mute=%v: body %q missing %q", tc.mute, body, tc.want)
		}
	}
}
