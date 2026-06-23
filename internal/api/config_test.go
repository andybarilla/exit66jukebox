package api

import (
	"encoding/json"
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
		srv, _ := newTestServer(t)
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

func TestConfigExposesSecurityModeEntryFlow(t *testing.T) {
	cases := []struct {
		mode            store.SecurityMode
		guestAccess     bool
		requiresProfile bool
		requiresLogin   bool
	}{
		{store.SecurityModeOpen, true, false, false},
		{store.SecurityModeOpenAdminLocked, true, false, false},
		{store.SecurityModeHouseholdProfiles, false, true, false},
		{store.SecurityModeFullLogin, false, false, true},
	}

	for _, tc := range cases {
		t.Run(string(tc.mode), func(t *testing.T) {
			db := setupAPITestDB(t)
			if err := store.SetSecurityMode(db, tc.mode); err != nil {
				t.Fatalf("SetSecurityMode: %v", err)
			}
			srv := NewServer(db, nil, nil)
			rec := httptest.NewRecorder()

			srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d", rec.Code)
			}
			var got map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("json: %v", err)
			}
			if got["security_mode"] != string(tc.mode) {
				t.Fatalf("security_mode = %v, want %s", got["security_mode"], tc.mode)
			}
			if got["guest_access"] != tc.guestAccess {
				t.Fatalf("guest_access = %v, want %v", got["guest_access"], tc.guestAccess)
			}
			if got["requires_profile"] != tc.requiresProfile {
				t.Fatalf("requires_profile = %v, want %v", got["requires_profile"], tc.requiresProfile)
			}
			if got["requires_login"] != tc.requiresLogin {
				t.Fatalf("requires_login = %v, want %v", got["requires_login"], tc.requiresLogin)
			}
		})
	}
}
