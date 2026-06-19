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

func TestMiddlewareBlocksAnonymous(t *testing.T) {
	s, _ := newTestServer(t)
	h := s.requireAuth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest("GET", "/api/tracks", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestForgotPasswordDoesNotRevealAccountsOrReturnLink(t *testing.T) {
	s, db := newTestServer(t)
	h, _ := auth.HashPassword("oldpassword")
	store.CreateUser(db, "reset@example.com", "Reset", h, false)

	existing := httptest.NewRecorder()
	s.forgotPassword(existing, httptest.NewRequest("POST", "/api/auth/password-reset/forgot", strings.NewReader(`{"email":"reset@example.com"}`)))
	missing := httptest.NewRecorder()
	s.forgotPassword(missing, httptest.NewRequest("POST", "/api/auth/password-reset/forgot", strings.NewReader(`{"email":"missing@example.com"}`)))

	if existing.Code != http.StatusOK || missing.Code != http.StatusOK {
		t.Fatalf("forgot responses: existing=%d missing=%d", existing.Code, missing.Code)
	}
	if existing.Body.String() != missing.Body.String() {
		t.Fatalf("forgot password leaked account existence: existing=%s missing=%s", existing.Body, missing.Body)
	}
	if strings.Contains(existing.Body.String(), "link") {
		t.Fatalf("public forgot password returned a link without SMTP: %s", existing.Body)
	}
}

func TestResetPasswordSetsNewPasswordWithoutLoginAndInvalidatesSessions(t *testing.T) {
	s, db := newTestServer(t)
	oldHash, _ := auth.HashPassword("oldpassword")
	userID, _ := store.CreateUser(db, "reset@example.com", "Reset", oldHash, false)
	sessionRaw, _ := auth.GenerateToken()
	store.CreateSession(db, auth.HashToken(sessionRaw), userID, 4_000_000_000)
	tokenRaw, _ := auth.GenerateToken()
	store.CreatePasswordReset(db, auth.HashToken(tokenRaw), userID, 4_000_000_000)

	rec := httptest.NewRecorder()
	body := `{"token":"` + tokenRaw + `","password":"newpassword"}`
	s.resetPassword(rec, httptest.NewRequest("POST", "/api/auth/password-reset/redeem", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("reset: want 200, got %d (%s)", rec.Code, rec.Body)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatalf("reset should not log in, got cookies: %+v", rec.Result().Cookies())
	}

	oldLogin := httptest.NewRecorder()
	s.login(oldLogin, httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"email":"reset@example.com","password":"oldpassword"}`)))
	if oldLogin.Code != http.StatusUnauthorized {
		t.Fatalf("old password should fail after reset, got %d", oldLogin.Code)
	}
	newLogin := httptest.NewRecorder()
	s.login(newLogin, httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"email":"reset@example.com","password":"newpassword"}`)))
	if newLogin.Code != http.StatusOK {
		t.Fatalf("new password should work after reset, got %d (%s)", newLogin.Code, newLogin.Body)
	}
	if _, ok, _ := store.UserBySession(db, auth.HashToken(sessionRaw)); ok {
		t.Fatal("reset did not invalidate existing sessions")
	}
	reuse := httptest.NewRecorder()
	s.resetPassword(reuse, httptest.NewRequest("POST", "/api/auth/password-reset/redeem", strings.NewReader(body)))
	if reuse.Code == http.StatusOK {
		t.Fatal("reset token was reusable")
	}
}

func TestForgotPasswordSendsEmailWhenConfigured(t *testing.T) {
	s, db := newTestServer(t)
	h, _ := auth.HashPassword("oldpassword")
	store.CreateUser(db, "reset@example.com", "Reset", h, false)
	var sentTo, sentLink string
	s.SetPasswordResetEmailer(func(to, link string) { sentTo, sentLink = to, link })

	rec := httptest.NewRecorder()
	s.forgotPassword(rec, httptest.NewRequest("POST", "/api/auth/password-reset/forgot", strings.NewReader(`{"email":"reset@example.com"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("forgot: want 200, got %d", rec.Code)
	}
	if sentTo != "reset@example.com" || !strings.Contains(sentLink, "/reset-password/") {
		t.Fatalf("reset email not sent: to=%q link=%q", sentTo, sentLink)
	}
}

func TestForgotPasswordThrottle(t *testing.T) {
	s, _ := newTestServer(t)
	var last int
	for i := 0; i < 12; i++ {
		rec := httptest.NewRecorder()
		s.forgotPassword(rec, httptest.NewRequest("POST", "/api/auth/password-reset/forgot", strings.NewReader(`{"email":"reset@example.com"}`)))
		last = rec.Code
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("forgot password throttle should trip: got %d", last)
	}
}

func TestMiddlewareAllowsSession(t *testing.T) {
	s, db := newTestServer(t)
	uid, _ := store.CreateUser(db, "a@b.com", "A", "h", false)
	raw, _ := auth.GenerateToken()
	store.CreateSession(db, auth.HashToken(raw), uid, 4_000_000_000)
	h := s.requireAuth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/tracks", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: raw})
	h(rec, req)
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d", rec.Code)
	}
}

func TestMiddlewareGuestToggle(t *testing.T) {
	s, db := newTestServer(t)
	store.SetGuestAccessEnabled(db, true)
	h := s.requireAuth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest("GET", "/api/tracks", nil))
	if rec.Code != 200 {
		t.Fatalf("guest access on: want 200, got %d", rec.Code)
	}
}

func TestSignupBootstrapsAdmin(t *testing.T) {
	s, db := newTestServer(t)
	rec := httptest.NewRecorder()
	body := `{"email":"a@b.com","display_name":"A","password":"pw123456"}`
	s.signup(rec, httptest.NewRequest("POST", "/api/auth/signup", strings.NewReader(body)))
	if rec.Code != 200 {
		t.Fatalf("bootstrap signup: want 200, got %d (%s)", rec.Code, rec.Body)
	}
	u, _, _ := store.GetUserByEmail(db, "a@b.com")
	if !u.IsAdmin {
		t.Fatal("first user not admin")
	}
	rec2 := httptest.NewRecorder()
	body2 := `{"email":"c@d.com","display_name":"C","password":"pw123456"}`
	s.signup(rec2, httptest.NewRequest("POST", "/api/auth/signup", strings.NewReader(body2)))
	if rec2.Code == 200 {
		t.Fatal("second signup allowed while disabled")
	}
}

func TestLoginSetsCookieAndMe(t *testing.T) {
	s, db := newTestServer(t)
	h, _ := auth.HashPassword("pw123456")
	store.CreateUser(db, "a@b.com", "A", h, true)
	rec := httptest.NewRecorder()
	s.login(rec, httptest.NewRequest("POST", "/api/auth/login",
		strings.NewReader(`{"email":"a@b.com","password":"pw123456"}`)))
	if rec.Code != 200 {
		t.Fatalf("login: want 200, got %d", rec.Code)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != sessionCookie {
		t.Fatal("no session cookie set")
	}
	meReq := httptest.NewRequest("GET", "/api/auth/me", nil)
	meReq.AddCookie(cookies[0])
	meRec := httptest.NewRecorder()
	s.me(meRec, meReq)
	if meRec.Code != 200 || !strings.Contains(meRec.Body.String(), "a@b.com") {
		t.Fatalf("me: %d %s", meRec.Code, meRec.Body)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	s, db := newTestServer(t)
	h, _ := auth.HashPassword("right")
	store.CreateUser(db, "a@b.com", "A", h, false)
	rec := httptest.NewRecorder()
	s.login(rec, httptest.NewRequest("POST", "/api/auth/login",
		strings.NewReader(`{"email":"a@b.com","password":"wrong"}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestInviteAcceptCreatesUser(t *testing.T) {
	s, db := newTestServer(t)
	admin, _ := store.CreateUser(db, "admin@b.com", "Ad", "h", true)
	raw, _ := auth.GenerateToken()
	store.CreateInvite(db, auth.HashToken(raw), "inv@b.com", true, admin, 4_000_000_000)
	rec := httptest.NewRecorder()
	body := `{"token":"` + raw + `","display_name":"Inv","password":"pw123456"}`
	s.inviteAccept(rec, httptest.NewRequest("POST", "/api/auth/invite/accept", strings.NewReader(body)))
	if rec.Code != 200 {
		t.Fatalf("accept: want 200, got %d (%s)", rec.Code, rec.Body)
	}
	u, ok, _ := store.GetUserByEmail(db, "inv@b.com")
	if !ok || !u.IsAdmin {
		t.Fatalf("invited admin not created: ok=%v admin=%v", ok, u.IsAdmin)
	}
	rec2 := httptest.NewRecorder()
	s.inviteAccept(rec2, httptest.NewRequest("POST", "/api/auth/invite/accept", strings.NewReader(body)))
	if rec2.Code == 200 {
		t.Fatal("invite reused")
	}
}

func TestLoginThrottlePerEmailDefeatsIPRotation(t *testing.T) {
	s, _ := newTestServer(t)
	body := `{"email":"victim@b.com","password":"x"}`
	var last int
	for i := 0; i < 12; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
		req.RemoteAddr = "203.0.113." + strconv.Itoa(i) + ":1234" // distinct public IP each attempt
		s.login(rec, req)
		last = rec.Code
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("per-email throttle should trip despite rotating IPs: got %d", last)
	}
}
