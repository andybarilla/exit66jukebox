package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/auth"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

// adminReq builds a request carrying a valid admin session cookie.
func adminReq(t *testing.T, db *sql.DB, method, path, body string) *http.Request {
	t.Helper()
	_, cookie := adminSessionWithEmail(t, db, "admin@b.com")
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.AddCookie(cookie)
	return req
}

func setupAPITestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func adminSessionWithEmail(t *testing.T, db *sql.DB, email string) (int64, *http.Cookie) {
	t.Helper()
	uid, err := store.CreateUser(db, email, "Ad", "h", true, true)
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	raw, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("admin token: %v", err)
	}
	if err := store.CreateSession(db, auth.HashToken(raw), uid, 4_000_000_000); err != nil {
		t.Fatalf("admin session: %v", err)
	}
	return uid, &http.Cookie{Name: sessionCookie, Value: raw}
}

func upsertMFAFactor(t *testing.T, db *sql.DB, userID int64, enabledAt int64) {
	t.Helper()
	if err := store.UpsertMFAFactor(db, store.MFAFactor{
		UserID:           userID,
		SecretCiphertext: []byte("cipher"),
		SecretNonce:      []byte("nonce"),
		KeyVersion:       1,
		EnabledAt:        enabledAt,
		LastAcceptedStep: -1,
	}); err != nil {
		t.Fatalf("upsert mfa factor: %v", err)
	}
}

func TestAdminCreatePasswordResetReturnsLink(t *testing.T) {
	s, db := newTestServer(t)
	h, _ := auth.HashPassword("oldpassword")
	userID, _ := store.CreateUser(db, "reset@example.com", "Reset", h, false, true)
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

func TestAdminCreateEmailVerificationReturnsLinkForUnverifiedUser(t *testing.T) {
	s, db := newTestServer(t)
	userID, err := store.CreateUser(db, "manual@example.com", "Manual", "h", false, false)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, adminReq(t, db, "POST", "/api/admin/users/"+strconv.FormatInt(userID, 10)+"/email-verification", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("verification link: want 200, got %d (%s)", rec.Code, rec.Body)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["email"] != "manual@example.com" || !strings.Contains(body["link"], "/verify/") {
		t.Fatalf("verification link response mismatch: %#v", body)
	}
}

func TestAdminCreateEmailVerificationRejectsVerifiedUser(t *testing.T) {
	s, db := newTestServer(t)
	userID, err := store.CreateUser(db, "verified@example.com", "Verified", "h", false, true)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, adminReq(t, db, "POST", "/api/admin/users/"+strconv.FormatInt(userID, 10)+"/email-verification", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("verified user: want 400, got %d (%s)", rec.Code, rec.Body)
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

func TestAdminSettingsReadsAndWritesSecurityMode(t *testing.T) {
	db := setupAPITestDB(t)
	s := NewServer(db, nil, nil)

	req := adminReq(t, db, http.MethodPost, "/api/admin/settings", `{"security_mode":"household_profiles"}`)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := store.SecurityModeSetting(db); got != store.SecurityModeHouseholdProfiles {
		t.Fatalf("security mode = %q", got)
	}
	if !strings.Contains(rec.Body.String(), `"security_mode":"household_profiles"`) {
		t.Fatalf("response missing security mode: %s", rec.Body.String())
	}
}

func TestAdminSettingsSecurityModeWinsOverLegacyGuestAccess(t *testing.T) {
	db := setupAPITestDB(t)
	s := NewServer(db, nil, nil)

	req := adminReq(t, db, http.MethodPost, "/api/admin/settings", `{"security_mode":"household_profiles","guest_access_enabled":false}`)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := store.SecurityModeSetting(db); got != store.SecurityModeHouseholdProfiles {
		t.Fatalf("security mode = %q", got)
	}
	if !strings.Contains(rec.Body.String(), `"security_mode":"household_profiles"`) {
		t.Fatalf("response security mode mismatch: %s", rec.Body.String())
	}
}

func TestAdminSettingsRejectsUnsupportedSecurityMode(t *testing.T) {
	db := setupAPITestDB(t)
	s := NewServer(db, nil, nil)

	req := adminReq(t, db, http.MethodPost, "/api/admin/settings", `{"security_mode":"guest"}`)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminUserListIncludesPasswordlessProfileMarker(t *testing.T) {
	db := setupAPITestDB(t)
	profileID, err := store.CreatePasswordlessProfile(db, "Casey")
	if err != nil {
		t.Fatalf("CreatePasswordlessProfile: %v", err)
	}
	s := NewServer(db, nil, nil)

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, adminReq(t, db, http.MethodGet, "/api/admin/users", ""))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), fmt.Sprintf(`"id":%d`, profileID)) {
		t.Fatalf("response missing profile id: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"is_passwordless_profile":true`) {
		t.Fatalf("response missing passwordless marker: %s", rec.Body.String())
	}
}

func TestAdminSettingsIncludesAdminMFARequired(t *testing.T) {
	s, db := newTestServer(t)
	if err := store.SetAdminMFARequired(db, true); err != nil {
		t.Fatalf("set admin mfa required: %v", err)
	}
	adminID, cookie := adminSessionWithEmail(t, db, "settings-admin@example.com")
	upsertMFAFactor(t, db, adminID, time.Now().Unix())
	req := httptest.NewRequest(http.MethodGet, "/api/admin/settings", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.requireAdmin(s.getAdminSettings)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if body["admin_mfa_required"] != true {
		t.Fatalf("admin_mfa_required missing or false: %#v", body)
	}
}

func TestAdminSettingsEnableMFADeniesCurrentAdminWithoutEnabledMFA(t *testing.T) {
	s, db := newTestServer(t)
	currentAdminID, cookie := adminSessionWithEmail(t, db, "admin-without-mfa@example.com")
	upsertMFAFactor(t, db, currentAdminID, 0)
	otherAdminID, _ := adminSessionWithEmail(t, db, "other-admin-with-mfa@example.com")
	upsertMFAFactor(t, db, otherAdminID, time.Now().Unix())

	req := httptest.NewRequest(http.MethodPost, "/api/admin/settings", strings.NewReader(`{"admin_mfa_required":true}`))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.requireAdmin(s.setAdminSettings)(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("current admin without MFA: want 403, got %d (%s)", rec.Code, rec.Body)
	}
	if store.AdminMFARequired(db) {
		t.Fatalf("admin MFA requirement was enabled")
	}
}

func TestAdminSettingsEnableMFADeniesLockoutWhenNoAdminHasEnabledMFA(t *testing.T) {
	s, db := newTestServer(t)
	currentAdminID, cookie := adminSessionWithEmail(t, db, "pending-admin@example.com")
	upsertMFAFactor(t, db, currentAdminID, 0)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/settings", strings.NewReader(`{"admin_mfa_required":true}`))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.requireAdmin(s.setAdminSettings)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("no admin with MFA: want 400, got %d (%s)", rec.Code, rec.Body)
	}
	if store.AdminMFARequired(db) {
		t.Fatalf("admin MFA requirement was enabled")
	}
}

func TestAdminSettingsEnableMFASucceedsWhenCurrentAdminHasEnabledMFA(t *testing.T) {
	s, db := newTestServer(t)
	adminID, cookie := adminSessionWithEmail(t, db, "admin-enable-mfa@example.com")
	upsertMFAFactor(t, db, adminID, time.Now().Unix())

	req := httptest.NewRequest(http.MethodPost, "/api/admin/settings", strings.NewReader(`{"admin_mfa_required":true}`))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.requireAdmin(s.setAdminSettings)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("enable admin MFA: want 200, got %d (%s)", rec.Code, rec.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if body["admin_mfa_required"] != true {
		t.Fatalf("admin_mfa_required missing or false: %#v", body)
	}
	if !store.AdminMFARequired(db) {
		t.Fatalf("admin MFA requirement was not enabled")
	}
}

func TestAdminSettingsDisableMFASucceedsForAdmittedAdmin(t *testing.T) {
	s, db := newTestServer(t)
	if err := store.SetAdminMFARequired(db, true); err != nil {
		t.Fatalf("set admin mfa required: %v", err)
	}
	adminID, cookie := adminSessionWithEmail(t, db, "admin-disable-mfa@example.com")
	upsertMFAFactor(t, db, adminID, time.Now().Unix())

	req := httptest.NewRequest(http.MethodPost, "/api/admin/settings", strings.NewReader(`{"admin_mfa_required":false}`))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.requireAdmin(s.setAdminSettings)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("disable admin MFA: want 200, got %d (%s)", rec.Code, rec.Body)
	}
	if store.AdminMFARequired(db) {
		t.Fatalf("admin MFA requirement was not disabled")
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
	uid, _ := store.CreateUser(db, "u@b.com", "U", "h", false, true) // not admin
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

func TestRequireAdminBlocksAdminWithoutMFAWhenRequired(t *testing.T) {
	s, db := newTestServer(t)
	if err := store.SetAdminMFARequired(db, true); err != nil {
		t.Fatalf("set admin mfa required: %v", err)
	}
	_, cookie := adminSessionWithEmail(t, db, "admin-no-mfa@example.com")
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.requireAdmin(s.listUsers)(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("admin without MFA: want 403, got %d (%s)", rec.Code, rec.Body)
	}
}

func TestRequireAdminAllowsAuthenticatedRouteWhenAdminMFARequired(t *testing.T) {
	s, db := newTestServer(t)
	if err := store.SetAdminMFARequired(db, true); err != nil {
		t.Fatalf("set admin mfa required: %v", err)
	}
	_, cookie := adminSessionWithEmail(t, db, "admin-normal-route@example.com")
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("auth me with admin MFA required: want 200, got %d (%s)", rec.Code, rec.Body)
	}
}

func TestRequireAdminAllowsAdminWithMFAWhenRequired(t *testing.T) {
	s, db := newTestServer(t)
	if err := store.SetAdminMFARequired(db, true); err != nil {
		t.Fatalf("set admin mfa required: %v", err)
	}
	userID, cookie := adminSessionWithEmail(t, db, "admin-with-mfa@example.com")
	upsertMFAFactor(t, db, userID, time.Now().Unix())
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.requireAdmin(s.listUsers)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin with MFA: want 200, got %d (%s)", rec.Code, rec.Body)
	}
}

// nonAdminCookie seeds a non-admin account + session and returns its cookie.
func nonAdminCookie(t *testing.T, db *sql.DB) *http.Cookie {
	t.Helper()
	uid, _ := store.CreateUser(db, "user@b.com", "U", "h", false, true)
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

func TestHouseControlsAreModeAware(t *testing.T) {
	cases := []struct {
		mode store.SecurityMode
		want int
	}{
		{store.SecurityModeOpen, http.StatusOK},
		{store.SecurityModeOpenAdminLocked, http.StatusUnauthorized},
		{store.SecurityModeHouseholdProfiles, http.StatusUnauthorized},
		{store.SecurityModeFullLogin, http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(string(tc.mode), func(t *testing.T) {
			srv, db := newTestServer(t)
			if err := store.SetSecurityMode(db, tc.mode); err != nil {
				t.Fatalf("SetSecurityMode: %v", err)
			}
			rec := httptest.NewRecorder()

			req := httptest.NewRequest(http.MethodPost, "/api/streams/house/shuffle", strings.NewReader(`{"shuffle":true}`))
			srv.Handler().ServeHTTP(rec, req)

			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d body=%s", rec.Code, tc.want, rec.Body.String())
			}
		})
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

func TestListUsersIncludesMFAEnabled(t *testing.T) {
	s, db := newTestServer(t)
	noMFAID, err := store.CreateUser(db, "no-mfa@example.com", "No MFA", "h", false, true)
	if err != nil {
		t.Fatalf("create no mfa user: %v", err)
	}
	pendingMFAID, err := store.CreateUser(db, "pending-mfa@example.com", "Pending MFA", "h", false, true)
	if err != nil {
		t.Fatalf("create pending mfa user: %v", err)
	}
	enabledMFAID, err := store.CreateUser(db, "enabled-mfa@example.com", "Enabled MFA", "h", false, true)
	if err != nil {
		t.Fatalf("create enabled mfa user: %v", err)
	}
	upsertMFAFactor(t, db, pendingMFAID, 0)
	upsertMFAFactor(t, db, enabledMFAID, time.Now().Unix())

	req := adminReq(t, db, http.MethodGet, "/api/admin/users", "")
	rec := httptest.NewRecorder()
	s.requireAdmin(s.listUsers)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list users: want 200, got %d (%s)", rec.Code, rec.Body)
	}
	var users []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &users); err != nil {
		t.Fatalf("decode users: %v", err)
	}
	mfaEnabledByID := map[int64]bool{}
	for _, user := range users {
		id := int64(user["id"].(float64))
		mfaEnabled, ok := user["mfa_enabled"].(bool)
		if !ok {
			t.Fatalf("mfa_enabled missing for user %#v", user)
		}
		mfaEnabledByID[id] = mfaEnabled
	}
	if mfaEnabledByID[noMFAID] {
		t.Fatalf("no MFA user marked enabled")
	}
	if mfaEnabledByID[pendingMFAID] {
		t.Fatalf("pending MFA user marked enabled")
	}
	if !mfaEnabledByID[enabledMFAID] {
		t.Fatalf("enabled MFA user not marked enabled")
	}
}

// TestAdminLinkEndpointsRefuseWithoutPublicOrigin covers the three admin-facing
// link endpoints on a wildcard bind, where the link would otherwise come back
// pointing at the admin's own loopback and be pasted to someone it can't reach.
func TestAdminLinkEndpointsRefuseWithoutPublicOrigin(t *testing.T) {
	s, db := newTestServer(t)
	s.SetListenAddr("0.0.0.0:8066")
	userID, err := store.CreateUser(db, "remote@example.com", "Remote", "h", false, false)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	_, cookie := adminSessionWithEmail(t, db, "admin@b.com")
	idPath := "/api/admin/users/" + strconv.FormatInt(userID, 10)
	cases := []struct{ name, path, body string }{
		{"invite", "/api/admin/invites", `{"email":"x@y.com","is_admin":false}`},
		{"password reset", idPath + "/password-reset", ""},
		{"email verification", idPath + "/email-verification", ""},
	}
	for _, tc := range cases {
		req := httptest.NewRequest("POST", tc.path, strings.NewReader(tc.body))
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: want 503, got %d (%s)", tc.name, rec.Code, rec.Body)
			continue
		}
		if !strings.Contains(rec.Body.String(), "EXIT66_PUBLIC_ORIGIN") {
			t.Errorf("%s: error should name the missing setting: %s", tc.name, rec.Body)
		}
	}
	if invs, _ := store.ListInvites(db); len(invs) != 0 {
		t.Errorf("refused invite should not be stored, got %d", len(invs))
	}
}

// TestAdminLinkEndpointsUsePublicOriginOnWildcardBind is the other half: the
// same wildcard bind mints links once the origin is configured.
func TestAdminLinkEndpointsUsePublicOriginOnWildcardBind(t *testing.T) {
	s, db := newTestServer(t)
	s.SetListenAddr("0.0.0.0:8066")
	s.SetPublicOrigin("https://jukebox.example.com")
	userID, err := store.CreateUser(db, "remote@example.com", "Remote", "h", false, false)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	_, cookie := adminSessionWithEmail(t, db, "admin@b.com")
	idPath := "/api/admin/users/" + strconv.FormatInt(userID, 10)
	cases := []struct{ name, path, body, wantPrefix string }{
		{"invite", "/api/admin/invites", `{"email":"x@y.com","is_admin":false}`, "https://jukebox.example.com/invite/"},
		{"password reset", idPath + "/password-reset", "", "https://jukebox.example.com/reset-password/"},
		{"email verification", idPath + "/email-verification", "", "https://jukebox.example.com/verify/"},
	}
	for _, tc := range cases {
		req := httptest.NewRequest("POST", tc.path, strings.NewReader(tc.body))
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: want 200, got %d (%s)", tc.name, rec.Code, rec.Body)
			continue
		}
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Errorf("%s: decode: %v", tc.name, err)
			continue
		}
		if !strings.HasPrefix(body["link"], tc.wantPrefix) {
			t.Errorf("%s: link = %q, want prefix %q", tc.name, body["link"], tc.wantPrefix)
		}
	}
}

// TestPublicOriginDocumentedInREADME keeps the 503 and the docs on the same
// variable name. The message tells the operator to set something they then have
// to look up, so a rename here that misses the README leaves them with a name
// that appears nowhere they can read about it.
func TestPublicOriginDocumentedInREADME(t *testing.T) {
	name, _, ok := strings.Cut(strings.TrimPrefix(publicOriginRequired, "set "), ":")
	if !ok || !strings.HasPrefix(name, "EXIT66_") {
		t.Fatalf("cannot read a variable name out of %q", publicOriginRequired)
	}
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	if !strings.Contains(string(readme), name) {
		t.Fatalf("README does not mention %s, the variable the 503 tells operators to set", name)
	}
}
