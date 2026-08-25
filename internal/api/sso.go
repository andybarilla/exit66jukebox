package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/andybarilla/exit66jukebox/internal/auth"
	"github.com/andybarilla/exit66jukebox/internal/config"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

// ssoCallbackPath is shared by every provider and is fixed rather than
// configurable: it is half of the redirect URI the operator registers, and a
// second way to spell it buys nothing but a mismatch to debug. The full URI is
// EXIT66_PUBLIC_ORIGIN + this.
//
// One path for all providers, with which provider is in flight carried in the
// transaction cookie: the operator registers a single redirect URI whatever
// they enable, and the provider a callback is completed against is one this
// server minted rather than one the caller named in the URL.
const ssoCallbackPath = "/api/auth/sso/callback"

// ssoStartPath is where the sign-in surface sends the browser to begin. The
// provider id is a path segment so each button has its own address.
func ssoStartPath(id string) string { return "/api/auth/sso/" + id + "/start" }

// ssoCookiePath scopes the transaction cookie to the sign-in routes. It has to
// cover both the per-provider start paths and the shared callback.
const ssoCookiePath = "/api/auth/sso/"

// ssoTxCookie carries the one sign-in attempt's provider, CSRF state, ID-token
// nonce and PKCE verifier across the round trip to the provider. All are values
// the browser must hold but never read, so they ride in a single HttpOnly
// cookie rather than in the session table: an abandoned sign-in then costs
// nothing to clean up. SameSite=Lax is required, not incidental — the provider
// returns the user by top-level GET from another site, which Strict would strip
// the cookie from.
const ssoTxCookie = "exit66_sso_tx"

const ssoTxTTL = 10 * time.Minute

// errSSODisabled is the answer on the sign-in routes when the named provider is
// not enabled — either none is configured, or SetSSO refused them all because
// the redirect URI could not be built.
var errSSODisabled = errors.New("sso sign-in is not enabled")

// ssoProvider is one resolved, enabled provider.
//
// The three methods below are the whole of what differs between providers, and
// they are where a provider that is not OpenID Connect plugs in (#181, GitHub:
// plain OAuth2 with no ID token). It would carry literal endpoints instead of
// discovering them, ask for no nonce, and build its ssoIdentity from the
// provider's user API rather than from a verified token. Everything else — the
// transaction cookie, the state check, PKCE, the throttle and every one of the
// account-linking rules — is provider-agnostic already.
type ssoProvider struct {
	cfg         config.Provider
	redirectURI string

	// discovered caches the OpenID discovery document, fetched on first sign-in
	// rather than at startup and guarded by mu. Per provider, so one unreachable
	// identity provider does not stall sign-in through another.
	mu         sync.Mutex
	discovered *oidc.Provider
}

// ssoIdentity is what a completed sign-in says about the person signing in.
// Issuer and Subject are the key the local account hangs off; the rest is only
// ever consulted when creating one.
type ssoIdentity struct {
	Issuer        string
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
}

// SetSSO enables sign-in through the configured providers. It is called after
// SetListenAddr and SetPublicOrigin, because the redirect URI is built from the
// public origin and there is no browser-facing origin to build it from on a
// non-loopback bind with the variable unset (#129).
//
// No configured provider is not an error: the feature is simply off. A
// configured provider with no resolvable public origin IS an error, returned so
// startup can say so once and leave the feature off, rather than letting every
// sign-in attempt discover it separately. The origin is the same for every
// provider, so this is all-or-nothing by construction.
func (s *Server) SetSSO(providers []config.Provider) error {
	s.sso = nil
	s.ssoByID = nil
	if len(providers) == 0 {
		return nil
	}
	base, err := s.remoteBaseURL()
	if err != nil {
		return err
	}
	s.ssoByID = make(map[string]*ssoProvider, len(providers))
	for _, cfg := range providers {
		p := &ssoProvider{cfg: cfg, redirectURI: base + ssoCallbackPath}
		s.sso = append(s.sso, p)
		s.ssoByID[cfg.ID] = p
	}
	return nil
}

// ssoButtons is what /api/config advertises: one entry per enabled provider, in
// the order the sign-in surface should offer them. Always non-nil so the JSON
// carries [] rather than null.
func (s *Server) ssoButtons() []map[string]string {
	out := make([]map[string]string, 0, len(s.sso))
	for _, p := range s.sso {
		out = append(out, map[string]string{"id": p.cfg.ID, "name": p.cfg.ButtonLabel})
	}
	return out
}

// discover fetches (and then caches) the provider's discovery document.
// Discovery is deliberately not done at startup: the identity provider being
// briefly unreachable should delay sign-in through it, not stop the jukebox
// from booting.
func (p *ssoProvider) discover(ctx context.Context) (*oidc.Provider, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.discovered != nil {
		return p.discovered, nil
	}
	got, err := oidc.NewProvider(ctx, p.cfg.Issuer)
	if err != nil {
		return nil, err
	}
	p.discovered = got
	return got, nil
}

// oauth2Config resolves the endpoints and credentials for both legs of the
// exchange.
func (p *ssoProvider) oauth2Config(ctx context.Context) (*oauth2.Config, error) {
	discovered, err := p.discover(ctx)
	if err != nil {
		return nil, err
	}
	return &oauth2.Config{
		ClientID:     p.cfg.ClientID,
		ClientSecret: p.cfg.ClientSecret,
		Endpoint:     discovered.Endpoint(),
		RedirectURL:  p.redirectURI,
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}, nil
}

// authCodeOptions are the extra parameters on the authorization request: the
// ID-token nonce and the PKCE challenge.
func (p *ssoProvider) authCodeOptions(nonce, verifier string) []oauth2.AuthCodeOption {
	return []oauth2.AuthCodeOption{oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)}
}

// identity validates the provider's answer and returns who signed in, or a
// reason code for the sign-in surface. It verifies the ID token through the
// library (signature, issuer, audience, expiry) and then the nonce against the
// one this server minted.
//
// The issuer it returns is the one the *verified token* carries, not the one
// configured, so the value accounts are keyed on is always one the provider
// signed.
func (p *ssoProvider) identity(ctx context.Context, tok *oauth2.Token, nonce string) (ssoIdentity, string) {
	rawID, ok := tok.Extra("id_token").(string)
	if !ok || rawID == "" {
		return ssoIdentity{}, "token"
	}
	discovered, err := p.discover(ctx)
	if err != nil {
		return ssoIdentity{}, "unreachable"
	}
	idToken, err := discovered.Verifier(&oidc.Config{ClientID: p.cfg.ClientID}).Verify(ctx, rawID)
	if err != nil {
		return ssoIdentity{}, "token"
	}
	if subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(nonce)) != 1 {
		return ssoIdentity{}, "token"
	}
	var claims struct {
		Email    string `json:"email"`
		Verified bool   `json:"email_verified"`
		Name     string `json:"name"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return ssoIdentity{}, "token"
	}
	return ssoIdentity{
		Issuer:        idToken.Issuer,
		Subject:       idToken.Subject,
		Email:         strings.TrimSpace(strings.ToLower(claims.Email)),
		EmailVerified: claims.Verified,
		Name:          strings.TrimSpace(claims.Name),
	}, ""
}

// ssoStart sends the browser to the provider named in the path. state, nonce
// and a PKCE verifier are minted here and kept only in the transaction cookie,
// so the callback can tell its own redirect apart from one an attacker induced.
func (s *Server) ssoStart(w http.ResponseWriter, r *http.Request) {
	provider, ok := s.ssoByID[r.PathValue("provider")]
	if !ok {
		writeErr(w, http.StatusNotFound, errSSODisabled.Error())
		return
	}
	// Throttled like the callback, on its own key so one leg cannot exhaust the
	// other's budget. Nothing here writes a row, but discovery can reach the
	// provider and every call mints two random tokens, and a sign-in surface
	// that is hit ten times a minute from one address is not a person. The key
	// is not per provider: alternating buttons must not buy a second budget.
	if !s.allowAttempt("sso-start-ip:" + clientIP(r)) {
		writeErr(w, http.StatusTooManyRequests, "too many attempts; wait a minute")
		return
	}
	conf, err := provider.oauth2Config(r.Context())
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "sso provider is unreachable")
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
		Name:     ssoTxCookie,
		Value:    strings.Join([]string{provider.cfg.ID, state, nonce, verifier}, "."),
		Path:     ssoCookiePath,
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ssoTxTTL / time.Second),
	})
	http.Redirect(w, r, conf.AuthCodeURL(state, provider.authCodeOptions(nonce, verifier)...), http.StatusFound)
}

// ssoTx is the provider/state/nonce/verifier tuple read back from the cookie.
type ssoTx struct{ provider, state, nonce, verifier string }

func readSSOTx(r *http.Request) (ssoTx, bool) {
	c, err := r.Cookie(ssoTxCookie)
	if err != nil || c.Value == "" {
		return ssoTx{}, false
	}
	// "." separates the four because none of them can contain one: the provider
	// id is one of our own slugs, state and nonce are hex, and the PKCE verifier
	// is base64url.
	parts := strings.Split(c.Value, ".")
	if len(parts) != 4 {
		return ssoTx{}, false
	}
	for _, part := range parts {
		if part == "" {
			return ssoTx{}, false
		}
	}
	return ssoTx{provider: parts[0], state: parts[1], nonce: parts[2], verifier: parts[3]}, true
}

func clearSSOTx(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: ssoTxCookie, Value: "", Path: ssoCookiePath, HttpOnly: true,
		Secure: isHTTPS(r), SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}

// ssoCallback completes the sign-in: identifies the provider from the cookie,
// verifies state against it, exchanges the code, validates the provider's
// answer, then applies the linking rules.
//
// It always answers with a redirect to the SPA rather than JSON: the browser
// arrives here by navigation, so a JSON body would leave the user staring at
// raw text. Failures carry ?sso_error=<reason> for the sign-in surface to show.
func (s *Server) ssoCallback(w http.ResponseWriter, r *http.Request) {
	if len(s.sso) == 0 {
		writeErr(w, http.StatusNotFound, errSSODisabled.Error())
		return
	}
	tx, ok := readSSOTx(r)
	clearSSOTx(w, r)
	// Which provider this callback belongs to comes from the cookie alone. The
	// query is the provider's — and an attacker's — to write; the cookie is one
	// this server minted at /start, so the credentials the code is exchanged
	// with are always the ones the code was issued for.
	var provider *ssoProvider
	if ok {
		provider, ok = s.ssoByID[tx.provider]
	}
	// The state check comes before anything that costs a round trip, so an
	// unsolicited callback can't make this server talk to the provider.
	// Constant-time, as everywhere else this repo compares a token: neither
	// value is guessable per attempt, so this is consistency rather than a
	// break being closed.
	state := r.URL.Query().Get("state")
	if !ok || state == "" || subtle.ConstantTimeCompare([]byte(state), []byte(tx.state)) != 1 {
		s.ssoFail(w, r, "state")
		return
	}
	if r.URL.Query().Get("error") != "" {
		s.ssoFail(w, r, "provider")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		s.ssoFail(w, r, "provider")
		return
	}
	// Everything past here costs a request to the provider, and a caller can
	// mint its own state by calling /start first. Throttle on the same sliding
	// window password login uses so this can't be looped into traffic at the
	// identity provider.
	if !s.allowAttempt("sso-ip:" + clientIP(r)) {
		s.ssoFail(w, r, "throttled")
		return
	}
	conf, err := provider.oauth2Config(r.Context())
	if err != nil {
		s.ssoFail(w, r, "unreachable")
		return
	}
	token, err := conf.Exchange(r.Context(), code, oauth2.VerifierOption(tx.verifier))
	if err != nil {
		s.ssoFail(w, r, "exchange")
		return
	}
	identity, reason := provider.identity(r.Context(), token, tx.nonce)
	if reason != "" {
		s.ssoFail(w, r, reason)
		return
	}
	user, reason := s.ssoResolveUser(identity)
	if reason != "" {
		s.ssoFail(w, r, reason)
		return
	}
	s.ssoFinishSignIn(w, r, user)
}

// ssoResolveUser applies the account-linking rules and returns the account to
// sign in, or a failure reason for the SPA.
//
//   - A known issuer+subject is that account, full stop. The email the provider
//     asserts is not consulted, so an address changing hands there neither
//     orphans the link nor moves it. Two providers asserting the same subject
//     are two identities, because the issuer is half of the key.
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
func (s *Server) ssoResolveUser(id ssoIdentity) (store.User, string) {
	if id.Issuer == "" || id.Subject == "" {
		return store.User{}, "token"
	}
	user, ok, err := store.UserByOIDCIdentity(s.db, id.Issuer, id.Subject)
	if err != nil {
		return store.User{}, "server"
	}
	if ok {
		return user, ""
	}
	// Every path below creates an account, so an address is mandatory and the
	// provider must vouch for it: an unverified one could be anybody's, and the
	// row it creates would hold that address against its real owner forever.
	if id.Email == "" || !id.EmailVerified {
		return store.User{}, "email_unverified"
	}
	if _, exists, err := store.GetUserByEmail(s.db, id.Email); err != nil {
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
	uid, err := store.CreateUser(s.db, id.Email, id.Name, store.OIDCPasswordHash, false, true)
	if err != nil {
		return store.User{}, "server"
	}
	if err := store.LinkOIDCIdentity(s.db, id.Issuer, id.Subject, uid); err != nil {
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

// ssoFinishSignIn issues the session through the same path a password login
// uses, so cookie flags, expiry and every security-mode gate behave identically
// afterwards. An account with TOTP enabled still has to present it: the ticket
// goes back in an HttpOnly cookie and the redirect carries only a flag saying a
// code is wanted, so the ticket never lands in the address bar or in history.
// The sign-in surface then posts the code to the existing
// /api/auth/mfa/complete, which reads the cookie when the body has no ticket.
func (s *Server) ssoFinishSignIn(w http.ResponseWriter, r *http.Request, user store.User) {
	factor, hasFactor, err := store.GetMFAFactor(s.db, user.ID)
	if err != nil {
		s.ssoFail(w, r, "server")
		return
	}
	if hasFactor && factor.EnabledAt > 0 {
		ticket, err := auth.GenerateToken()
		if err != nil {
			s.ssoFail(w, r, "server")
			return
		}
		if err := store.CreateMFATicket(s.db, auth.HashToken(ticket), user.ID, time.Now().Add(mfaTicketTTL).Unix()); err != nil {
			s.ssoFail(w, r, "server")
			return
		}
		setMFATicketCookie(w, r, ticket)
		http.Redirect(w, r, "/?sso_mfa=1", http.StatusFound)
		return
	}
	if err := s.setSessionCookie(w, r, user.ID); err != nil {
		s.ssoFail(w, r, "server")
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

// ssoFail sends the browser back to the sign-in surface with a reason code.
// The codes are a closed vocabulary the frontend maps to text; nothing the
// provider said is echoed into the URL.
func (s *Server) ssoFail(w http.ResponseWriter, r *http.Request, reason string) {
	http.Redirect(w, r, "/?sso_error="+url.QueryEscape(reason), http.StatusFound)
}
