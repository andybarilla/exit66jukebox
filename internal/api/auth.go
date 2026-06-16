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

// isHTTPS reports whether the original request was HTTPS, honoring a reverse
// proxy's X-Forwarded-Proto.
func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// clientIP extracts a throttle key from the request. X-Forwarded-For is honored
// only when the immediate peer is loopback/private (a real reverse proxy);
// otherwise it is attacker-controlled, so a public client can't rotate the
// header to mint a fresh throttle key each request and escape the limit.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if peer := net.ParseIP(host); peer != nil && (peer.IsLoopback() || peer.IsPrivate()) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return strings.TrimSpace(strings.Split(xff, ",")[0])
		}
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

// mediaAllowed reports whether a media request is authorized by a session, the
// guest toggle, or a loopback origin (the server's own ffmpeg house source, which
// fetches /api/tracks/{id}/audio over 127.0.0.1 with no cookie).
func (s *Server) mediaAllowed(r *http.Request) bool {
	if _, ok := s.currentUser(r); ok {
		return true
	}
	if store.GuestAccessEnabled(s.db) {
		return true
	}
	return isLoopback(r)
}

// isLoopback reports whether the request's TCP peer is a loopback address.
func isLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
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
