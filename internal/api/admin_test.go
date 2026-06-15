package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

// queueTrack requests trackID into a stream so the privileged mutations
// (next/remove/clear) have something to act on.
func queueTrack(t *testing.T, srv *Server, stream string, id int64) {
	t.Helper()
	form := url.Values{"kind": {"track"}, "id": {strconv.FormatInt(id, 10)}}
	req := httptest.NewRequest(http.MethodPost, "/api/streams/"+stream+"/requests",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("seed request status %d", rec.Code)
	}
}

func do(srv *Server, method, target, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

// login posts the password and returns the issued token.
func login(t *testing.T, srv *Server, password string) string {
	t.Helper()
	body := strings.NewReader(`{"password":` + strconv.Quote(password) + `}`)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/login", body)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status %d, body %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("login decode: %v", err)
	}
	if out.Token == "" {
		t.Fatal("login returned empty token")
	}
	return out.Token
}

// Gate disabled (no admin password) → privileged house endpoints stay open.
func TestGateOpenWhenNoPassword(t *testing.T) {
	srv := newTestServer(t)
	id, _ := store.UpsertTrack(srv.db, model.Track{Path: "/m/a.mp3", Title: "A"}, "Band", "", "Album")
	queueTrack(t, srv, "house", id)

	if rec := do(srv, http.MethodGet, "/api/streams/house/next", ""); rec.Code == http.StatusForbidden {
		t.Fatalf("house/next should be open with no password, got 403")
	}
	if rec := do(srv, http.MethodPost, "/api/sonos/cast", ""); rec.Code == http.StatusForbidden {
		t.Fatalf("sonos/cast should be open with no password, got 403")
	}
}

// Gate enabled → house privileged endpoints 403 without a token, pass with one.
func TestGateClosedRequiresToken(t *testing.T) {
	srv := newTestServer(t)
	srv.SetAdminPassword("hunter2")
	id, _ := store.UpsertTrack(srv.db, model.Track{Path: "/m/a.mp3", Title: "A"}, "Band", "", "Album")

	// Without a token: 403.
	for _, ep := range []struct{ method, target string }{
		{http.MethodGet, "/api/streams/house/next"},
		{http.MethodPost, "/api/streams/house/shuffle"},
		{http.MethodDelete, "/api/streams/house/requests"},
		{http.MethodDelete, "/api/streams/house/requests/1"},
	} {
		if rec := do(srv, ep.method, ep.target, ""); rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s without token: want 403, got %d", ep.method, ep.target, rec.Code)
		}
	}
	// Sonos gate fires before the handler too.
	if rec := do(srv, http.MethodPost, "/api/sonos/cast", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("sonos/cast without token: want 403, got %d", rec.Code)
	}

	// With a valid token: the gate passes (handler runs → 200, not 403).
	token := login(t, srv, "hunter2")
	queueTrack(t, srv, "house", id)
	if rec := do(srv, http.MethodGet, "/api/streams/house/next", token); rec.Code != http.StatusOK {
		t.Fatalf("house/next with token: want 200, got %d", rec.Code)
	}
	// Sonos cast with a token clears the gate; it then 400s on the unknown device
	// (proving the gate let it through rather than 403ing).
	if rec := do(srv, http.MethodPost, "/api/sonos/cast", token); rec.Code == http.StatusForbidden {
		t.Fatalf("sonos/cast with token should pass the gate, got 403")
	}
}

// Personal ("me") stream stays open even with a password set: each guest must be
// able to drive their own queue. Regression guard for the house-only scoping.
func TestPersonalStreamStaysOpen(t *testing.T) {
	srv := newTestServer(t)
	srv.SetAdminPassword("hunter2")
	id, _ := store.UpsertTrack(srv.db, model.Track{Path: "/m/a.mp3", Title: "A"}, "Band", "", "Album")
	queueTrack(t, srv, "me", id)

	for _, ep := range []struct{ method, target string }{
		{http.MethodGet, "/api/streams/me/next"},
		{http.MethodPost, "/api/streams/me/shuffle"},
		{http.MethodDelete, "/api/streams/me/requests"},
		{http.MethodDelete, "/api/streams/me/requests/1"},
	} {
		if rec := do(srv, ep.method, ep.target, ""); rec.Code == http.StatusForbidden {
			t.Fatalf("%s %s on personal stream should stay open, got 403", ep.method, ep.target)
		}
	}
}

// Requesting a song never needs a token, password set or not.
func TestRequestStaysOpenWithPassword(t *testing.T) {
	srv := newTestServer(t)
	srv.SetAdminPassword("hunter2")
	id, _ := store.UpsertTrack(srv.db, model.Track{Path: "/m/a.mp3", Title: "A"}, "Band", "", "Album")
	queueTrack(t, srv, "house", id) // 200 or it fatals
}

func TestLoginSuccessAndFailure(t *testing.T) {
	srv := newTestServer(t)
	srv.SetAdminPassword("hunter2")

	// Correct password → token.
	login(t, srv, "hunter2")

	// Wrong password → 401.
	req := httptest.NewRequest(http.MethodPost, "/api/admin/login", strings.NewReader(`{"password":"nope"}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: want 401, got %d", rec.Code)
	}
}

func TestLogoutRevokesToken(t *testing.T) {
	srv := newTestServer(t)
	srv.SetAdminPassword("hunter2")
	id, _ := store.UpsertTrack(srv.db, model.Track{Path: "/m/a.mp3", Title: "A"}, "Band", "", "Album")
	token := login(t, srv, "hunter2")

	// Logout, then the token must no longer pass the gate.
	logout := httptest.NewRequest(http.MethodPost, "/api/admin/logout", nil)
	logout.Header.Set("Authorization", "Bearer "+token)
	srv.Handler().ServeHTTP(httptest.NewRecorder(), logout)

	queueTrack(t, srv, "house", id)
	if rec := do(srv, http.MethodGet, "/api/streams/house/next", token); rec.Code != http.StatusForbidden {
		t.Fatalf("revoked token should 403, got %d", rec.Code)
	}
}

func TestConfigReportsAdminState(t *testing.T) {
	// No password: admin_required false, is_admin true (gate open → everyone admin).
	open := newTestServer(t)
	rec := do(open, http.MethodGet, "/api/config", "")
	body := rec.Body.String()
	if !strings.Contains(body, `"admin_required":false`) {
		t.Fatalf("open: want admin_required:false, got %s", body)
	}
	if !strings.Contains(body, `"is_admin":true`) {
		t.Fatalf("open: want is_admin:true, got %s", body)
	}

	// Password set, no token: admin_required true, is_admin false.
	srv := newTestServer(t)
	srv.SetAdminPassword("hunter2")
	rec = do(srv, http.MethodGet, "/api/config", "")
	body = rec.Body.String()
	if !strings.Contains(body, `"admin_required":true`) {
		t.Fatalf("closed: want admin_required:true, got %s", body)
	}
	if !strings.Contains(body, `"is_admin":false`) {
		t.Fatalf("closed no-token: want is_admin:false, got %s", body)
	}

	// With a valid token: is_admin true.
	token := login(t, srv, "hunter2")
	rec = do(srv, http.MethodGet, "/api/config", token)
	if !strings.Contains(rec.Body.String(), `"is_admin":true`) {
		t.Fatalf("closed with token: want is_admin:true, got %s", rec.Body.String())
	}
}
