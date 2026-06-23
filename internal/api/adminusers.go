package api

import (
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
	if req.GuestAccessEnabled != nil {
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

// createInvite issues a single-use invite and returns the shareable link. The
// link base is derived from the request (scheme + host).
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
	exp := time.Now().Add(inviteTTL).Unix()
	if _, err := store.CreateInvite(s.db, auth.HashToken(raw), req.Email, req.IsAdmin, u.ID, exp); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	link := s.publicBaseURL() + "/invite/" + raw
	// Best-effort email when SMTP is configured; failure doesn't fail the call.
	if s.emailInvite != nil && req.Email != "" {
		go s.emailInvite(req.Email, link)
	}
	writeJSON(w, http.StatusOK, map[string]any{"link": link, "email": req.Email})
}

func (s *Server) publicBaseURL() string {
	if s.publicOrigin != "" {
		return s.publicOrigin
	}
	host := s.listenAddr
	if host == "" || strings.HasPrefix(host, ":") {
		return "http://127.0.0.1" + host
	}
	if _, _, err := net.SplitHostPort(host); err != nil && !strings.Contains(host, ":") {
		host = net.JoinHostPort(host, "80")
	}
	return "http://" + host
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
	raw, err := store.RegenerateEmailVerification(s.db, u.ID, time.Now().Add(emailVerificationTTL).Unix())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"link": s.publicBaseURL() + "/verify/" + raw, "email": u.Email})
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
	raw, err := auth.GenerateToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token error")
		return
	}
	if _, err := store.CreatePasswordReset(s.db, auth.HashToken(raw), u.ID, time.Now().Add(passwordResetTTL).Unix()); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"link": s.publicBaseURL() + "/reset-password/" + raw, "email": u.Email})
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
