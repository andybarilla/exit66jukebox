package api

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/auth"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

const sessionCookie = "exit66_session"

const sessionTTL = 30 * 24 * time.Hour

const passwordResetTTL = time.Hour

// requireAuth gates non-admin routes. A valid session passes. With no session it
// passes only when guest access is enabled; otherwise 401.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.currentUser(r); ok {
			next(w, r)
			return
		}
		if store.GuestAccessEnabled(s.db) {
			next(w, r)
			return
		}
		writeErr(w, http.StatusUnauthorized, "login required")
	}
}

// setSessionCookie issues a session: stores its hash, sets the cookie. Secure is
// set when the request arrived over TLS (direct or via a TLS-terminating proxy).
func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, userID int64) error {
	raw, err := auth.GenerateToken()
	if err != nil {
		return err
	}
	exp := time.Now().Add(sessionTTL)
	if err := store.CreateSession(s.db, auth.HashToken(raw), userID, exp.Unix()); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    raw,
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		Expires:  exp,
	})
	return nil
}

// clearSessionCookie deletes the server session and expires the cookie.
func (s *Server) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		store.DeleteSession(s.db, auth.HashToken(c.Value))
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", HttpOnly: true,
		Secure: isHTTPS(r), SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}

// fromTrustedProxy reports whether the immediate TCP peer is a loopback/private
// address — plausibly our own reverse proxy, whose forwarded headers
// (X-Forwarded-For / -Proto) we may trust. A public peer's headers are
// attacker-controlled and must be ignored.
func fromTrustedProxy(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}

// isHTTPS reports whether the original request was HTTPS. A proxy's
// X-Forwarded-Proto is honored only from a trusted peer, so a direct public
// client can't force Secure cookies (which would break a plain-HTTP LAN deploy).
func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return fromTrustedProxy(r) && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// clientIP extracts a throttle key from the request. X-Forwarded-For is honored
// only from a trusted (loopback/private) peer; otherwise it is attacker-
// controlled, so a public client can't rotate the header to mint a fresh
// throttle key each request and escape the limit.
func clientIP(r *http.Request) string {
	if fromTrustedProxy(r) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return strings.TrimSpace(strings.Split(xff, ",")[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// decodeJSON is a small helper for the auth handlers.
func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

type signupReq struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

const minPasswordLen = 8

// signup creates an account. Rules: an empty user table always allows the signup
// and makes that first account an admin (bootstrap); otherwise signup is allowed
// only when the signup toggle is on, and the account is non-admin.
func (s *Server) signup(w http.ResponseWriter, r *http.Request) {
	var req signupReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || len(req.Password) < minPasswordLen {
		writeErr(w, http.StatusBadRequest, "email and an 8+ char password are required")
		return
	}
	n, err := store.CountUsers(s.db)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	bootstrap := n == 0
	if !bootstrap && !store.SignupEnabled(s.db) {
		writeErr(w, http.StatusForbidden, "signup is disabled")
		return
	}
	s.createAccountAndLogin(w, r, req.Email, req.DisplayName, req.Password, bootstrap)
}

// createAccountAndLogin hashes the password, inserts the user, and logs them in
// by setting a session cookie. isAdmin grants the admin role.
func (s *Server) createAccountAndLogin(w http.ResponseWriter, r *http.Request, email, name, pw string, isAdmin bool) {
	hash, err := auth.HashPassword(pw)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash error")
		return
	}
	uid, err := store.CreateUser(s.db, email, name, hash, isAdmin)
	if err != nil {
		writeErr(w, http.StatusConflict, "email already registered")
		return
	}
	if err := s.setSessionCookie(w, r, uid); err != nil {
		writeErr(w, http.StatusInternalServerError, "session error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": uid, "email": email, "is_admin": isAdmin})
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// login validates credentials and issues a session cookie. Throttled on both
// the client IP and the target email so a single account can't be brute-forced
// even if the attacker rotates X-Forwarded-For across many apparent IPs.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if !s.allowAttempt("ip:"+clientIP(r)) || !s.allowAttempt("email:"+req.Email) {
		writeErr(w, http.StatusTooManyRequests, "too many attempts; wait a minute")
		return
	}
	u, ok, err := store.GetUserByEmail(s.db, req.Email)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if !ok || !auth.VerifyPassword(req.Password, u.PasswordHash) {
		writeErr(w, http.StatusUnauthorized, "incorrect email or password")
		return
	}
	if err := s.setSessionCookie(w, r, u.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "session error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": u.ID, "email": u.Email, "is_admin": u.IsAdmin})
}

// logout clears the session.
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	s.clearSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// me returns the current user, or 401 when anonymous.
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "not logged in")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": u.ID, "email": u.Email, "display_name": u.DisplayName, "is_admin": u.IsAdmin,
	})
}

type inviteAcceptReq struct {
	Token       string `json:"token"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

type forgotPasswordReq struct {
	Email string `json:"email"`
}

func (s *Server) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var req forgotPasswordReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if !s.allowAttempt("password-reset-ip:"+clientIP(r)) || !s.allowAttempt("password-reset-email:"+req.Email) {
		writeErr(w, http.StatusTooManyRequests, "too many attempts; wait a minute")
		return
	}
	if req.Email == "" {
		writeJSON(w, http.StatusOK, passwordResetAccepted())
		return
	}
	u, ok, err := store.GetUserByEmail(s.db, req.Email)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if !ok || s.emailPasswordReset == nil {
		writeJSON(w, http.StatusOK, passwordResetAccepted())
		return
	}
	raw, err := auth.GenerateToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token error")
		return
	}
	expiresAt := time.Now().Add(passwordResetTTL).Unix()
	if _, err := store.CreatePasswordReset(s.db, auth.HashToken(raw), u.ID, expiresAt); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	s.emailPasswordReset(req.Email, inviteBaseURL(r)+"/reset-password/"+raw)
	writeJSON(w, http.StatusOK, passwordResetAccepted())
}

func passwordResetAccepted() map[string]any {
	return map[string]any{"ok": true}
}

type resetPasswordReq struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetPasswordReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Token == "" || len(req.Password) < minPasswordLen {
		writeErr(w, http.StatusBadRequest, "token and an 8+ char password are required")
		return
	}
	reset, ok, err := store.PendingPasswordReset(s.db, auth.HashToken(req.Token))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid or expired reset token")
		return
	}
	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash error")
		return
	}
	if err := store.UpdateUserPassword(s.db, reset.UserID, passwordHash); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if err := store.MarkPasswordResetUsed(s.db, reset.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if err := store.DeleteSessionsForUser(s.db, reset.UserID); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// inviteAccept redeems an invite: validates the token, creates the account
// (admin if the invite granted it), marks the invite used, and logs in. The
// account email comes from the invite (set by the admin), never client input.
func (s *Server) inviteAccept(w http.ResponseWriter, r *http.Request) {
	var req inviteAcceptReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(req.Password) < minPasswordLen {
		writeErr(w, http.StatusBadRequest, "password must be 8+ characters")
		return
	}
	inv, ok, err := store.PendingInvite(s.db, auth.HashToken(req.Token))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if !ok {
		writeErr(w, http.StatusBadRequest, "invite is invalid or expired")
		return
	}
	if inv.Email == "" {
		writeErr(w, http.StatusBadRequest, "invite has no email; ask the admin to reissue")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash error")
		return
	}
	uid, err := store.CreateUser(s.db, inv.Email, req.DisplayName, hash, inv.IsAdmin)
	if err != nil {
		writeErr(w, http.StatusConflict, "email already registered")
		return
	}
	if err := store.MarkInviteAccepted(s.db, inv.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if err := s.setSessionCookie(w, r, uid); err != nil {
		writeErr(w, http.StatusInternalServerError, "session error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": uid, "email": inv.Email, "is_admin": inv.IsAdmin})
}

// mediaAllowed reports whether the request carries a valid session or guest
// access is enabled. It deliberately does NOT trust the peer address: behind a
// same-host reverse proxy every request arrives from 127.0.0.1, so a loopback
// bypass would open the whole API to the internet. Cookie-less internal callers
// (the ffmpeg house source) and Sonos use signed URLs instead — see signedOK.
func (s *Server) mediaAllowed(r *http.Request) bool {
	if _, ok := s.currentUser(r); ok {
		return true
	}
	return store.GuestAccessEnabled(s.db)
}

// signedOK reports whether the request carries a path-scoped signed token valid
// for its own URL path (the Sonos cast and the ffmpeg house source both fetch
// with no cookie). A forged or wrong-path token fails VerifyPath.
func (s *Server) signedOK(r *http.Request) bool {
	sig := r.URL.Query().Get("sig")
	return sig != "" && auth.VerifyPath(s.signingSecret, sig, r.URL.Path, time.Now().Unix())
}

// RequireAuthMiddleware gates the public listener's API routes. Anything not
// under /api/ (the static SPA shell, and /stream/ which self-guards) passes
// through; open auth/config endpoints pass; otherwise the request needs a valid
// session, the guest toggle, or a valid signed token for its path (the ffmpeg
// house source fetches /api/tracks/{id}/audio this way). This is the production
// gate; it wraps ONLY the public http.Server, never the federation MemberHandler.
func (s *Server) RequireAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") || isOpenPath(r.URL.Path) || s.mediaAllowed(r) || s.signedOK(r) {
			next.ServeHTTP(w, r)
			return
		}
		writeErr(w, http.StatusUnauthorized, "login required")
	})
}

// isOpenPath lists API routes reachable without authentication: the auth
// endpoints and /api/config (so the unauthenticated SPA can decide whether to
// show login, signup, or first-run bootstrap).
func isOpenPath(p string) bool {
	switch p {
	case "/api/auth/login", "/api/auth/signup", "/api/auth/logout",
		"/api/auth/me", "/api/auth/invite/accept", "/api/config":
		return true
	}
	return false
}

// allowAttempt records an attempt under key and reports whether key is still
// under the limit (10 attempts / 60s sliding window). Login keys on both the
// client IP and the target email so neither dimension alone can be brute-forced.
func (s *Server) allowAttempt(key string) bool {
	const window = 60 * 1000
	const maxAttempts = 10
	now := time.Now().UnixMilli()
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	cutoff := now - window
	// Opportunistic sweep so rotating ips/emails can't grow the map without bound
	// (a key's last entry is its most recent attempt; all-stale keys are dropped).
	if len(s.loginAttempts) > 4096 {
		for k, ts := range s.loginAttempts {
			if len(ts) == 0 || ts[len(ts)-1] <= cutoff {
				delete(s.loginAttempts, k)
			}
		}
	}
	kept := s.loginAttempts[key][:0]
	for _, t := range s.loginAttempts[key] {
		if t > cutoff {
			kept = append(kept, t)
		}
	}
	kept = append(kept, now)
	s.loginAttempts[key] = kept
	return len(kept) <= maxAttempts
}
