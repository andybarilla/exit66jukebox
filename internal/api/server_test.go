package api

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/andybarilla/exit66jukebox/internal/jukebox"
	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

func newTestServer(t *testing.T) (*Server, *sql.DB) {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	jb := jukebox.New(db, jukebox.Config{HistoryWindow: 5})
	s := NewServer(db, jb, nil)
	s.SetSigningSecret([]byte("test-secret"))
	return s, db
}

func TestArtistsEndpointReturnsJSON(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/artists", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type: want application/json, got %q", ct)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
		t.Fatalf("want empty JSON array, got %q", body)
	}
}

func TestClientRoutesServeUIIndex(t *testing.T) {
	ui := fstest.MapFS{
		"index.html":     &fstest.MapFile{Data: []byte("<main>app shell</main>")},
		"assets/app.js":  &fstest.MapFile{Data: []byte("const loaded = true;\n")},
		"assets/app.css": &fstest.MapFile{Data: []byte("body { color: white; }\n")},
	}
	srv := NewServer(nil, nil, ui)
	handler := srv.Handler()

	for _, path := range []string{"/", "/admin", "/verify/email-token", "/invite/invite-token", "/reset-password/reset-token"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("status: want 200, got %d", rec.Code)
			}
			if body := rec.Body.String(); body != "<main>app shell</main>" {
				t.Fatalf("body: want UI index, got %q", body)
			}
		})
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("asset status: want 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "const loaded = true;\n" {
		t.Fatalf("asset body: want JS asset, got %q", body)
	}
}

func TestRequestRecordsRequesterAndStreamReturnsIt(t *testing.T) {
	srv, _ := newTestServer(t)
	id, _ := store.UpsertTrack(srv.db, model.Track{Path: "/m/a.mp3", Title: "Hello"}, "Band", "", "Album")

	form := url.Values{"kind": {"track"}, "id": {strconv.FormatInt(id, 10)}, "by": {"Mira"}}
	req := httptest.NewRequest(http.MethodPost, "/api/streams/sess/requests",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("request status %d", rec.Code)
	}

	rec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/streams/sess", nil))
	if !strings.Contains(rec2.Body.String(), `"requested_by":"Mira"`) {
		t.Fatalf("stream body missing requester: %s", rec2.Body.String())
	}
	if !strings.Contains(rec2.Body.String(), `"listeners":`) {
		t.Fatalf("stream body missing listeners: %s", rec2.Body.String())
	}
}

func TestGetStreamNowPlayingWhenPlaying(t *testing.T) {
	srv, _ := newTestServer(t)
	id, _ := store.UpsertTrack(srv.db, model.Track{Path: "/m/a.mp3", Title: "Hello"}, "Band", "", "Album")
	np := NewNowPlaying()
	tr, _, _ := store.GetTrack(srv.db, id)
	np.Set(tr)
	srv.nowPlaying["house"] = np

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/streams/house", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"now_playing":{`) {
		t.Fatalf("expected now_playing object, got %s", body)
	}
	if !strings.Contains(body, `"offset_seconds":`) {
		t.Fatalf("expected offset_seconds, got %s", body)
	}
	if !strings.Contains(body, `"title":"Hello"`) {
		t.Fatalf("expected track title, got %s", body)
	}
}

func TestGetStreamNowPlayingNullWhenIdle(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.nowPlaying["house"] = NewNowPlaying() // tracker present but idle

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/streams/house", nil))
	if !strings.Contains(rec.Body.String(), `"now_playing":null`) {
		t.Fatalf("expected now_playing:null when idle, got %s", rec.Body.String())
	}
}

func TestGetStreamNowPlayingNullForUntrackedStream(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/streams/me", nil))
	if !strings.Contains(rec.Body.String(), `"now_playing":null`) {
		t.Fatalf("expected now_playing:null for untracked stream, got %s", rec.Body.String())
	}
}

func TestRequestThenNextRoundTrip(t *testing.T) {
	srv, _ := newTestServer(t)
	id, _ := store.UpsertTrack(srv.db, model.Track{Path: "/m/a.mp3", Title: "Hello"}, "Band", "", "Album")

	form := url.Values{"kind": {"track"}, "id": {strconv.FormatInt(id, 10)}}
	req := httptest.NewRequest(http.MethodPost, "/api/streams/sess/requests",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("request status: %d", rec.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/streams/sess/next", nil)
	rec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("next status: %d", rec2.Code)
	}
	if !strings.Contains(rec2.Body.String(), "\"ok\":true") {
		t.Fatalf("expected ok:true, got %s", rec2.Body.String())
	}
}

func TestGetNextDoesNotAdvanceQueue(t *testing.T) {
	srv, _ := newTestServer(t)
	id, _ := store.UpsertTrack(srv.db, model.Track{Path: "/m/a.mp3", Title: "Hello"}, "Band", "", "Album")

	form := url.Values{"kind": {"track"}, "id": {strconv.FormatInt(id, 10)}}
	req := httptest.NewRequest(http.MethodPost, "/api/streams/sess/requests",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("request status: %d", rec.Code)
	}

	getNext := httptest.NewRequest(http.MethodGet, "/api/streams/sess/next", nil)
	getNextRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getNextRec, getNext)
	if getNextRec.Code == http.StatusOK {
		t.Fatalf("GET next should not advance queue, got status %d and body %s", getNextRec.Code, getNextRec.Body.String())
	}

	postNext := httptest.NewRequest(http.MethodPost, "/api/streams/sess/next", nil)
	postNextRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(postNextRec, postNext)
	if postNextRec.Code != http.StatusOK {
		t.Fatalf("POST next status: %d", postNextRec.Code)
	}
	if !strings.Contains(postNextRec.Body.String(), "\"ok\":true") {
		t.Fatalf("expected POST next to advance queued track, got %s", postNextRec.Body.String())
	}
}

func TestRawHandlerDoesNotApplyBrowserAuth(t *testing.T) {
	srv, db := newTestServer(t)
	if err := store.SetSecurityMode(db, store.SecurityModeFullLogin); err != nil {
		t.Fatalf("SetSecurityMode: %v", err)
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("raw Handler status = %d, want 200", rec.Code)
	}
}

func TestPublicHandlerAppliesBrowserAuthAroundRawHandler(t *testing.T) {
	srv, db := newTestServer(t)
	if err := store.SetSecurityMode(db, store.SecurityModeFullLogin); err != nil {
		t.Fatalf("SetSecurityMode: %v", err)
	}

	rec := httptest.NewRecorder()
	srv.RequireAuthMiddleware(srv.Handler()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tracks", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("public handler status = %d, want 401", rec.Code)
	}
}
