package api

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/auth"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

// getAdminSettings returns the current toggles.
func (s *Server) getAdminSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"signup_enabled":       store.SignupEnabled(s.db),
		"guest_access_enabled": store.GuestAccessEnabled(s.db),
		"security_mode":        string(store.SecurityModeSetting(s.db)),
		"admin_mfa_required":   store.AdminMFARequired(s.db),
	})
}

type adminSettingsReq struct {
	SignupEnabled      *bool   `json:"signup_enabled"`
	GuestAccessEnabled *bool   `json:"guest_access_enabled"`
	SecurityMode       *string `json:"security_mode"`
	AdminMFARequired   *bool   `json:"admin_mfa_required"`
}

// setAdminSettings flips whichever toggles are present in the body.
func (s *Server) setAdminSettings(w http.ResponseWriter, r *http.Request) {
	var req adminSettingsReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.SecurityMode != nil {
		mode, err := store.ParseSecurityMode(*req.SecurityMode)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "unsupported security mode")
			return
		}
		if err := store.SetSecurityMode(s.db, mode); err != nil {
			writeErr(w, http.StatusInternalServerError, "db error")
			return
		}
	}
	if req.SignupEnabled != nil {
		if err := store.SetSignupEnabled(s.db, *req.SignupEnabled); err != nil {
			writeErr(w, http.StatusInternalServerError, "db error")
			return
		}
	}
	if req.GuestAccessEnabled != nil && req.SecurityMode == nil {
		if err := store.SetGuestAccessEnabled(s.db, *req.GuestAccessEnabled); err != nil {
			writeErr(w, http.StatusInternalServerError, "db error")
			return
		}
	}
	if req.AdminMFARequired != nil {
		if !s.setAdminMFARequired(w, r, *req.AdminMFARequired) {
			return
		}
	}
	s.getAdminSettings(w, r)
}

func (s *Server) setAdminMFARequired(w http.ResponseWriter, r *http.Request, required bool) bool {
	if !required {
		if err := store.SetAdminMFARequired(s.db, false); err != nil {
			writeErr(w, http.StatusInternalServerError, "db error")
			return false
		}
		return true
	}

	users, err := store.ListUsers(s.db)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return false
	}
	hasAdminWithEnabledMFA := false
	for _, user := range users {
		if !user.IsAdmin {
			continue
		}
		factor, ok, err := store.GetMFAFactor(s.db, user.ID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "db error")
			return false
		}
		if ok && factor.EnabledAt > 0 {
			hasAdminWithEnabledMFA = true
			break
		}
	}
	if !hasAdminWithEnabledMFA {
		writeErr(w, http.StatusBadRequest, "at least one admin must have enabled MFA")
		return false
	}

	currentAdmin, ok := s.currentUser(r)
	if !ok {
		writeErr(w, http.StatusForbidden, "mfa required for admin access")
		return false
	}
	factor, ok, err := store.GetMFAFactor(s.db, currentAdmin.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return false
	}
	if !ok || factor.EnabledAt <= 0 {
		writeErr(w, http.StatusForbidden, "mfa required for admin access")
		return false
	}
	if err := store.SetAdminMFARequired(s.db, true); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return false
	}
	return true
}

type createInviteReq struct {
	Email   string `json:"email"`
	IsAdmin bool   `json:"is_admin"`
}

const inviteTTL = 7 * 24 * time.Hour

// createInvite issues a single-use invite and returns the shareable link.
func (s *Server) createInvite(w http.ResponseWriter, r *http.Request) {
	var req createInviteReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	// An invite is redeemed against its stored email (inviteAccept rejects a
	// blank one) and the accept screen never collects an address, so a blank
	// email here would mint a permanently dead link. Require it.
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" {
		writeErr(w, http.StatusBadRequest, "email is required")
		return
	}
	u, _ := s.currentUser(r)
	raw, err := auth.GenerateToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token error")
		return
	}
	base, err := s.remoteBaseURL()
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, publicOriginRequired)
		return
	}
	exp := time.Now().Add(inviteTTL).Unix()
	if _, err := store.CreateInvite(s.db, auth.HashToken(raw), req.Email, req.IsAdmin, u.ID, exp); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	link := base + "/invite/" + raw
	// Best-effort email when SMTP is configured; failure doesn't fail the call.
	if s.emailInvite != nil && req.Email != "" {
		go s.emailInvite(req.Email, link)
	}
	writeJSON(w, http.StatusOK, map[string]any{"link": link, "email": req.Email})
}

// publicBaseURL resolves a browser-facing base URL, rewriting a wildcard listen
// address to loopback since "http://0.0.0.0:8066" is not a URL anyone can open.
// That rewrite only holds for a reader on this machine; anything sent elsewhere
// goes through remoteBaseURL.
func (s *Server) publicBaseURL() string {
	if s.publicOrigin != "" {
		return s.publicOrigin
	}
	host, port := s.listenHostPort()
	if isWildcardHost(host) {
		// A wildcard with no port at all leaves the port implied rather than
		// inventing :80 for it.
		if port == "" {
			return "http://127.0.0.1"
		}
		host = "127.0.0.1"
	}
	if port == "" {
		port = "80"
	}
	return "http://" + net.JoinHostPort(host, port)
}

// listenHostPort splits the listen address into a bare host — brackets stripped
// from an IPv6 literal, so callers can compare and JoinHostPort can re-add them
// — and its port, which is "" when the address carries none.
func (s *Server) listenHostPort() (host, port string) {
	if h, p, err := net.SplitHostPort(s.listenAddr); err == nil {
		return strings.Trim(h, "[]"), p
	}
	return strings.Trim(s.listenAddr, "[]"), ""
}

var errPublicOriginUnset = errors.New("public origin is not configured")

// publicOriginRequired names the setting, because the operator reading it is
// the only person who can fix it.
const publicOriginRequired = "set EXIT66_PUBLIC_ORIGIN: links can't be generated from this listen address"

// remoteBaseURL resolves the base for a link that reaches someone other than
// the operator at the machine — invite, password reset, email verification.
// publicBaseURL's loopback rewrite is a guess that only holds locally: in
// someone else's inbox that link points at their own machine and silently
// fails. So this refuses rather than guesses, and callers fail the operation
// instead of sending it.
func (s *Server) remoteBaseURL() (string, error) {
	if s.publicOrigin != "" {
		return s.publicOrigin, nil
	}
	if !s.listensOnLoopback() {
		return "", errPublicOriginUnset
	}
	return s.publicBaseURL(), nil
}

// listensOnLoopback reports whether the listen address is one only this machine
// can reach. A wildcard bind is deliberately excluded even though publicBaseURL
// maps it to loopback: it stands for every interface, including the ones a
// remote recipient came in on.
func (s *Server) listensOnLoopback() bool {
	host, _ := s.listenHostPort()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// isWildcardHost reports whether host is an "any interface" bind address. It
// takes a host already stripped of IPv6 brackets, as listenHostPort returns.
func isWildcardHost(host string) bool {
	switch host {
	case "0.0.0.0", "::", "":
		return true
	}
	return false
}

// listInvites returns all invites with a derived status.
func (s *Server) listInvites(w http.ResponseWriter, r *http.Request) {
	invs, err := store.ListInvites(s.db)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	out := make([]map[string]any, 0, len(invs))
	now := time.Now().Unix()
	for _, inv := range invs {
		status := "pending"
		switch {
		case inv.AcceptedAt != 0:
			status = "accepted"
		case inv.ExpiresAt <= now:
			status = "expired"
		}
		out = append(out, map[string]any{
			"id": inv.ID, "email": inv.Email, "is_admin": inv.IsAdmin, "status": status,
			"created_at": inv.CreatedAt, "expires_at": inv.ExpiresAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// deleteInvite revokes an invite by id.
func (s *Server) deleteInvite(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	if err := store.DeleteInvite(s.db, id); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// listUsers returns all accounts (no password hashes).
func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := store.ListUsers(s.db)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		factor, ok, err := store.GetMFAFactor(s.db, u.ID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "db error")
			return
		}
		out = append(out, map[string]any{
			"id": u.ID, "email": u.Email, "display_name": u.DisplayName,
			"is_admin": u.IsAdmin, "created_at": u.CreatedAt, "mfa_enabled": ok && factor.EnabledAt > 0,
			"email_verified": u.EmailVerifiedAt != 0, "email_verified_at": u.EmailVerifiedAt,
			"is_passwordless_profile": u.IsPasswordlessProfile,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createEmailVerification(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	u, ok, err := store.GetUserByID(s.db, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, "user not found")
		return
	}
	if u.EmailVerifiedAt != 0 {
		writeErr(w, http.StatusBadRequest, "user is already verified")
		return
	}
	base, err := s.remoteBaseURL()
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, publicOriginRequired)
		return
	}
	raw, err := store.RegenerateEmailVerification(s.db, u.ID, time.Now().Add(emailVerificationTTL).Unix())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"link": base + "/verify/" + raw, "email": u.Email})
}

func (s *Server) createPasswordReset(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	u, ok, err := store.GetUserByID(s.db, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, "user not found")
		return
	}
	base, err := s.remoteBaseURL()
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, publicOriginRequired)
		return
	}
	raw, err := auth.GenerateToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token error")
		return
	}
	if _, err := store.CreatePasswordReset(s.db, auth.HashToken(raw), u.ID, time.Now().Add(passwordResetTTL).Unix()); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"link": base + "/reset-password/" + raw, "email": u.Email})
}

// deleteUser removes an account. An admin can't delete themselves (avoids
// locking out the last admin by accident).
func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	if me, ok := s.currentUser(r); ok && me.ID == id {
		writeErr(w, http.StatusBadRequest, "can't delete your own account")
		return
	}
	if err := store.DeleteUser(s.db, id); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
