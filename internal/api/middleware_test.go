package api

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/auth"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
}

func TestMiddlewareBlocksAnonAPI(t *testing.T) {
	s, _ := newTestServer(t)
	h := s.RequireAuthMiddleware(okHandler())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/tracks", nil)
	req.RemoteAddr = "203.0.113.9:1" // public, not loopback
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anon /api/tracks: want 401, got %d", rec.Code)
	}
}

func TestMiddlewareAllowsOpenPaths(t *testing.T) {
	s, _ := newTestServer(t)
	h := s.RequireAuthMiddleware(okHandler())
	for _, p := range []string{"/api/config", "/api/auth/login", "/api/auth/me"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", p, nil)
		req.RemoteAddr = "203.0.113.9:1"
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("open path %s: want 200, got %d", p, rec.Code)
		}
	}
}

func TestMiddlewareAllowsStaticAndStream(t *testing.T) {
	s, _ := newTestServer(t)
	h := s.RequireAuthMiddleware(okHandler())
	for _, p := range []string{"/", "/assets/app.js", "/stream/house.mp3"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", p, nil)
		req.RemoteAddr = "203.0.113.9:1"
		h.ServeHTTP(rec, req)
		if rec.Code != 200 { // non-/api paths pass the middleware (their own handlers/guards apply downstream)
			t.Fatalf("path %s: want pass-through 200, got %d", p, rec.Code)
		}
	}
}

func TestMiddlewareAllowsSessionAndSignedURL(t *testing.T) {
	s, db := newTestServer(t)
	uid, _ := store.CreateUser(db, "a@b.com", "A", "h", false)
	raw, _ := auth.GenerateToken()
	store.CreateSession(db, auth.HashToken(raw), uid, 4_000_000_000)
	h := s.RequireAuthMiddleware(okHandler())

	// session cookie
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/tracks", nil)
	req.RemoteAddr = "203.0.113.9:1"
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: raw})
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("session: want 200, got %d", rec.Code)
	}

	// signed URL (ffmpeg house source), no cookie, valid for this exact path
	tok := auth.SignPath([]byte("test-secret"), "/api/tracks/5/audio", 4_000_000_000)
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "/api/tracks/5/audio?sig="+tok, nil)
	req2.RemoteAddr = "203.0.113.9:1"
	h.ServeHTTP(rec2, req2)
	if rec2.Code != 200 {
		t.Fatalf("signed url: want 200, got %d", rec2.Code)
	}
}

func TestMiddlewareRejectsPasswordlessProfileSessionInFullLogin(t *testing.T) {
	s, db := newTestServer(t)
	if err := store.SetSecurityMode(db, store.SecurityModeFullLogin); err != nil {
		t.Fatalf("SetSecurityMode: %v", err)
	}
	profileID, err := store.CreatePasswordlessProfile(db, "Casey")
	if err != nil {
		t.Fatalf("CreatePasswordlessProfile: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tracks", nil)
	req.AddCookie(createSessionCookie(t, db, profileID))
	s.RequireAuthMiddleware(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("passwordless profile in full_login: want 401, got %d", rec.Code)
	}
}

func TestMiddlewareRejectsPasswordAccountSessionInHouseholdProfiles(t *testing.T) {
	s, db := newTestServer(t)
	if err := store.SetSecurityMode(db, store.SecurityModeHouseholdProfiles); err != nil {
		t.Fatalf("SetSecurityMode: %v", err)
	}
	userID, err := store.CreateUser(db, "account@example.com", "Account", "h", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tracks", nil)
	req.AddCookie(createSessionCookie(t, db, userID))
	s.RequireAuthMiddleware(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("password account in household_profiles: want 401, got %d", rec.Code)
	}
}

func TestMiddlewareAllowsHouseholdProfileEntryEndpoints(t *testing.T) {
	s, db := newTestServer(t)
	if err := store.SetSecurityMode(db, store.SecurityModeHouseholdProfiles); err != nil {
		t.Fatalf("SetSecurityMode: %v", err)
	}
	profileID, err := store.CreatePasswordlessProfile(db, "Casey")
	if err != nil {
		t.Fatalf("CreatePasswordlessProfile: %v", err)
	}
	h := s.RequireAuthMiddleware(s.Handler())

	list := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/api/auth/profiles", nil)
	listReq.RemoteAddr = "203.0.113.9:1"
	h.ServeHTTP(list, listReq)
	if list.Code != http.StatusOK {
		t.Fatalf("profile list: want 200, got %d (%s)", list.Code, list.Body)
	}

	selectRec := httptest.NewRecorder()
	selectReq := httptest.NewRequest(http.MethodPost, "/api/auth/profiles/select", strings.NewReader(`{"id":`+strconv.FormatInt(profileID, 10)+`}`))
	selectReq.RemoteAddr = "203.0.113.9:1"
	h.ServeHTTP(selectRec, selectReq)
	if selectRec.Code != http.StatusOK {
		t.Fatalf("profile select: want 200, got %d (%s)", selectRec.Code, selectRec.Body)
	}
}

func TestMiddlewareAllowsPasswordAdminAPIInHouseholdProfiles(t *testing.T) {
	s, db := newTestServer(t)
	if err := store.SetSecurityMode(db, store.SecurityModeHouseholdProfiles); err != nil {
		t.Fatalf("SetSecurityMode: %v", err)
	}
	adminID, err := store.CreateUser(db, "admin@example.com", "Admin", "h", true)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req.AddCookie(createSessionCookie(t, db, adminID))
	s.RequireAuthMiddleware(s.Handler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("admin API in household_profiles: want 200, got %d (%s)", rec.Code, rec.Body)
	}
}

func TestMiddlewareAllowsPasswordAdminHouseControlsInHouseholdProfiles(t *testing.T) {
	s, db := newTestServer(t)
	if err := store.SetSecurityMode(db, store.SecurityModeHouseholdProfiles); err != nil {
		t.Fatalf("SetSecurityMode: %v", err)
	}
	adminID, err := store.CreateUser(db, "admin@example.com", "Admin", "h", true)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/streams/house/shuffle", strings.NewReader(`{"shuffle":true}`))
	req.AddCookie(createSessionCookie(t, db, adminID))
	s.RequireAuthMiddleware(s.Handler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("house shuffle as password admin in household_profiles: want 200, got %d (%s)", rec.Code, rec.Body)
	}
}

// TestMiddlewareLoopbackNotTrusted locks in the fix: a loopback peer with no
// cookie/guest/signed-token is NOT trusted, so a same-host reverse proxy can't
// be used to bypass auth.
func TestMiddlewareLoopbackNotTrusted(t *testing.T) {
	s, _ := newTestServer(t)
	h := s.RequireAuthMiddleware(okHandler())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/tracks", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("loopback without auth: want 401, got %d", rec.Code)
	}
}
