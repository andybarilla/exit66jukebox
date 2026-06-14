package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
