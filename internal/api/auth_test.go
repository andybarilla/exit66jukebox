package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

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
	s.SetPublicOrigin("https://jukebox.example.com")
	h, _ := auth.HashPassword("oldpassword")
	store.CreateUser(db, "reset@example.com", "Reset", h, false)
	var sentTo, sentLink string
	s.SetPasswordResetEmailer(func(to, link string) { sentTo, sentLink = to, link })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/auth/password-reset/forgot", strings.NewReader(`{"email":"reset@example.com"}`))
	req.Host = "attacker.example.net"
	s.forgotPassword(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("forgot: want 200, got %d", rec.Code)
	}
	if sentTo != "reset@example.com" || !strings.HasPrefix(sentLink, "https://jukebox.example.com/reset-password/") {
		t.Fatalf("reset email not sent: to=%q link=%q", sentTo, sentLink)
	}
	if strings.Contains(sentLink, "attacker.example.net") {
		t.Fatalf("reset email link used Host header: %q", sentLink)
	}
}

func TestPasswordResetEndpointsPassProductionMiddleware(t *testing.T) {
	s, db := newTestServer(t)
	s.SetPublicOrigin("https://jukebox.example.com")
	hash, _ := auth.HashPassword("oldpassword")
	userID, _ := store.CreateUser(db, "reset@example.com", "Reset", hash, false)
	tokenRaw, _ := auth.GenerateToken()
	store.CreatePasswordReset(db, auth.HashToken(tokenRaw), userID, 4_000_000_000)
	h := s.RequireAuthMiddleware(s.Handler())

	forgot := httptest.NewRecorder()
	forgotReq := httptest.NewRequest("POST", "/api/auth/password-reset/forgot", strings.NewReader(`{"email":"reset@example.com"}`))
	forgotReq.RemoteAddr = "203.0.113.9:1"
	h.ServeHTTP(forgot, forgotReq)
	if forgot.Code == http.StatusUnauthorized {
		t.Fatal("forgot password endpoint was blocked by production middleware")
	}

	redeem := httptest.NewRecorder()
	redeemReq := httptest.NewRequest("POST", "/api/auth/password-reset/redeem", strings.NewReader(`{"token":"`+tokenRaw+`","password":"newpassword"}`))
	redeemReq.RemoteAddr = "203.0.113.9:1"
	h.ServeHTTP(redeem, redeemReq)
	if redeem.Code == http.StatusUnauthorized {
		t.Fatal("password reset redeem endpoint was blocked by production middleware")
	}
	if redeem.Code != http.StatusOK {
		t.Fatalf("redeem: want 200, got %d (%s)", redeem.Code, redeem.Body)
	}
}

func TestResetPasswordTokenIsSingleUseBeforePasswordChange(t *testing.T) {
	s, db := newTestServer(t)
	oldHash, _ := auth.HashPassword("oldpassword")
	userID, _ := store.CreateUser(db, "reset@example.com", "Reset", oldHash, false)
	tokenRaw, _ := auth.GenerateToken()
	store.CreatePasswordReset(db, auth.HashToken(tokenRaw), userID, 4_000_000_000)
	body := `{"token":"` + tokenRaw + `","password":"firstpassword"}`

	first := httptest.NewRecorder()
	s.resetPassword(first, httptest.NewRequest("POST", "/api/auth/password-reset/redeem", strings.NewReader(body)))
	if first.Code != http.StatusOK {
		t.Fatalf("first reset: want 200, got %d (%s)", first.Code, first.Body)
	}

	second := httptest.NewRecorder()
	s.resetPassword(second, httptest.NewRequest("POST", "/api/auth/password-reset/redeem", strings.NewReader(`{"token":"`+tokenRaw+`","password":"secondpassword"}`)))
	if second.Code == http.StatusOK {
		t.Fatal("reset token was accepted twice")
	}

	login := httptest.NewRecorder()
	s.login(login, httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"email":"reset@example.com","password":"secondpassword"}`)))
	if login.Code == http.StatusOK {
		t.Fatalf("second reset changed password after token was consumed: %s", second.Body)
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

func TestLoginWithMFAEnabledReturnsTicketAndNoSession(t *testing.T) {
	s, db := newTestServer(t)
	createMFAUser(t, s, db, "mfa@example.com")

	rec := httptest.NewRecorder()
	s.login(rec, httptest.NewRequest("POST", "/api/auth/login",
		strings.NewReader(`{"email":"mfa@example.com","password":"pw123456"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("login: want 200, got %d (%s)", rec.Code, rec.Body)
	}
	if sessionCookieFrom(rec) != nil {
		t.Fatal("MFA password login set a session cookie")
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if body["mfa_required"] != true || body["ticket"] == "" {
		t.Fatalf("login did not return MFA ticket: %v", body)
	}
}

func TestMFACompleteTOTPCreatesSession(t *testing.T) {
	s, db := newTestServer(t)
	secret, _ := createMFAUser(t, s, db, "mfa@example.com")
	ticket := loginMFATicket(t, s, "mfa@example.com")
	code, err := auth.TOTPCode(secret, time.Now())
	if err != nil {
		t.Fatalf("totp code: %v", err)
	}

	rec := httptest.NewRecorder()
	s.mfaComplete(rec, httptest.NewRequest("POST", "/api/auth/mfa/complete",
		strings.NewReader(`{"ticket":"`+ticket+`","code":"`+code+`"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("mfa complete: want 200, got %d (%s)", rec.Code, rec.Body)
	}
	if sessionCookieFrom(rec) == nil {
		t.Fatal("MFA completion did not set session cookie")
	}
}

func TestMFACompleteRejectsReusedTicket(t *testing.T) {
	s, db := newTestServer(t)
	secret, _ := createMFAUser(t, s, db, "mfa@example.com")
	ticket := loginMFATicket(t, s, "mfa@example.com")
	code, _ := auth.TOTPCode(secret, time.Now())
	body := `{"ticket":"` + ticket + `","code":"` + code + `"}`

	first := httptest.NewRecorder()
	s.mfaComplete(first, httptest.NewRequest("POST", "/api/auth/mfa/complete", strings.NewReader(body)))
	if first.Code != http.StatusOK {
		t.Fatalf("first mfa complete: want 200, got %d (%s)", first.Code, first.Body)
	}
	second := httptest.NewRecorder()
	s.mfaComplete(second, httptest.NewRequest("POST", "/api/auth/mfa/complete", strings.NewReader(body)))
	if second.Code == http.StatusOK || sessionCookieFrom(second) != nil {
		t.Fatalf("reused ticket accepted: %d %s", second.Code, second.Body)
	}
}

func TestMFACompleteRejectsExpiredTicket(t *testing.T) {
	s, db := newTestServer(t)
	secret, userID := createMFAUser(t, s, db, "mfa@example.com")
	ticket, _ := auth.GenerateToken()
	if err := store.CreateMFATicket(db, auth.HashToken(ticket), userID, time.Now().Add(-time.Minute).Unix()); err != nil {
		t.Fatalf("create expired ticket: %v", err)
	}
	code, _ := auth.TOTPCode(secret, time.Now())

	rec := httptest.NewRecorder()
	s.mfaComplete(rec, httptest.NewRequest("POST", "/api/auth/mfa/complete",
		strings.NewReader(`{"ticket":"`+ticket+`","code":"`+code+`"}`)))

	if rec.Code == http.StatusOK || sessionCookieFrom(rec) != nil {
		t.Fatalf("expired ticket accepted: %d %s", rec.Code, rec.Body)
	}
}

func TestMFACompleteRejectsReusedTOTPStep(t *testing.T) {
	s, db := newTestServer(t)
	secret, _ := createMFAUser(t, s, db, "mfa@example.com")
	code, _ := auth.TOTPCode(secret, time.Now())

	first := httptest.NewRecorder()
	s.mfaComplete(first, httptest.NewRequest("POST", "/api/auth/mfa/complete",
		strings.NewReader(`{"ticket":"`+loginMFATicket(t, s, "mfa@example.com")+`","code":"`+code+`"}`)))
	if first.Code != http.StatusOK {
		t.Fatalf("first mfa complete: want 200, got %d (%s)", first.Code, first.Body)
	}
	second := httptest.NewRecorder()
	s.mfaComplete(second, httptest.NewRequest("POST", "/api/auth/mfa/complete",
		strings.NewReader(`{"ticket":"`+loginMFATicket(t, s, "mfa@example.com")+`","code":"`+code+`"}`)))
	if second.Code == http.StatusOK || sessionCookieFrom(second) != nil {
		t.Fatalf("reused TOTP step accepted: %d %s", second.Code, second.Body)
	}
}

func TestAcceptMFAChallengeRejectsStaleAcceptedTOTPStep(t *testing.T) {
	s, db := newTestServer(t)
	secret, userID := createMFAUser(t, s, db, "mfa-stale@example.com")
	factor, ok, err := store.GetMFAFactor(db, userID)
	if err != nil || !ok {
		t.Fatalf("get factor: ok=%v err=%v", ok, err)
	}
	code, err := auth.TOTPCode(secret, time.Now())
	if err != nil {
		t.Fatalf("totp code: %v", err)
	}

	first := httptest.NewRecorder()
	if !s.acceptMFAChallenge(first, mfaCompleteReq{Code: code}, factor) {
		t.Fatalf("first stale-factor challenge rejected: %d %s", first.Code, first.Body)
	}
	second := httptest.NewRecorder()
	if s.acceptMFAChallenge(second, mfaCompleteReq{Code: code}, factor) {
		t.Fatalf("same accepted TOTP step with stale factor accepted: %d %s", second.Code, second.Body)
	}
	if second.Code != http.StatusUnauthorized {
		t.Fatalf("same accepted TOTP step status = %d; want %d", second.Code, http.StatusUnauthorized)
	}
}

func TestMFACompleteRecoveryCodeCreatesSessionOnce(t *testing.T) {
	s, db := newTestServer(t)
	_, userID := createMFAUser(t, s, db, "mfa@example.com")
	recoveryCode := "ABCD-EFGH-IJKL-MNOP"
	if err := store.ReplaceRecoveryCodes(db, userID, []string{auth.HashRecoveryCode(recoveryCode)}); err != nil {
		t.Fatalf("replace recovery codes: %v", err)
	}

	first := httptest.NewRecorder()
	s.mfaComplete(first, httptest.NewRequest("POST", "/api/auth/mfa/complete",
		strings.NewReader(`{"ticket":"`+loginMFATicket(t, s, "mfa@example.com")+`","recovery_code":"`+recoveryCode+`"}`)))
	if first.Code != http.StatusOK || sessionCookieFrom(first) == nil {
		t.Fatalf("recovery complete: want 200 with cookie, got %d (%s)", first.Code, first.Body)
	}
	second := httptest.NewRecorder()
	s.mfaComplete(second, httptest.NewRequest("POST", "/api/auth/mfa/complete",
		strings.NewReader(`{"ticket":"`+loginMFATicket(t, s, "mfa@example.com")+`","code":"`+recoveryCode+`"}`)))
	if second.Code == http.StatusOK || sessionCookieFrom(second) != nil {
		t.Fatalf("reused recovery code accepted: %d %s", second.Code, second.Body)
	}
}

func TestMFACompletePassesProductionMiddleware(t *testing.T) {
	s, _ := newTestServer(t)
	h := s.RequireAuthMiddleware(s.Handler())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/auth/mfa/complete", strings.NewReader(`{"ticket":"missing","code":"000000"}`))
	req.RemoteAddr = "203.0.113.9:1"
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized && strings.Contains(rec.Body.String(), "login required") {
		t.Fatal("MFA completion endpoint was blocked by production middleware")
	}
}

func TestMFAEnrollMissingOrInvalidKeyFailsWithoutEnablingMFA(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  []byte
	}{
		{name: "missing key"},
		{name: "invalid key", key: []byte("short")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, db := newTestServer(t)
			if tc.key != nil {
				s.SetMFAKey(tc.key)
			}
			_, userID, cookie := createAuthenticatedUser(t, s, db, "enroll-key@example.com")
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/api/auth/mfa/enroll/begin", nil)
			req.AddCookie(cookie)

			s.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("begin with bad key: want 500, got %d (%s)", rec.Code, rec.Body)
			}
			factor, ok, err := store.GetMFAFactor(db, userID)
			if err != nil {
				t.Fatalf("get mfa factor: %v", err)
			}
			if ok && factor.EnabledAt > 0 {
				t.Fatalf("bad key enabled MFA: %+v", factor)
			}
		})
	}
}

func TestMFAEnrollBeginAndConfirmEnablesMFAAndReturnsRecoveryCodesOnce(t *testing.T) {
	s, db := newTestServer(t)
	s.SetMFAKey([]byte("12345678901234567890123456789012"))
	_, userID, cookie := createAuthenticatedUser(t, s, db, "enroll@example.com")

	begin := httptest.NewRecorder()
	beginReq := httptest.NewRequest("POST", "/api/auth/mfa/enroll/begin", nil)
	beginReq.AddCookie(cookie)
	s.Handler().ServeHTTP(begin, beginReq)
	if begin.Code != http.StatusOK {
		t.Fatalf("begin: want 200, got %d (%s)", begin.Code, begin.Body)
	}
	var beginBody struct {
		Secret     string `json:"secret"`
		OTPAUTHURI string `json:"otpauth_uri"`
	}
	if err := json.NewDecoder(begin.Body).Decode(&beginBody); err != nil {
		t.Fatalf("decode begin: %v", err)
	}
	if beginBody.Secret == "" || !strings.Contains(beginBody.OTPAUTHURI, "otpauth://totp/Exit66:enroll%40example.com") {
		t.Fatalf("begin did not return secret and otpauth uri: %+v", beginBody)
	}

	code, err := auth.TOTPCode(beginBody.Secret, time.Now())
	if err != nil {
		t.Fatalf("totp code: %v", err)
	}
	confirm := httptest.NewRecorder()
	confirmReq := httptest.NewRequest("POST", "/api/auth/mfa/enroll/confirm", strings.NewReader(`{"code":"`+code+`"}`))
	confirmReq.AddCookie(cookie)
	s.Handler().ServeHTTP(confirm, confirmReq)
	if confirm.Code != http.StatusOK {
		t.Fatalf("confirm: want 200, got %d (%s)", confirm.Code, confirm.Body)
	}
	var confirmBody struct {
		RecoveryCodes []string `json:"recovery_codes"`
	}
	if err := json.NewDecoder(confirm.Body).Decode(&confirmBody); err != nil {
		t.Fatalf("decode confirm: %v", err)
	}
	if len(confirmBody.RecoveryCodes) != 10 {
		t.Fatalf("confirm returned %d recovery codes", len(confirmBody.RecoveryCodes))
	}
	factor, ok, err := store.GetMFAFactor(db, userID)
	if err != nil || !ok || factor.EnabledAt <= 0 {
		t.Fatalf("MFA not enabled: factor=%+v ok=%v err=%v", factor, ok, err)
	}
	hashes, err := store.ListRecoveryCodeHashes(db, userID)
	if err != nil {
		t.Fatalf("list recovery hashes: %v", err)
	}
	if len(hashes) != len(confirmBody.RecoveryCodes) {
		t.Fatalf("stored %d hashes for %d codes", len(hashes), len(confirmBody.RecoveryCodes))
	}
	for i, recoveryCode := range confirmBody.RecoveryCodes {
		if hashes[i] == recoveryCode {
			t.Fatalf("stored recovery code plaintext at index %d", i)
		}
		if hashes[i] != auth.HashRecoveryCode(recoveryCode) {
			t.Fatalf("stored hash mismatch at index %d", i)
		}
	}

	failure := httptest.NewRecorder()
	failureReq := httptest.NewRequest("POST", "/api/auth/mfa/recovery/regenerate", strings.NewReader(`{"password":"wrongpassword","code":"`+confirmBody.RecoveryCodes[0]+`"}`))
	failureReq.AddCookie(cookie)
	s.Handler().ServeHTTP(failure, failureReq)
	if failure.Code == http.StatusOK {
		t.Fatalf("regenerate with wrong password succeeded: %s", failure.Body)
	}
	for _, recoveryCode := range confirmBody.RecoveryCodes {
		if strings.Contains(failure.Body.String(), recoveryCode) {
			t.Fatalf("failure response exposed recovery code %q: %s", recoveryCode, failure.Body)
		}
	}
}

func TestMFAEnrollBeginRejectsAlreadyEnabledMFAWithoutDowngradingFactor(t *testing.T) {
	s, db := newTestServer(t)
	_, userID := createMFAUser(t, s, db, "enabled-enroll@example.com")
	cookie := createSessionCookie(t, db, userID)
	originalFactor, ok, err := store.GetMFAFactor(db, userID)
	if err != nil || !ok || originalFactor.EnabledAt <= 0 {
		t.Fatalf("MFA fixture not enabled: factor=%+v ok=%v err=%v", originalFactor, ok, err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/auth/mfa/enroll/begin", nil)
	req.AddCookie(cookie)
	s.Handler().ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("begin allowed already-enabled MFA: %s", rec.Body)
	}
	currentFactor, ok, err := store.GetMFAFactor(db, userID)
	if err != nil || !ok {
		t.Fatalf("enabled factor missing after rejected begin: ok=%v err=%v", ok, err)
	}
	if currentFactor.EnabledAt != originalFactor.EnabledAt || currentFactor.LastAcceptedStep != originalFactor.LastAcceptedStep || currentFactor.KeyVersion != originalFactor.KeyVersion {
		t.Fatalf("enabled factor metadata changed: before=%+v after=%+v", originalFactor, currentFactor)
	}
	if string(currentFactor.SecretCiphertext) != string(originalFactor.SecretCiphertext) || string(currentFactor.SecretNonce) != string(originalFactor.SecretNonce) {
		t.Fatalf("enabled factor secret changed: before=%+v after=%+v", originalFactor, currentFactor)
	}

	login := httptest.NewRecorder()
	s.login(login, httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"email":"enabled-enroll@example.com","password":"pw123456"}`)))
	if login.Code != http.StatusOK || sessionCookieFrom(login) != nil {
		t.Fatalf("login after rejected begin did not require MFA: code=%d body=%s", login.Code, login.Body)
	}
	var body struct {
		Ticket      string `json:"ticket"`
		MFARequired bool   `json:"mfa_required"`
	}
	if err := json.NewDecoder(login.Body).Decode(&body); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if !body.MFARequired || body.Ticket == "" {
		t.Fatalf("login after rejected begin did not return MFA ticket: %+v", body)
	}
}

func TestMFAEnrollConfirmRejectsAlreadyEnabledMFAWithoutReplacingRecoveryCodes(t *testing.T) {
	s, db := newTestServer(t)
	secret, userID := createMFAUser(t, s, db, "enabled-confirm@example.com")
	existingRecoveryHash := auth.HashRecoveryCode("ABCD-EFGH-IJKL-MNOP")
	if err := store.ReplaceRecoveryCodes(db, userID, []string{existingRecoveryHash}); err != nil {
		t.Fatalf("replace recovery codes: %v", err)
	}
	cookie := createSessionCookie(t, db, userID)
	originalFactor, ok, err := store.GetMFAFactor(db, userID)
	if err != nil || !ok || originalFactor.EnabledAt <= 0 {
		t.Fatalf("MFA fixture not enabled: factor=%+v ok=%v err=%v", originalFactor, ok, err)
	}
	originalRecoveryHashes, err := store.ListRecoveryCodeHashes(db, userID)
	if err != nil {
		t.Fatalf("list original recovery hashes: %v", err)
	}
	code, err := auth.TOTPCode(secret, time.Now())
	if err != nil {
		t.Fatalf("totp code: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/auth/mfa/enroll/confirm", strings.NewReader(`{"code":"`+code+`"}`))
	req.AddCookie(cookie)
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("confirm enabled MFA: want 409, got %d (%s)", rec.Code, rec.Body)
	}
	for _, forbidden := range []string{"recovery_codes", "secret", "otpauth_uri"} {
		if strings.Contains(rec.Body.String(), forbidden) {
			t.Fatalf("rejected confirm exposed %q: %s", forbidden, rec.Body)
		}
	}
	currentFactor, ok, err := store.GetMFAFactor(db, userID)
	if err != nil || !ok {
		t.Fatalf("enabled factor missing after rejected confirm: ok=%v err=%v", ok, err)
	}
	if currentFactor.EnabledAt != originalFactor.EnabledAt || currentFactor.LastAcceptedStep != originalFactor.LastAcceptedStep || currentFactor.KeyVersion != originalFactor.KeyVersion {
		t.Fatalf("enabled factor metadata changed: before=%+v after=%+v", originalFactor, currentFactor)
	}
	if string(currentFactor.SecretCiphertext) != string(originalFactor.SecretCiphertext) || string(currentFactor.SecretNonce) != string(originalFactor.SecretNonce) {
		t.Fatalf("enabled factor secret changed: before=%+v after=%+v", originalFactor, currentFactor)
	}
	currentRecoveryHashes, err := store.ListRecoveryCodeHashes(db, userID)
	if err != nil {
		t.Fatalf("list current recovery hashes: %v", err)
	}
	if len(currentRecoveryHashes) != len(originalRecoveryHashes) {
		t.Fatalf("recovery hashes changed count: before=%v after=%v", originalRecoveryHashes, currentRecoveryHashes)
	}
	for i := range originalRecoveryHashes {
		if currentRecoveryHashes[i] != originalRecoveryHashes[i] {
			t.Fatalf("recovery hash changed at %d: before=%v after=%v", i, originalRecoveryHashes, currentRecoveryHashes)
		}
	}
}

func TestMFADisableRequiresPasswordAndCurrentTOTPOrRecoveryCode(t *testing.T) {
	s, db := newTestServer(t)
	secret, userID := createMFAUser(t, s, db, "disable@example.com")
	recoveryCode := "ABCD-EFGH-IJKL-MNOP"
	if err := store.ReplaceRecoveryCodes(db, userID, []string{auth.HashRecoveryCode(recoveryCode)}); err != nil {
		t.Fatalf("replace recovery codes: %v", err)
	}
	cookie := createSessionCookie(t, db, userID)
	code, _ := auth.TOTPCode(secret, time.Now())

	wrongPassword := httptest.NewRecorder()
	wrongPasswordReq := httptest.NewRequest("POST", "/api/auth/mfa/disable", strings.NewReader(`{"password":"wrongpassword","code":"`+code+`"}`))
	wrongPasswordReq.AddCookie(cookie)
	s.Handler().ServeHTTP(wrongPassword, wrongPasswordReq)
	if wrongPassword.Code == http.StatusOK || strings.Contains(wrongPassword.Body.String(), "wrongpassword") || strings.Contains(wrongPassword.Body.String(), code) {
		t.Fatalf("wrong password accepted or echoed secrets: %d %s", wrongPassword.Code, wrongPassword.Body)
	}

	wrongCode := httptest.NewRecorder()
	wrongCodeReq := httptest.NewRequest("POST", "/api/auth/mfa/disable", strings.NewReader(`{"password":"pw123456","code":"000000"}`))
	wrongCodeReq.AddCookie(cookie)
	s.Handler().ServeHTTP(wrongCode, wrongCodeReq)
	if wrongCode.Code == http.StatusOK || strings.Contains(wrongCode.Body.String(), "000000") {
		t.Fatalf("wrong code accepted or echoed: %d %s", wrongCode.Code, wrongCode.Body)
	}

	success := httptest.NewRecorder()
	successReq := httptest.NewRequest("POST", "/api/auth/mfa/disable", strings.NewReader(`{"password":"pw123456","code":"`+code+`"}`))
	successReq.AddCookie(cookie)
	s.Handler().ServeHTTP(success, successReq)
	if success.Code != http.StatusOK {
		t.Fatalf("disable with totp: want 200, got %d (%s)", success.Code, success.Body)
	}
	if _, ok, err := store.GetMFAFactor(db, userID); err != nil || ok {
		t.Fatalf("disable left factor: ok=%v err=%v", ok, err)
	}
	hashes, err := store.ListRecoveryCodeHashes(db, userID)
	if err != nil || len(hashes) != 0 {
		t.Fatalf("disable left recovery hashes: hashes=%v err=%v", hashes, err)
	}

	_, recoveryUserID := createMFAUser(t, s, db, "disable-recovery@example.com")
	if err := store.ReplaceRecoveryCodes(db, recoveryUserID, []string{auth.HashRecoveryCode(recoveryCode)}); err != nil {
		t.Fatalf("replace recovery codes: %v", err)
	}
	recoverySuccess := httptest.NewRecorder()
	recoveryReq := httptest.NewRequest("POST", "/api/auth/mfa/disable", strings.NewReader(`{"password":"pw123456","code":"`+recoveryCode+`"}`))
	recoveryReq.AddCookie(createSessionCookie(t, db, recoveryUserID))
	s.Handler().ServeHTTP(recoverySuccess, recoveryReq)
	if recoverySuccess.Code != http.StatusOK {
		t.Fatalf("disable with recovery code: want 200, got %d (%s)", recoverySuccess.Code, recoverySuccess.Body)
	}
}

func TestRecoveryRegenerationRequiresVerificationAndReplacesRecoveryCodes(t *testing.T) {
	s, db := newTestServer(t)
	secret, userID := createMFAUser(t, s, db, "regenerate@example.com")
	oldRecoveryCode := "ABCD-EFGH-IJKL-MNOP"
	if err := store.ReplaceRecoveryCodes(db, userID, []string{auth.HashRecoveryCode(oldRecoveryCode)}); err != nil {
		t.Fatalf("replace recovery codes: %v", err)
	}
	cookie := createSessionCookie(t, db, userID)
	code, _ := auth.TOTPCode(secret, time.Now())

	wrong := httptest.NewRecorder()
	wrongReq := httptest.NewRequest("POST", "/api/auth/mfa/recovery/regenerate", strings.NewReader(`{"password":"pw123456","code":"000000"}`))
	wrongReq.AddCookie(cookie)
	s.Handler().ServeHTTP(wrong, wrongReq)
	if wrong.Code == http.StatusOK || strings.Contains(wrong.Body.String(), "000000") {
		t.Fatalf("wrong code accepted or echoed: %d %s", wrong.Code, wrong.Body)
	}

	first := httptest.NewRecorder()
	firstReq := httptest.NewRequest("POST", "/api/auth/mfa/recovery/regenerate", strings.NewReader(`{"password":"pw123456","code":"`+code+`"}`))
	firstReq.AddCookie(cookie)
	s.Handler().ServeHTTP(first, firstReq)
	if first.Code != http.StatusOK {
		t.Fatalf("regenerate with totp: want 200, got %d (%s)", first.Code, first.Body)
	}
	var firstBody struct {
		RecoveryCodes []string `json:"recovery_codes"`
	}
	if err := json.NewDecoder(first.Body).Decode(&firstBody); err != nil {
		t.Fatalf("decode regenerate: %v", err)
	}
	if len(firstBody.RecoveryCodes) != 10 {
		t.Fatalf("regenerate returned %d recovery codes", len(firstBody.RecoveryCodes))
	}
	if accepted, err := store.MarkRecoveryCodeUsed(db, userID, auth.HashRecoveryCode(oldRecoveryCode)); err != nil || accepted {
		t.Fatalf("old recovery code still worked: accepted=%v err=%v", accepted, err)
	}

	second := httptest.NewRecorder()
	secondReq := httptest.NewRequest("POST", "/api/auth/mfa/recovery/regenerate", strings.NewReader(`{"password":"pw123456","code":"`+firstBody.RecoveryCodes[0]+`"}`))
	secondReq.AddCookie(cookie)
	s.Handler().ServeHTTP(second, secondReq)
	if second.Code != http.StatusOK {
		t.Fatalf("regenerate with recovery code: want 200, got %d (%s)", second.Code, second.Body)
	}
	secondHashes, err := store.ListRecoveryCodeHashes(db, userID)
	if err != nil {
		t.Fatalf("list second hashes: %v", err)
	}
	for _, oldPlaintext := range firstBody.RecoveryCodes {
		oldHash := auth.HashRecoveryCode(oldPlaintext)
		for _, newHash := range secondHashes {
			if newHash == oldHash {
				t.Fatalf("regenerate retained old recovery hash for %q", oldPlaintext)
			}
		}
	}
}

func createMFAUser(t *testing.T, s *Server, db *sql.DB, email string) (string, int64) {
	t.Helper()
	key := []byte("12345678901234567890123456789012")
	s.SetMFAKey(key)
	h, err := auth.HashPassword("pw123456")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	userID, err := store.CreateUser(db, email, "MFA", h, false)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	secret, err := auth.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("generate totp secret: %v", err)
	}
	enc, err := auth.EncryptTOTPSecret(key, secret, 1)
	if err != nil {
		t.Fatalf("encrypt totp secret: %v", err)
	}
	if err := store.UpsertMFAFactor(db, store.MFAFactor{UserID: userID, SecretCiphertext: enc.Ciphertext, SecretNonce: enc.Nonce, KeyVersion: enc.KeyVersion, EnabledAt: time.Now().Unix(), LastAcceptedStep: -1}); err != nil {
		t.Fatalf("upsert mfa factor: %v", err)
	}
	return secret, userID
}

func createAuthenticatedUser(t *testing.T, s *Server, db *sql.DB, email string) (string, int64, *http.Cookie) {
	t.Helper()
	passwordHash, err := auth.HashPassword("pw123456")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	userID, err := store.CreateUser(db, email, "Authenticated", passwordHash, false)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return "pw123456", userID, createSessionCookie(t, db, userID)
}

func createSessionCookie(t *testing.T, db *sql.DB, userID int64) *http.Cookie {
	t.Helper()
	raw, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate session token: %v", err)
	}
	if err := store.CreateSession(db, auth.HashToken(raw), userID, time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return &http.Cookie{Name: sessionCookie, Value: raw}
}

func loginMFATicket(t *testing.T, s *Server, email string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	s.login(rec, httptest.NewRequest("POST", "/api/auth/login",
		strings.NewReader(`{"email":"`+email+`","password":"pw123456"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("login: want 200, got %d (%s)", rec.Code, rec.Body)
	}
	var body struct {
		Ticket string `json:"ticket"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if body.Ticket == "" {
		t.Fatal("login did not return MFA ticket")
	}
	return body.Ticket
}

func sessionCookieFrom(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == sessionCookie {
			return cookie
		}
	}
	return nil
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
