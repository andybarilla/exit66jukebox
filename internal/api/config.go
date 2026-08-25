package api

import (
	"database/sql"
	"net/http"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	peers := []string{}
	if s.fedPeers != nil {
		peers = s.fedPeers()
	}
	u, authed := s.currentUser(r)
	mode := store.SecurityModeSetting(s.db)
	isPasswordlessProfile := authed && u.IsPasswordlessProfile
	_, hasPersonalStream := personalStreamFor(mode, u, authed)
	writeJSON(w, http.StatusOK, map[string]any{
		"mute_local_on_cast": s.muteLocalOnCast,
		"fed_peers":          peers,
		"authenticated":      authed,
		"is_admin":           authed && u.IsAdmin,
		"security_mode":      string(mode),
		"guest_access":       store.SecurityModeAllowsAnonymous(mode),
		"requires_profile":   mode == store.SecurityModeHouseholdProfiles && !isPasswordlessProfile,
		"requires_login":     mode == store.SecurityModeFullLogin && (!authed || isPasswordlessProfile),
		"signup_enabled":     mode == store.SecurityModeFullLogin && store.SignupEnabled(s.db),
		"needs_bootstrap":    countUsersZero(s.db),
		"max_shared_streams": store.MaxSharedStreams,
		// Whether this caller has a personal stream at all: false in the two
		// open modes, where there is no user to key one on, so the client hides
		// the Personal control rather than calling a route that 404s.
		"personal_stream": hasPersonalStream,
	})
}

// countUsersZero reports whether no accounts exist yet (first signup bootstraps
// the admin). Errors are treated as "not zero" so a transient DB error doesn't
// reopen bootstrap.
func countUsersZero(db *sql.DB) bool {
	n, err := store.CountUsers(db)
	return err == nil && n == 0
}
