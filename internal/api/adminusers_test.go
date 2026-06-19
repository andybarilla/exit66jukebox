package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/auth"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

// adminReq builds a request carrying a valid admin session cookie.
func adminReq(t *testing.T, db *sql.DB, method, path, body string) *http.Request {
	t.Helper()
	uid, _ := store.CreateUser(db, "admin@b.com", "Ad", "h", true)
	raw, _ := auth.GenerateToken()
	store.CreateSession(db, auth.HashToken(raw), uid, 4_000_000_000)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: raw})
	return req
}

func TestAdminCreatePasswordResetReturnsLink(t *testing.T) {
	s, db := newTestServer(t)
	h, _ := auth.HashPassword("oldpassword")
	userID, _ := store.CreateUser(db, "reset@example.com", "Reset", h, false)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, adminReq(t, db, "POST", "/api/admin/users/"+strconv.FormatInt(userID, 10)+"/password-reset", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("password reset link: want 200, got %d (%s)", rec.Code, rec.Body)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["email"] != "reset@example.com" || !strings.Contains(body["link"], "/reset-password/") {
		t.Fatalf("reset link response mismatch: %#v", body)
	}
}

func TestAdminSettingsToggle(t *testing.T) {
	s, db := newTestServer(t)
	req := adminReq(t, db, "POST", "/api/admin/settings", `{"signup_enabled":true,"guest_access_enabled":true}`)
	rec := httptest.NewRecorder()
	s.requireAdmin(s.setAdminSettings)(rec, req)
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body)
	}
	if !store.SignupEnabled(db) || !store.GuestAccessEnabled(db) {
		t.Fatal("toggles not applied")
	}
}

func TestAdminCreateInviteReturnsLink(t *testing.T) {
	s, db := newTestServer(t)
	req := adminReq(t, db, "POST", "/api/admin/invites", `{"email":"x@y.com","is_admin":false}`)
	rec := httptest.NewRecorder()
	s.requireAdmin(s.createInvite)(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "/invite/") {
		t.Fatalf("invite link missing: %d %s", rec.Code, rec.Body)
	}
	if invs, _ := store.ListInvites(db); len(invs) != 1 {
		t.Fatalf("invite not stored")
	}
}

func TestAdminNonAdminForbidden(t *testing.T) {
	s, db := newTestServer(t)
	uid, _ := store.CreateUser(db, "u@b.com", "U", "h", false) // not admin
	raw, _ := auth.GenerateToken()
	store.CreateSession(db, auth.HashToken(raw), uid, 4_000_000_000)
	req := httptest.NewRequest("GET", "/api/admin/users", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: raw})
	rec := httptest.NewRecorder()
	s.requireAdmin(s.listUsers)(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rec.Code)
	}
}

// nonAdminCookie seeds a non-admin account + session and returns its cookie.
func nonAdminCookie(t *testing.T, db *sql.DB) *http.Cookie {
	t.Helper()
	uid, _ := store.CreateUser(db, "user@b.com", "U", "h", false)
	raw, _ := auth.GenerateToken()
	store.CreateSession(db, auth.HashToken(raw), uid, 4_000_000_000)
	return &http.Cookie{Name: sessionCookie, Value: raw}
}

func TestCreateInviteRequiresEmail(t *testing.T) {
	s, db := newTestServer(t)
	req := adminReq(t, db, "POST", "/api/admin/invites", `{"email":"","is_admin":false}`)
	rec := httptest.NewRecorder()
	s.requireAdmin(s.createInvite)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("blank-email invite: want 400, got %d (%s)", rec.Code, rec.Body)
	}
}

func TestHouseStationRequiresAdmin(t *testing.T) {
	s, db := newTestServer(t)
	cookie := nonAdminCookie(t, db)
	for _, method := range []string{"POST", "DELETE"} {
		req := httptest.NewRequest(method, "/api/streams/house/station", strings.NewReader(`{"genre":"rock"}`))
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s house station as non-admin: want 403, got %d", method, rec.Code)
		}
	}
}

func TestEnrichPostRequiresAdmin(t *testing.T) {
	s, db := newTestServer(t)
	req := httptest.NewRequest("POST", "/api/enrich", nil)
	req.AddCookie(nonAdminCookie(t, db))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin POST /api/enrich: want 403, got %d", rec.Code)
	}
}
