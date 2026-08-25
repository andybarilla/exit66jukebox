package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/auth"
	"github.com/andybarilla/exit66jukebox/internal/config"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

const (
	testOIDCClientID = "jukebox-client"
	testOIDCSecret   = "jukebox-secret"
	testPublicOrigin = "https://jukebox.example.com"
)

func testOIDCConfig(issuer string) config.OIDC {
	return config.OIDC{
		Issuer:       issuer,
		ClientID:     testOIDCClientID,
		ClientSecret: testOIDCSecret,
		ButtonLabel:  "Corp SSO",
	}
}

// newOIDCServer is a server with a public origin and an enabled provider,
// standing in for a normal install that has configured single sign-on.
func newOIDCServer(t *testing.T) (*Server, *sql.DB, *fakeIDP) {
	t.Helper()
	s, db := newTestServer(t)
	idp := newFakeIDP(t)
	s.SetPublicOrigin(testPublicOrigin)
	if err := s.SetOIDC(testOIDCConfig(idp.issuer)); err != nil {
		t.Fatalf("SetOIDC: %v", err)
	}
	return s, db, idp
}

// allowSignup puts the instance in the one state that lets a first-time OIDC
// sign-in create an account: full_login with self-service signup on, and at
// least one account already present so bootstrap is not at stake.
func allowSignup(t *testing.T, db *sql.DB) {
	t.Helper()
	if err := store.SetSecurityMode(db, store.SecurityModeFullLogin); err != nil {
		t.Fatal(err)
	}
	if err := store.SetSignupEnabled(db, true); err != nil {
		t.Fatal(err)
	}
	hash, _ := auth.HashPassword("adminpassword")
	if _, err := store.CreateUser(db, "admin@example.com", "Admin", hash, true, true); err != nil {
		t.Fatal(err)
	}
}

// signIn drives one complete sign-in: /start to mint the transaction, then the
// provider's callback carrying the state the server just issued.
func signIn(t *testing.T, s *Server, idp *fakeIDP) *httptest.ResponseRecorder {
	t.Helper()
	start := httptest.NewRecorder()
	s.oidcStart(start, httptest.NewRequest(http.MethodGet, "/api/auth/oidc/start", nil))
	if start.Code != http.StatusFound {
		t.Fatalf("start: want 302, got %d (%s)", start.Code, start.Body)
	}
	authURL, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	idp.nonce = authURL.Query().Get("nonce")
	tx := mustCookie(t, start.Result(), oidcTxCookie)

	req := httptest.NewRequest(http.MethodGet,
		oidcCallbackPath+"?code=auth-code&state="+url.QueryEscape(authURL.Query().Get("state")), nil)
	req.AddCookie(tx)
	rec := httptest.NewRecorder()
	s.oidcCallback(rec, req)
	return rec
}

// sessionUser resolves the session cookie a response set back to its account,
// which is how "signed in as" is asserted throughout this file.
func sessionUser(t *testing.T, db *sql.DB, rec *httptest.ResponseRecorder) (store.User, bool) {
	t.Helper()
	c := cookieNamed(rec.Result(), sessionCookie)
	if c == nil {
		return store.User{}, false
	}
	u, ok, err := store.UserBySession(db, auth.HashToken(c.Value))
	if err != nil {
		t.Fatal(err)
	}
	return u, ok
}

func TestOIDCOffWithNoConfiguration(t *testing.T) {
	s, _ := newTestServer(t)
	if err := s.SetOIDC(config.OIDC{ButtonLabel: config.DefaultOIDCButtonLabel}); err != nil {
		t.Fatalf("an unconfigured provider is not an error: %v", err)
	}
	if s.oidcEnabled() {
		t.Fatal("OIDC enabled with no issuer, client id or secret")
	}
	for _, path := range []string{"/api/auth/oidc/start", oidcCallbackPath} {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s with OIDC off: want 404, got %d (%s)", path, rec.Code, rec.Body)
		}
	}
	cfg := httptest.NewRecorder()
	s.getConfig(cfg, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	var got map[string]any
	if err := json.Unmarshal(cfg.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["oidc_enabled"] != false || got["oidc_name"] != "" {
		t.Errorf("config advertises OIDC while off: enabled=%v name=%v", got["oidc_enabled"], got["oidc_name"])
	}
}

// A partially filled environment must leave password login alone rather than
// half-enabling a provider that can never complete a sign-in.
func TestOIDCPartialConfigurationStaysOff(t *testing.T) {
	for _, missing := range []string{"issuer", "client id", "client secret"} {
		s, _ := newTestServer(t)
		s.SetPublicOrigin(testPublicOrigin)
		cfg := testOIDCConfig("https://idp.example")
		switch missing {
		case "issuer":
			cfg.Issuer = ""
		case "client id":
			cfg.ClientID = ""
		case "client secret":
			cfg.ClientSecret = ""
		}
		if err := s.SetOIDC(cfg); err != nil {
			t.Errorf("missing %s: SetOIDC returned %v, want nil", missing, err)
		}
		if s.oidcEnabled() {
			t.Errorf("missing %s: OIDC enabled anyway", missing)
		}
	}
}

// #129's rule applies to the redirect URI as much as to an emailed link: with no
// public origin and a non-loopback bind there is no browser-facing origin to
// build one from, so the provider is refused at startup rather than at sign-in.
func TestOIDCRefusedWithoutPublicOriginOnWildcardBind(t *testing.T) {
	s, _ := newTestServer(t)
	s.SetListenAddr(":8066") // the default: a wildcard, which is not loopback
	err := s.SetOIDC(testOIDCConfig("https://idp.example"))
	if !errors.Is(err, errPublicOriginUnset) {
		t.Fatalf("SetOIDC on a wildcard bind = %v, want errPublicOriginUnset", err)
	}
	if s.oidcEnabled() {
		t.Fatal("OIDC left enabled with no resolvable redirect URI")
	}
}

func TestOIDCRedirectURIComesFromPublicOrigin(t *testing.T) {
	s, _ := newTestServer(t)
	s.SetListenAddr(":8066")
	s.SetPublicOrigin(testPublicOrigin + "/")
	if err := s.SetOIDC(testOIDCConfig("https://idp.example")); err != nil {
		t.Fatal(err)
	}
	if want := testPublicOrigin + oidcCallbackPath; s.oidc.redirectURI != want {
		t.Fatalf("redirect URI = %q, want %q", s.oidc.redirectURI, want)
	}
}

func TestOIDCStartSendsStateNoncePKCEInACookieBoundRequest(t *testing.T) {
	s, _, idp := newOIDCServer(t)
	rec := httptest.NewRecorder()
	s.oidcStart(rec, httptest.NewRequest(http.MethodGet, "/api/auth/oidc/start", nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("want 302, got %d (%s)", rec.Code, rec.Body)
	}
	authURL, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := authURL.Scheme+"://"+authURL.Host+authURL.Path, idp.issuer+"/authorize"; got != want {
		t.Errorf("authorization endpoint = %q, want %q", got, want)
	}
	q := authURL.Query()
	if q.Get("client_id") != testOIDCClientID {
		t.Errorf("client_id = %q", q.Get("client_id"))
	}
	if q.Get("redirect_uri") != testPublicOrigin+oidcCallbackPath {
		t.Errorf("redirect_uri = %q", q.Get("redirect_uri"))
	}
	if !strings.Contains(q.Get("scope"), "openid") {
		t.Errorf("scope = %q, want it to include openid", q.Get("scope"))
	}
	if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
		t.Errorf("PKCE missing: method=%q challenge=%q", q.Get("code_challenge_method"), q.Get("code_challenge"))
	}

	tx := mustCookie(t, rec.Result(), oidcTxCookie)
	if !tx.HttpOnly || tx.SameSite != http.SameSiteLaxMode {
		t.Errorf("transaction cookie must be HttpOnly and SameSite=Lax, got HttpOnly=%v SameSite=%v", tx.HttpOnly, tx.SameSite)
	}
	parts := strings.Split(tx.Value, ".")
	if len(parts) != 3 {
		t.Fatalf("transaction cookie = %q, want state.nonce.verifier", tx.Value)
	}
	if parts[0] != q.Get("state") {
		t.Errorf("cookie state %q does not match the state sent to the provider %q", parts[0], q.Get("state"))
	}
	if parts[1] != q.Get("nonce") {
		t.Errorf("cookie nonce %q does not match the nonce sent to the provider %q", parts[1], q.Get("nonce"))
	}
	if !verifierMatchesChallenge(parts[2], q.Get("code_challenge")) {
		t.Errorf("cookie verifier %q does not hash to challenge %q", parts[2], q.Get("code_challenge"))
	}
}

func TestOIDCFirstSignInCreatesLinkedAccountAndSession(t *testing.T) {
	s, db, idp := newOIDCServer(t)
	allowSignup(t, db)

	rec := signIn(t, s, idp)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/" {
		t.Fatalf("callback = %d %q, want 302 /", rec.Code, rec.Header().Get("Location"))
	}
	user, ok := sessionUser(t, db, rec)
	if !ok {
		t.Fatal("callback issued no usable session")
	}
	if user.Email != idp.email || user.IsAdmin || user.IsPasswordlessProfile {
		t.Fatalf("created account = %+v; want email %s, not admin, not a passwordless profile", user, idp.email)
	}
	if user.EmailVerifiedAt == 0 {
		t.Error("provider-verified address did not land as verified, so login would refuse it")
	}
	linked, ok, err := store.UserByOIDCIdentity(db, idp.issuer, idp.subject)
	if err != nil || !ok || linked.ID != user.ID {
		t.Fatalf("identity link: ok=%v err=%v id=%d want %d", ok, err, linked.ID, user.ID)
	}
	// The account has no password, and the reset flow is its only route to one.
	if auth.VerifyPassword("", user.PasswordHash) || auth.VerifyPassword(store.OIDCPasswordHash, user.PasswordHash) {
		t.Error("an OIDC-created account accepted a password")
	}
	if idp.gotVerifier == "" {
		t.Error("token exchange sent no PKCE verifier")
	}
	if idp.gotClientID != testOIDCClientID || idp.gotClientSecret != testOIDCSecret {
		t.Errorf("token exchange authenticated as %q/%q", idp.gotClientID, idp.gotClientSecret)
	}
}

// The session an OIDC sign-in issues must be the same object a password login
// issues — same cookie flags, same lifetime, same session row.
func TestOIDCSessionMatchesPasswordLoginSession(t *testing.T) {
	s, db, idp := newOIDCServer(t)
	allowSignup(t, db)
	hash, _ := auth.HashPassword("localpassword")
	store.CreateUser(db, "local@example.com", "Local", hash, false, true)

	pw := httptest.NewRecorder()
	s.login(pw, httptest.NewRequest(http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"email":"local@example.com","password":"localpassword"}`)))
	if pw.Code != http.StatusOK {
		t.Fatalf("password login: %d (%s)", pw.Code, pw.Body)
	}
	pwCookie := mustCookie(t, pw.Result(), sessionCookie)
	ssoCookie := mustCookie(t, signIn(t, s, idp).Result(), sessionCookie)

	if pwCookie.HttpOnly != ssoCookie.HttpOnly || pwCookie.Secure != ssoCookie.Secure ||
		pwCookie.SameSite != ssoCookie.SameSite || pwCookie.Path != ssoCookie.Path {
		t.Errorf("session cookie flags differ:\npassword: %+v\noidc:     %+v", pwCookie, ssoCookie)
	}
	if delta := ssoCookie.Expires.Sub(pwCookie.Expires); delta > time.Minute || delta < -time.Minute {
		t.Errorf("session expiry differs by %v", delta)
	}
}

// The identity is keyed on subject, so the provider renaming a user's address
// signs the same account in rather than orphaning it or making a second one.
func TestOIDCKnownIdentityIgnoresChangedEmail(t *testing.T) {
	s, db, idp := newOIDCServer(t)
	hash, _ := auth.HashPassword("localpassword")
	uid, _ := store.CreateUser(db, "original@example.com", "Original", hash, false, true)
	if err := store.LinkOIDCIdentity(db, idp.issuer, idp.subject, uid); err != nil {
		t.Fatal(err)
	}
	before, _ := store.CountUsers(db)
	idp.email = "renamed@example.com"

	rec := signIn(t, s, idp)
	user, ok := sessionUser(t, db, rec)
	if !ok || user.ID != uid {
		t.Fatalf("signed in as %d (ok=%v), want the linked account %d", user.ID, ok, uid)
	}
	if user.Email != "original@example.com" {
		t.Errorf("local email changed to %q; the provider does not own it", user.Email)
	}
	if after, _ := store.CountUsers(db); after != before {
		t.Errorf("account count %d → %d: a duplicate was created", before, after)
	}
}

// The rule with the most security in it: an email the provider asserts is not
// proof of ownership of the local account already holding that address.
func TestOIDCRefusesToClaimAnExistingLocalAccountByEmail(t *testing.T) {
	s, db, idp := newOIDCServer(t)
	allowSignup(t, db)
	hash, _ := auth.HashPassword("localpassword")
	victim, _ := store.CreateUser(db, idp.email, "Victim", hash, true, true)

	rec := signIn(t, s, idp)
	if got := redirectQuery(t, rec).Get("oidc_error"); got != "email_taken" {
		t.Fatalf("oidc_error = %q, want email_taken", got)
	}
	if _, ok := sessionUser(t, db, rec); ok {
		t.Fatal("a session was issued for an account the provider merely named")
	}
	if _, linked, _ := store.UserByOIDCIdentity(db, idp.issuer, idp.subject); linked {
		t.Fatal("the identity was linked to the existing account anyway")
	}
	// And the account it tried to claim is untouched.
	u, _, _ := store.GetUserByID(db, victim)
	if !auth.VerifyPassword("localpassword", u.PasswordHash) {
		t.Error("the existing account's password no longer works")
	}
}

func TestOIDCAccountCreationObeysTheSignupGates(t *testing.T) {
	cases := []struct {
		name       string
		setup      func(*testing.T, *sql.DB)
		wantCreate bool
	}{
		{
			name:       "full_login with signup on",
			setup:      allowSignup,
			wantCreate: true,
		},
		{
			name: "signup toggle off",
			setup: func(t *testing.T, db *sql.DB) {
				allowSignup(t, db)
				if err := store.SetSignupEnabled(db, false); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "not full_login",
			setup: func(t *testing.T, db *sql.DB) {
				allowSignup(t, db)
				if err := store.SetSecurityMode(db, store.SecurityModeHouseholdProfiles); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			// An empty user table means the first admin has not been claimed.
			// Creating a non-admin here would disarm bootstrap and leave the
			// install with no administrator.
			name: "no accounts yet",
			setup: func(t *testing.T, db *sql.DB) {
				if err := store.SetSecurityMode(db, store.SecurityModeFullLogin); err != nil {
					t.Fatal(err)
				}
				if err := store.SetSignupEnabled(db, true); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, db, idp := newOIDCServer(t)
			tc.setup(t, db)
			before, _ := store.CountUsers(db)

			rec := signIn(t, s, idp)
			_, signedIn := sessionUser(t, db, rec)
			after, _ := store.CountUsers(db)
			if tc.wantCreate {
				if !signedIn || after != before+1 {
					t.Fatalf("want an account created and signed in; signedIn=%v accounts %d → %d", signedIn, before, after)
				}
				return
			}
			if signedIn {
				t.Error("a session was issued where signup is not allowed")
			}
			if after != before {
				t.Errorf("account count %d → %d; none should have been created", before, after)
			}
			if got := redirectQuery(t, rec).Get("oidc_error"); got != "no_signup" {
				t.Errorf("oidc_error = %q, want no_signup", got)
			}
		})
	}
}

// An address the provider will not vouch for could be anyone's, and the account
// it creates would hold that address against its real owner.
func TestOIDCRefusesUnvouchedEmail(t *testing.T) {
	for _, tc := range []struct {
		name, email string
		verified    bool
	}{
		{name: "unverified", email: "sso@example.com"},
		{name: "absent", verified: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, db, idp := newOIDCServer(t)
			allowSignup(t, db)
			idp.email, idp.emailVerified = tc.email, tc.verified
			before, _ := store.CountUsers(db)

			rec := signIn(t, s, idp)
			if got := redirectQuery(t, rec).Get("oidc_error"); got != "email_unverified" {
				t.Errorf("oidc_error = %q, want email_unverified", got)
			}
			if _, ok := sessionUser(t, db, rec); ok {
				t.Error("a session was issued for an address the provider did not vouch for")
			}
			if after, _ := store.CountUsers(db); after != before {
				t.Errorf("account count %d → %d; none should have been created", before, after)
			}
		})
	}
}

// Signature, issuer, audience and expiry are each verified, and each failure
// alone is enough to refuse the sign-in.
func TestOIDCRejectsInvalidIDTokens(t *testing.T) {
	cases := []struct {
		name   string
		break_ func(*fakeIDP)
	}{
		{"bad signature", func(f *fakeIDP) { f.signWrong = true }},
		{"wrong issuer", func(f *fakeIDP) { f.issuerClaim = "https://evil.example" }},
		{"wrong audience", func(f *fakeIDP) { f.audience = "some-other-client" }},
		{"expired", func(f *fakeIDP) { f.expiry = time.Now().Add(-time.Hour) }},
		{"replayed nonce", func(f *fakeIDP) { f.nonce = "not-the-nonce-we-sent" }},
		{"no subject", func(f *fakeIDP) { f.subject = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, db, idp := newOIDCServer(t)
			allowSignup(t, db)
			before, _ := store.CountUsers(db)

			// signIn sets the nonce from the authorization request; a case that
			// wants a different one overrides it after, hence the ordering here.
			start := httptest.NewRecorder()
			s.oidcStart(start, httptest.NewRequest(http.MethodGet, "/api/auth/oidc/start", nil))
			authURL, _ := url.Parse(start.Header().Get("Location"))
			idp.nonce = authURL.Query().Get("nonce")
			tc.break_(idp)

			req := httptest.NewRequest(http.MethodGet,
				oidcCallbackPath+"?code=auth-code&state="+url.QueryEscape(authURL.Query().Get("state")), nil)
			req.AddCookie(mustCookie(t, start.Result(), oidcTxCookie))
			rec := httptest.NewRecorder()
			s.oidcCallback(rec, req)

			if _, ok := sessionUser(t, db, rec); ok {
				t.Fatal("a session was issued for an invalid ID token")
			}
			if got := redirectQuery(t, rec).Get("oidc_error"); got != "token" {
				t.Errorf("oidc_error = %q, want token", got)
			}
			if after, _ := store.CountUsers(db); after != before {
				t.Errorf("account count %d → %d; an invalid token created an account", before, after)
			}
		})
	}
}

// A callback the browser did not start — no cookie, no state, or a state that
// does not match the cookie — is refused before the server talks to anyone.
func TestOIDCRejectsUnsolicitedCallbacks(t *testing.T) {
	cases := []struct {
		name, query string
		withCookie  bool
	}{
		{name: "no state, no cookie", query: "?code=auth-code"},
		{name: "state without a cookie", query: "?code=auth-code&state=guessed"},
		{name: "cookie without a state", query: "?code=auth-code", withCookie: true},
		{name: "state does not match the cookie", query: "?code=auth-code&state=guessed", withCookie: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, db, idp := newOIDCServer(t)
			allowSignup(t, db)
			start := httptest.NewRecorder()
			s.oidcStart(start, httptest.NewRequest(http.MethodGet, "/api/auth/oidc/start", nil))
			authURL, _ := url.Parse(start.Header().Get("Location"))
			idp.nonce = authURL.Query().Get("nonce")

			req := httptest.NewRequest(http.MethodGet, oidcCallbackPath+tc.query, nil)
			if tc.withCookie {
				req.AddCookie(mustCookie(t, start.Result(), oidcTxCookie))
			}
			rec := httptest.NewRecorder()
			s.oidcCallback(rec, req)

			if _, ok := sessionUser(t, db, rec); ok {
				t.Fatal("an unsolicited callback produced a session")
			}
			if got := redirectQuery(t, rec).Get("oidc_error"); got != "state" {
				t.Errorf("oidc_error = %q, want state", got)
			}
		})
	}
}

// The transaction cookie is single-use: replaying the same callback after the
// first one cleared it must not sign anyone in again.
func TestOIDCTransactionCookieIsClearedOnCallback(t *testing.T) {
	s, db, idp := newOIDCServer(t)
	allowSignup(t, db)
	rec := signIn(t, s, idp)
	if _, ok := sessionUser(t, db, rec); !ok {
		t.Fatal("setup: sign-in did not succeed")
	}
	tx := mustCookie(t, rec.Result(), oidcTxCookie)
	if tx.MaxAge >= 0 || tx.Value != "" {
		t.Fatalf("transaction cookie not expired on callback: %+v", tx)
	}
}

// TOTP is not bypassed by signing in through the provider: the user gets the
// same MFA ticket a password login gets, and no session until they complete it.
func TestOIDCHonoursEnabledMFA(t *testing.T) {
	s, db, idp := newOIDCServer(t)
	allowSignup(t, db)
	hash, _ := auth.HashPassword("localpassword")
	uid, _ := store.CreateUser(db, idp.email, "MFA User", hash, false, true)
	if err := store.LinkOIDCIdentity(db, idp.issuer, idp.subject, uid); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertMFAFactor(db, store.MFAFactor{
		UserID: uid, SecretCiphertext: []byte("c"), SecretNonce: []byte("n"),
		KeyVersion: 1, EnabledAt: time.Now().Unix(), LastAcceptedStep: -1,
	}); err != nil {
		t.Fatal(err)
	}

	rec := signIn(t, s, idp)
	if _, ok := sessionUser(t, db, rec); ok {
		t.Fatal("MFA was bypassed: the callback issued a session outright")
	}
	ticket := redirectQuery(t, rec).Get("oidc_mfa")
	if ticket == "" {
		t.Fatal("no MFA ticket handed to the sign-in surface")
	}
	got, ok, err := store.ConsumeMFATicket(db, auth.HashToken(ticket))
	if err != nil || !ok || got != uid {
		t.Fatalf("ticket does not resolve to the user: id=%d ok=%v err=%v", got, ok, err)
	}
}

// A caller can mint its own state by calling /start, so the callback must be
// throttled or it can be looped into traffic aimed at the identity provider.
func TestOIDCCallbackIsThrottled(t *testing.T) {
	s, db, idp := newOIDCServer(t)
	allowSignup(t, db)
	var last *httptest.ResponseRecorder
	for i := 0; i < 12; i++ {
		last = signIn(t, s, idp)
	}
	if got := redirectQuery(t, last).Get("oidc_error"); got != "throttled" {
		t.Fatalf("oidc_error after 12 callbacks = %q, want throttled", got)
	}
	if _, ok := sessionUser(t, db, last); ok {
		t.Error("a throttled callback still issued a session")
	}
}

// Both routes must be reachable without a session, or the sign-in they exist to
// perform can never start.
func TestOIDCRoutesAreOpenToAnonymousCallers(t *testing.T) {
	for _, p := range []string{"/api/auth/oidc/start", oidcCallbackPath} {
		if !isOpenPath(p) {
			t.Errorf("%s is behind the auth middleware, so sign-in can never begin", p)
		}
	}
}

func TestOIDCConfigAdvertisesTheProviderWhenEnabled(t *testing.T) {
	s, _, _ := newOIDCServer(t)
	rec := httptest.NewRecorder()
	s.getConfig(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["oidc_enabled"] != true || got["oidc_name"] != "Corp SSO" {
		t.Fatalf("config = enabled:%v name:%v, want true and the configured label", got["oidc_enabled"], got["oidc_name"])
	}
}
