package fed

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

// spyHandler records the requests that reached the application handler.
type spyHandler struct{ hits []string }

func (s *spyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.hits = append(s.hits, r.Method+" "+r.URL.Path)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("app"))
}

// refusedRoutes are application routes a peer must not reach over a federation
// session. Every one of them relies on the browser auth middleware for its
// protection, which never runs on a peer session.
var refusedRoutes = []struct{ method, path string }{
	{http.MethodGet, "/api/streams/house"},
	{http.MethodPost, "/api/streams/house/requests"},
	{http.MethodDelete, "/api/streams/house/requests"},
	{http.MethodDelete, "/api/streams/house/requests/1"},
	{http.MethodPost, "/api/streams/house/next"},
	{http.MethodPost, "/api/streams/house/shuffle"},
	{http.MethodPost, "/api/streams/house/station"},
	{http.MethodGet, "/api/admin/settings"},
	{http.MethodPost, "/api/admin/settings"},
	{http.MethodGet, "/api/admin/federation/peers"},
	{http.MethodGet, "/api/tracks"},
	{http.MethodGet, "/api/auth/me"},
	{http.MethodGet, "/"},
}

func TestAppRoutesServesTrackAudio(t *testing.T) {
	spy := &spyHandler{}
	rec := httptest.NewRecorder()

	AppRoutes(spy).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tracks/5/audio", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("track audio status = %d, want 200", rec.Code)
	}
	if len(spy.hits) != 1 || spy.hits[0] != "GET /api/tracks/5/audio" {
		t.Fatalf("app hits = %v, want one GET /api/tracks/5/audio", spy.hits)
	}
}

func TestAppRoutesRefusesEverythingElse(t *testing.T) {
	for _, r := range refusedRoutes {
		t.Run(r.method+" "+r.path, func(t *testing.T) {
			spy := &spyHandler{}
			rec := httptest.NewRecorder()

			AppRoutes(spy).ServeHTTP(rec, httptest.NewRequest(r.method, r.path, nil))

			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", rec.Code)
			}
			if len(spy.hits) != 0 {
				t.Fatalf("request reached the application handler: %v", spy.hits)
			}
		})
	}
}

// A path that matches the allowlisted pattern under another method is rejected
// by the mux itself, which answers 405 rather than 404. Either way it never
// reaches the application.
func TestAppRoutesRefusesTrackAudioUnderAnotherMethod(t *testing.T) {
	spy := &spyHandler{}
	rec := httptest.NewRecorder()

	AppRoutes(spy).ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/tracks/5/audio", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	if len(spy.hits) != 0 {
		t.Fatalf("request reached the application handler: %v", spy.hits)
	}
}

func TestAppRoutesWithNilAppRefusesEverything(t *testing.T) {
	rec := httptest.NewRecorder()

	AppRoutes(nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tracks/5/audio", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestPeerRoutesServesTrackAudioAndRefusesEverythingElse(t *testing.T) {
	spy := &spyHandler{}
	rec := httptest.NewRecorder()

	PeerRoutes(nil, spy).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tracks/5/audio", nil))

	if rec.Code != http.StatusOK || len(spy.hits) != 1 {
		t.Fatalf("track audio over peer routes: status %d, hits %v", rec.Code, spy.hits)
	}
	for _, r := range refusedRoutes {
		spy := &spyHandler{}
		rec := httptest.NewRecorder()

		PeerRoutes(nil, spy).ServeHTTP(rec, httptest.NewRequest(r.method, r.path, nil))

		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s %s: status = %d, want 404", r.method, r.path, rec.Code)
		}
		if len(spy.hits) != 0 {
			t.Fatalf("%s %s reached the application handler", r.method, r.path)
		}
	}
}

// TestWebRTCAudioReachesAppThroughAllowlist covers the WebRTC tier's answerer
// side, which serves data-channel audio requests against the composed
// PeerHandler rather than the application handler directly (Manager.Start).
// serveOneOverConn builds a host-less request, so this pins that the allowlist
// mux still matches it.
func TestWebRTCAudioReachesAppThroughAllowlist(t *testing.T) {
	spy := &spyHandler{}
	peerHandler := WithCapsRoute(Capabilities{DirectWebRTC: true}, PeerRoutes(nil, spy))

	conn := &framedConn{}
	if err := writeFrame(&conn.in, audioRequest{TrackID: 42, Range: "bytes=0-9"}); err != nil {
		t.Fatalf("write request frame: %v", err)
	}
	if err := ServeAudioOverConn(conn, peerHandler, ""); err != nil {
		t.Fatalf("serve audio over conn: %v", err)
	}

	var resp audioResponse
	if err := readFrame(&conn.out, &resp); err != nil {
		t.Fatalf("read response frame: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", resp.Status, resp.ErrorMessage)
	}
	if len(spy.hits) != 1 || spy.hits[0] != "GET /api/tracks/42/audio" {
		t.Fatalf("app hits = %v, want one GET /api/tracks/42/audio", spy.hits)
	}
}

// framedConn plays back a pre-written request stream and captures what the
// answerer writes back, standing in for a WebRTC data channel.
type framedConn struct{ in, out bytes.Buffer }

func (c *framedConn) Read(p []byte) (int, error)  { return c.in.Read(p) }
func (c *framedConn) Write(p []byte) (int, error) { return c.out.Write(p) }
