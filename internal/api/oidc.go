package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/andybarilla/exit66jukebox/internal/auth"
	"github.com/andybarilla/exit66jukebox/internal/config"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

// oidcCallbackPath is fixed rather than configurable: it is half of the
// redirect URI the operator registers with the provider, and a second way to
// spell it buys nothing but a mismatch to debug. The full URI is
// EXIT66_PUBLIC_ORIGIN + this.
const oidcCallbackPath = "/api/auth/oidc/callback"

// oidcTxCookie carries the one sign-in attempt's CSRF state, ID-token nonce and
// PKCE verifier across the round trip to the provider. All three are secrets the
// browser must hold but never read, so they ride in a single HttpOnly cookie
// rather than in the session table: an abandoned sign-in then costs nothing to
// clean up. SameSite=Lax is required, not incidental — the provider returns the
// user by top-level GET from another site, which Strict would strip the cookie
// from.
const oidcTxCookie = "exit66_oidc_tx"

const oidcTxTTL = 10 * time.Minute

// errOIDCDisabled is the answer on both OIDC routes when no provider was
// enabled — either none is configured, or SetOIDC refused one whose redirect
// URI could not be built.
var errOIDCDisabled = errors.New("oidc sign-in is not enabled")

// oidcConfig is the resolved, enabled provider. A nil *oidcConfig on the Server
// means sign-in through a provider is off, whatever the environment said.
type oidcConfig struct {
	cfg         config.OIDC
	redirectURI string
}

// SetOIDC enables OIDC sign-in from the operator's configuration. It is called
// after SetListenAddr and SetPublicOrigin, because the redirect URI is built
// from the public origin and there is no browser-facing origin to build it from
// on a non-loopback bind with the variable unset (#129).
//
// An unconfigured or partially configured provider is not an error: the feature
// is simply off. A configured provider with no resolvable public origin IS an
// error, returned so startup can say so once and leave OIDC off, rather than
// letting every sign-in attempt discover it separately.
func (s *Server) SetOIDC(cfg config.OIDC) error {
	s.oidc = nil
	if !cfg.Configured() {
		return nil
	}
	base, err := s.remoteBaseURL()
	if err != nil {
		return err
	}
	s.oidc = &oidcConfig{cfg: cfg, redirectURI: base + oidcCallbackPath}
	return nil
}

// oidcEnabled reports whether the sign-in surface should offer the provider.
func (s *Server) oidcEnabled() bool { return s.oidc != nil }

// oidcButtonLabel is what the sign-in surface calls the provider; empty when
// OIDC is off, so the frontend has nothing to render a button from.
func (s *Server) oidcButtonLabel() string {
	if s.oidc == nil {
		return ""
	}
	return s.oidc.cfg.ButtonLabel
}

// oidcProvider returns the discovered provider, fetching (and then caching) the
// discovery document on first use. Discovery is deliberately not done at
// startup: the identity provider being briefly unreachable should delay OIDC
// sign-in, not stop the jukebox from booting.
func (s *Server) oidcProvider(ctx context.Context) (*oidc.Provider, error) {
	if s.oidc == nil {
		return nil, errOIDCDisabled
	}
	s.oidcMu.Lock()
	defer s.oidcMu.Unlock()
	if s.oidcDiscovered != nil {
		return s.oidcDiscovered, nil
	}
	p, err := oidc.NewProvider(ctx, s.oidc.cfg.Issuer)
	if err != nil {
		return nil, err
	}
	s.oidcDiscovered = p
	return p, nil
}

func (s *Server) oidcOAuth2Config(p *oidc.Provider) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     s.oidc.cfg.ClientID,
		ClientSecret: s.oidc.cfg.ClientSecret,
		Endpoint:     p.Endpoint(),
		RedirectURL:  s.oidc.redirectURI,
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}
}

// oidcStart sends the browser to the provider. state, nonce and a PKCE verifier
// are minted here and kept only in the transaction cookie, so the callback can
// tell its own redirect apart from one an attacker induced.
func (s *Server) oidcStart(w http.ResponseWriter, r *http.Request) {
	if !s.oidcEnabled() {
		writeErr(w, http.StatusNotFound, errOIDCDisabled.Error())
		return
	}
	provider, err := s.oidcProvider(r.Context())
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "oidc provider is unreachable")
		return
	}
	state, err := auth.GenerateToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token error")
		return
	}
	nonce, err := auth.GenerateToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token error")
		return
	}
	verifier := oauth2.GenerateVerifier()
	http.SetCookie(w, &http.Cookie{
		Name:     oidcTxCookie,
		Value:    strings.Join([]string{state, nonce, verifier}, "."),
		Path:     "/api/auth/oidc/",
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(oidcTxTTL / time.Second),
	})
	authURL := s.oidcOAuth2Config(provider).AuthCodeURL(state,
		oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier))
	http.Redirect(w, r, authURL, http.StatusFound)
}

// oidcTx is the state/nonce/verifier triple read back from the cookie.
type oidcTx struct{ state, nonce, verifier string }

func readOIDCTx(r *http.Request) (oidcTx, bool) {
	c, err := r.Cookie(oidcTxCookie)
	if err != nil || c.Value == "" {
		return oidcTx{}, false
	}
	// "." separates the three because none of them can contain one: state and
	// nonce are hex, and the PKCE verifier is base64url.
	parts := strings.Split(c.Value, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return oidcTx{}, false
	}
	return oidcTx{state: parts[0], nonce: parts[1], verifier: parts[2]}, true
}

func clearOIDCTx(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: oidcTxCookie, Value: "", Path: "/api/auth/oidc/", HttpOnly: true,
		Secure: isHTTPS(r), SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}

// oidcCallback completes the sign-in: verifies state against the cookie,
// exchanges the code, validates the ID token through the library (signature,
// issuer, audience, expiry) plus the nonce, then applies the linking rules.
//
// It always answers with a redirect to the SPA rather than JSON: the browser
// arrives here by navigation, so a JSON body would leave the user staring at
// raw text. Failures carry ?oidc_error=<reason> for the sign-in surface to show.
func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	if !s.oidcEnabled() {
		writeErr(w, http.StatusNotFound, errOIDCDisabled.Error())
		return
	}
	tx, ok := readOIDCTx(r)
	clearOIDCTx(w, r)
	// The state check comes before anything that costs a round trip, so an
	// unsolicited callback can't make this server talk to the provider.
	if !ok || r.URL.Query().Get("state") == "" || r.URL.Query().Get("state") != tx.state {
		s.oidcFail(w, r, "state")
		return
	}
	if r.URL.Query().Get("error") != "" {
		s.oidcFail(w, r, "provider")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		s.oidcFail(w, r, "provider")
		return
	}
	// Everything past here costs a request to the provider, and a caller can
	// mint its own state by calling /start first. Throttle on the same sliding
	// window password login uses so this can't be looped into traffic at the
	// identity provider.
	if !s.allowAttempt("oidc-ip:" + clientIP(r)) {
		s.oidcFail(w, r, "throttled")
		return
	}
	provider, err := s.oidcProvider(r.Context())
	if err != nil {
		s.oidcFail(w, r, "unreachable")
		return
	}
	token, err := s.oidcOAuth2Config(provider).Exchange(r.Context(), code, oauth2.VerifierOption(tx.verifier))
	if err != nil {
		s.oidcFail(w, r, "exchange")
		return
	}
	rawID, ok := token.Extra("id_token").(string)
	if !ok || rawID == "" {
		s.oidcFail(w, r, "token")
		return
	}
	idToken, err := provider.Verifier(&oidc.Config{ClientID: s.oidc.cfg.ClientID}).Verify(r.Context(), rawID)
	if err != nil {
		s.oidcFail(w, r, "token")
		return
	}
	if idToken.Nonce != tx.nonce {
		s.oidcFail(w, r, "token")
		return
	}
	var claims struct {
		Email    string `json:"email"`
		Verified bool   `json:"email_verified"`
		Name     string `json:"name"`
	}
	if err := idToken.Claims(&claims); err != nil {
		s.oidcFail(w, r, "token")
		return
	}
	user, reason := s.oidcResolveUser(idToken.Issuer, idToken.Subject,
		strings.TrimSpace(strings.ToLower(claims.Email)), claims.Verified, strings.TrimSpace(claims.Name))
	if reason != "" {
		s.oidcFail(w, r, reason)
		return
	}
	s.oidcFinishSignIn(w, r, user)
}

// oidcResolveUser applies the account-linking rules and returns the account to
// sign in, or a failure reason for the SPA.
//
//   - A known issuer+subject is that account, full stop. The email the provider
//     asserts is not consulted, so an address changing hands there neither
//     orphans the link nor moves it.
//   - An unknown identity whose email already has a local account is REFUSED.
//     Auto-linking on a matching address would hand that account to whoever the
//     provider currently lets sign in as it. Linking an existing account is a
//     deliberate act for its owner, and there is no UI for it yet.
//   - An unknown identity with no matching account may create one, and only
//     then. All four conditions hold: the provider asserts a verified email, at
//     least one account already exists, the mode is full_login, and the signup
//     toggle is on — the last two being exactly self-service signup's own gates.
//     The new account is never an admin and never a passwordless profile, and it
//     carries no usable password.
func (s *Server) oidcResolveUser(issuer, subject, email string, emailVerified bool, name string) (store.User, string) {
	if subject == "" {
		return store.User{}, "token"
	}
	user, ok, err := store.UserByOIDCIdentity(s.db, issuer, subject)
	if err != nil {
		return store.User{}, "server"
	}
	if ok {
		return user, ""
	}
	// Every path below creates an account, so an address is mandatory and the
	// provider must vouch for it: an unverified one could be anybody's, and the
	// row it creates would hold that address against its real owner forever.
	if email == "" || !emailVerified {
		return store.User{}, "email_unverified"
	}
	if _, exists, err := store.GetUserByEmail(s.db, email); err != nil {
		return store.User{}, "server"
	} else if exists {
		return store.User{}, "email_taken"
	}
	// The first account has to be the bootstrap admin. Creating a non-admin one
	// here would disarm bootstrap (it keys on an empty user table) and leave the
	// install with no administrator and no way to make one.
	if n, err := store.CountUsers(s.db); err != nil {
		return store.User{}, "server"
	} else if n == 0 {
		return store.User{}, "no_signup"
	}
	if store.SecurityModeSetting(s.db) != store.SecurityModeFullLogin || !store.SignupEnabled(s.db) {
		return store.User{}, "no_signup"
	}
	uid, err := store.CreateUser(s.db, email, name, store.OIDCPasswordHash, false, true)
	if err != nil {
		return store.User{}, "server"
	}
	if err := store.LinkOIDCIdentity(s.db, issuer, subject, uid); err != nil {
		// The account without its identity would be unreachable — it has no
		// usable password — and would hold the address. Take it back out.
		store.DeleteUser(s.db, uid)
		return store.User{}, "server"
	}
	created, ok, err := store.GetUserByID(s.db, uid)
	if err != nil || !ok {
		return store.User{}, "server"
	}
	return created, ""
}

// oidcFinishSignIn issues the session through the same path a password login
// uses, so cookie flags, expiry and every security-mode gate behave identically
// afterwards. An account with TOTP enabled still has to present it: the ticket
// goes to the sign-in surface in the redirect, which then posts it to the
// existing /api/auth/mfa/complete. The ticket alone is not a credential — it is
// single-use, expires in five minutes, and is useless without the second factor.
func (s *Server) oidcFinishSignIn(w http.ResponseWriter, r *http.Request, user store.User) {
	factor, hasFactor, err := store.GetMFAFactor(s.db, user.ID)
	if err != nil {
		s.oidcFail(w, r, "server")
		return
	}
	if hasFactor && factor.EnabledAt > 0 {
		ticket, err := auth.GenerateToken()
		if err != nil {
			s.oidcFail(w, r, "server")
			return
		}
		if err := store.CreateMFATicket(s.db, auth.HashToken(ticket), user.ID, time.Now().Add(mfaTicketTTL).Unix()); err != nil {
			s.oidcFail(w, r, "server")
			return
		}
		http.Redirect(w, r, "/?oidc_mfa="+url.QueryEscape(ticket), http.StatusFound)
		return
	}
	if err := s.setSessionCookie(w, r, user.ID); err != nil {
		s.oidcFail(w, r, "server")
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

// oidcFail sends the browser back to the sign-in surface with a reason code.
// The codes are a closed vocabulary the frontend maps to text; nothing the
// provider said is echoed into the URL.
func (s *Server) oidcFail(w http.ResponseWriter, r *http.Request, reason string) {
	http.Redirect(w, r, "/?oidc_error="+url.QueryEscape(reason), http.StatusFound)
}
