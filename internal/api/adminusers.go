package api

import (
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
	})
}

type adminSettingsReq struct {
	SignupEnabled      *bool `json:"signup_enabled"`
	GuestAccessEnabled *bool `json:"guest_access_enabled"`
}

// setAdminSettings flips whichever toggles are present in the body.
func (s *Server) setAdminSettings(w http.ResponseWriter, r *http.Request) {
	var req adminSettingsReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
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
	s.getAdminSettings(w, r)
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
	link := inviteBaseURL(r) + "/invite/" + raw
	// Best-effort email when SMTP is configured; failure doesn't fail the call.
	if s.emailInvite != nil && req.Email != "" {
		go s.emailInvite(req.Email, link)
	}
	writeJSON(w, http.StatusOK, map[string]any{"link": link, "email": req.Email})
}

// inviteBaseURL reconstructs the public origin from the request.
func inviteBaseURL(r *http.Request) string {
	scheme := "http"
	if isHTTPS(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
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
		out = append(out, map[string]any{
			"id": u.ID, "email": u.Email, "display_name": u.DisplayName,
			"is_admin": u.IsAdmin, "created_at": u.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
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
