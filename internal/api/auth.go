package api

import (
	"encoding/json"
	"errors"
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

const emailVerificationTTL = 24 * time.Hour

const mfaTicketTTL = 5 * time.Minute

var errVerificationEmailerUnavailable = errors.New("verification emailer unavailable")

// requireAuth gates browser API routes. Anonymous browser access passes only in
// open modes; household_profiles requires a passwordless profile session and
// full_login requires a password account session.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.browserAccessAllowed(r) {
			next(w, r)
			return
		}
		writeErr(w, http.StatusUnauthorized, "login required")
	}
}

func (s *Server) browserAccessAllowed(r *http.Request) bool {
	mode := store.SecurityModeSetting(s.db)
	user, hasUser := s.currentUser(r)
	switch mode {
	case store.SecurityModeOpen, store.SecurityModeOpenAdminLocked:
		return true
	case store.SecurityModeHouseholdProfiles:
		return hasUser && user.IsPasswordlessProfile
	case store.SecurityModeFullLogin:
		return hasUser && !user.IsPasswordlessProfile
	default:
		return false
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

func userJSON(u store.User) map[string]any {
	return map[string]any{
		"id":                      u.ID,
		"email":                   u.Email,
		"display_name":            u.DisplayName,
		"is_admin":                u.IsAdmin,
		"email_verified":          u.EmailVerifiedAt != 0,
		"is_passwordless_profile": u.IsPasswordlessProfile,
	}
}

type passwordlessProfileReq struct {
	DisplayName string `json:"display_name"`
}

type selectPasswordlessProfileReq struct {
	ID int64 `json:"id"`
}

func (s *Server) passwordlessProfilesEnabled(w http.ResponseWriter) bool {
	if store.SecurityModeSetting(s.db) == store.SecurityModeHouseholdProfiles {
		return true
	}
	writeErr(w, http.StatusForbidden, "passwordless profiles require household_profiles mode")
	return false
}

func profileJSON(u store.User) map[string]any {
	return map[string]any{"id": u.ID, "display_name": u.DisplayName}
}

func (s *Server) listPasswordlessProfiles(w http.ResponseWriter, r *http.Request) {
	if !s.passwordlessProfilesEnabled(w) {
		return
	}
	profiles, err := store.ListPasswordlessProfiles(s.db)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	out := make([]map[string]any, 0, len(profiles))
	for _, profile := range profiles {
		out = append(out, profileJSON(profile))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createPasswordlessProfile(w http.ResponseWriter, r *http.Request) {
	if !s.passwordlessProfilesEnabled(w) {
		return
	}
	var req passwordlessProfileReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if req.DisplayName == "" {
		writeErr(w, http.StatusBadRequest, "display name is required")
		return
	}
	id, err := store.CreatePasswordlessProfile(s.db, req.DisplayName)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	user, ok, err := store.GetUserByID(s.db, id)
	if err != nil || !ok {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, profileJSON(user))
}

func (s *Server) selectPasswordlessProfile(w http.ResponseWriter, r *http.Request) {
	if !s.passwordlessProfilesEnabled(w) {
		return
	}
	var req selectPasswordlessProfileReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	user, ok, err := store.GetUserByID(s.db, req.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if !ok || !user.IsPasswordlessProfile {
		writeErr(w, http.StatusNotFound, "profile not found")
		return
	}
	if err := s.setSessionCookie(w, r, user.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "session error")
		return
	}
	writeJSON(w, http.StatusOK, userJSON(user))
}

type signupReq struct {
	Email          string `json:"email"`
	DisplayName    string `json:"display_name"`
	Password       string `json:"password"`
	BootstrapToken string `json:"bootstrap_token"`
}

const minPasswordLen = 8

// signup creates an account. Rules: on an empty user table the request must
// carry the startup bootstrap token, and that first account is created as admin
// through the atomic CreateFirstAdmin path; otherwise signup requires
// full_login mode with the signup toggle on, and the account is non-admin.
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
	if bootstrap {
		// A caller that read an empty table can still lose the bootstrap to a
		// concurrent winner, or find every account deleted under a claimed
		// bootstrap. Both mean "already claimed", not "your token is wrong".
		switch s.bootstrapTokenStatus(strings.TrimSpace(req.BootstrapToken)) {
		case bootstrapClaimed:
			writeErr(w, http.StatusConflict, "bootstrap already claimed")
			return
		case bootstrapInvalid:
			writeErr(w, http.StatusForbidden, "valid bootstrap token required")
			return
		}
	}
	if !bootstrap && store.SecurityModeSetting(s.db) != store.SecurityModeFullLogin {
		writeErr(w, http.StatusForbidden, "signup is available only in full_login mode")
		return
	}
	if !bootstrap && !store.SignupEnabled(s.db) {
		writeErr(w, http.StatusForbidden, "signup is disabled")
		return
	}
	s.createSignupAccount(w, r, req.Email, req.DisplayName, req.Password, bootstrap)
}

func (s *Server) createSignupAccount(w http.ResponseWriter, r *http.Request, email, name, pw string, bootstrap bool) {
	hash, err := auth.HashPassword(pw)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash error")
		return
	}
	if !bootstrap {
		uid, err := s.createAndSendVerification(email, name, hash)
		if err != nil {
			if existing, _, dbErr := store.GetUserByEmail(s.db, email); dbErr != nil {
				writeErr(w, http.StatusInternalServerError, "db error")
				return
			} else if existing.ID != 0 {
				writeErr(w, http.StatusConflict, "email already registered")
				return
			}
			if errors.Is(err, errVerificationEmailerUnavailable) {
				writeErr(w, http.StatusServiceUnavailable, "verification email is not configured")
				return
			}
			if errors.Is(err, errPublicOriginUnset) {
				writeErr(w, http.StatusServiceUnavailable, publicOriginRequired)
				return
			}
			writeErr(w, http.StatusServiceUnavailable, "verification email could not be sent")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": uid, "email": email, "is_admin": false, "email_verified": false})
		return
	}
	uid, err := store.CreateFirstAdmin(s.db, email, name, hash)
	if err != nil {
		if errors.Is(err, store.ErrBootstrapAlreadyClaimed) {
			writeErr(w, http.StatusConflict, "bootstrap already claimed")
			return
		}
		writeErr(w, http.StatusConflict, "email already registered")
		return
	}
	s.markBootstrapClaimed()
	if err := s.setSessionCookie(w, r, uid); err != nil {
		writeErr(w, http.StatusInternalServerError, "session error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": uid, "email": email, "is_admin": true, "email_verified": true})
}

func (s *Server) createAndSendVerification(email, name, hash string) (int64, error) {
	if s.emailVerification == nil {
		return 0, errVerificationEmailerUnavailable
	}
	// Resolve the link before the insert. An account whose verification mail
	// can't be addressed is worse than no account: login gates on
	// EmailVerifiedAt, so the row would be permanently unusable while still
	// holding the address against a retry.
	base, err := s.remoteBaseURL()
	if err != nil {
		return 0, err
	}
	uid, raw, err := store.CreateUnverifiedUserWithEmailVerification(s.db, email, name, hash, time.Now().Add(emailVerificationTTL).Unix())
	if err != nil {
		return 0, err
	}
	if err := s.emailVerification(email, base+"/verify/"+raw); err != nil {
		if deleteErr := store.DeleteUser(s.db, uid); deleteErr != nil {
			return 0, deleteErr
		}
		return 0, err
	}
	return uid, nil
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

type mfaCompleteReq struct {
	Ticket       string `json:"ticket"`
	Code         string `json:"code"`
	RecoveryCode string `json:"recovery_code"`
}

type mfaCodeReq struct {
	Code string `json:"code"`
}

type mfaPasswordChallengeReq struct {
	Password     string `json:"password"`
	Code         string `json:"code"`
	RecoveryCode string `json:"recovery_code"`
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
	if u.EmailVerifiedAt == 0 {
		writeErr(w, http.StatusForbidden, "verify your email before logging in")
		return
	}
	factor, hasFactor, err := store.GetMFAFactor(s.db, u.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if hasFactor && factor.EnabledAt > 0 {
		s.createMFATicket(w, u.ID)
		return
	}
	if err := s.setSessionCookie(w, r, u.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "session error")
		return
	}
	writeJSON(w, http.StatusOK, userJSON(u))
}

func (s *Server) createMFATicket(w http.ResponseWriter, userID int64) {
	ticket, err := auth.GenerateToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "mfa ticket error")
		return
	}
	expiresAt := time.Now().Add(mfaTicketTTL).Unix()
	if err := store.CreateMFATicket(s.db, auth.HashToken(ticket), userID, expiresAt); err != nil {
		writeErr(w, http.StatusInternalServerError, "mfa ticket error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mfa_required": true, "ticket": ticket})
}

func (s *Server) mfaComplete(w http.ResponseWriter, r *http.Request) {
	var req mfaCompleteReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Ticket = strings.TrimSpace(req.Ticket)
	req.Code = strings.TrimSpace(req.Code)
	req.RecoveryCode = strings.TrimSpace(req.RecoveryCode)
	if req.Ticket == "" || (req.Code == "" && req.RecoveryCode == "") {
		writeErr(w, http.StatusBadRequest, "ticket and code are required")
		return
	}
	ticketHash := auth.HashToken(req.Ticket)
	if !s.allowAttempt("mfa-ip:"+clientIP(r)) || !s.allowAttempt("mfa-ticket:"+ticketHash) {
		writeErr(w, http.StatusTooManyRequests, "too many attempts; wait a minute")
		return
	}
	userID, ok, err := store.ConsumeMFATicket(s.db, ticketHash)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if !ok {
		writeErr(w, http.StatusUnauthorized, "invalid mfa ticket")
		return
	}
	factor, hasFactor, err := store.GetMFAFactor(s.db, userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if !hasFactor || factor.EnabledAt <= 0 {
		writeErr(w, http.StatusUnauthorized, "mfa required")
		return
	}
	if !s.acceptMFAChallenge(w, req, factor) {
		return
	}
	u, ok, err := store.GetUserByID(s.db, userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if !ok {
		writeErr(w, http.StatusUnauthorized, "invalid mfa ticket")
		return
	}
	if err := s.setSessionCookie(w, r, userID); err != nil {
		writeErr(w, http.StatusInternalServerError, "session error")
		return
	}
	writeJSON(w, http.StatusOK, userJSON(u))
}

func (s *Server) acceptMFAChallenge(w http.ResponseWriter, req mfaCompleteReq, factor store.MFAFactor) bool {
	recoveryCode := req.RecoveryCode
	if recoveryCode == "" && !isSixDigitCode(req.Code) {
		recoveryCode = req.Code
	}
	if recoveryCode != "" {
		accepted, err := store.MarkRecoveryCodeUsed(s.db, factor.UserID, auth.HashRecoveryCode(recoveryCode))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "db error")
			return false
		}
		if !accepted {
			writeErr(w, http.StatusUnauthorized, "invalid mfa code")
			return false
		}
		return true
	}
	secret, err := auth.DecryptTOTPSecret(s.mfaKey, auth.EncryptedSecret{
		Ciphertext: factor.SecretCiphertext,
		Nonce:      factor.SecretNonce,
		KeyVersion: factor.KeyVersion,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "mfa unavailable")
		return false
	}
	acceptedStep, ok := auth.VerifyTOTPAfterStep(secret, req.Code, time.Now(), 1, factor.LastAcceptedStep)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "invalid mfa code")
		return false
	}
	accepted, err := store.UpdateMFALastAcceptedStep(s.db, factor.UserID, acceptedStep)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return false
	}
	if !accepted {
		writeErr(w, http.StatusUnauthorized, "invalid mfa code")
		return false
	}
	return true
}

func (s *Server) mfaEnrollBegin(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := s.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "login required")
		return
	}
	existingFactor, hasExistingFactor, err := store.GetMFAFactor(s.db, currentUser.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if hasExistingFactor && existingFactor.EnabledAt > 0 {
		writeErr(w, http.StatusConflict, "mfa is already enabled")
		return
	}
	secret, err := auth.GenerateTOTPSecret()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "mfa unavailable")
		return
	}
	encryptedSecret, err := auth.EncryptTOTPSecret(s.mfaKey, secret, 1)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "mfa unavailable")
		return
	}
	if err := store.UpsertMFAFactor(s.db, store.MFAFactor{
		UserID:           currentUser.ID,
		SecretCiphertext: encryptedSecret.Ciphertext,
		SecretNonce:      encryptedSecret.Nonce,
		KeyVersion:       encryptedSecret.KeyVersion,
		EnabledAt:        0,
		LastAcceptedStep: -1,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"secret":      secret,
		"otpauth_uri": auth.TOTPURI("Exit66", currentUser.Email, secret),
	})
}

func (s *Server) mfaEnrollConfirm(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := s.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "login required")
		return
	}
	var req mfaCodeReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	if req.Code == "" {
		writeErr(w, http.StatusBadRequest, "code is required")
		return
	}
	factor, hasFactor, err := store.GetMFAFactor(s.db, currentUser.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if !hasFactor {
		writeErr(w, http.StatusBadRequest, "mfa enrollment required")
		return
	}
	if factor.EnabledAt > 0 {
		writeErr(w, http.StatusConflict, "mfa is already enabled")
		return
	}
	secret, err := auth.DecryptTOTPSecret(s.mfaKey, auth.EncryptedSecret{
		Ciphertext: factor.SecretCiphertext,
		Nonce:      factor.SecretNonce,
		KeyVersion: factor.KeyVersion,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "mfa unavailable")
		return
	}
	acceptedStep, accepted := auth.VerifyTOTPAfterStep(secret, req.Code, time.Now(), 1, factor.LastAcceptedStep)
	if !accepted {
		writeErr(w, http.StatusUnauthorized, "invalid mfa code")
		return
	}
	recoveryCodes, err := auth.GenerateRecoveryCodes(10)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "mfa unavailable")
		return
	}
	if err := store.ReplaceRecoveryCodes(s.db, currentUser.ID, hashRecoveryCodes(recoveryCodes)); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	factor.EnabledAt = time.Now().Unix()
	factor.LastAcceptedStep = acceptedStep
	if err := store.UpsertMFAFactor(s.db, factor); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recovery_codes": recoveryCodes})
}

func (s *Server) mfaDisable(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := s.verifyPasswordAndMFA(w, r)
	if !ok {
		return
	}
	if err := store.DisableMFAFactor(s.db, currentUser.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if err := store.ReplaceRecoveryCodes(s.db, currentUser.ID, nil); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) mfaRecoveryRegenerate(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := s.verifyPasswordAndMFA(w, r)
	if !ok {
		return
	}
	recoveryCodes, err := auth.GenerateRecoveryCodes(10)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "mfa unavailable")
		return
	}
	if err := store.ReplaceRecoveryCodes(s.db, currentUser.ID, hashRecoveryCodes(recoveryCodes)); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recovery_codes": recoveryCodes})
}

func (s *Server) verifyPasswordAndMFA(w http.ResponseWriter, r *http.Request) (store.User, bool) {
	currentUser, ok := s.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "login required")
		return store.User{}, false
	}
	var req mfaPasswordChallengeReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return store.User{}, false
	}
	req.Password = strings.TrimSpace(req.Password)
	req.Code = strings.TrimSpace(req.Code)
	req.RecoveryCode = strings.TrimSpace(req.RecoveryCode)
	if req.Password == "" || (req.Code == "" && req.RecoveryCode == "") {
		writeErr(w, http.StatusBadRequest, "password and mfa code are required")
		return store.User{}, false
	}
	if !auth.VerifyPassword(req.Password, currentUser.PasswordHash) {
		writeErr(w, http.StatusUnauthorized, "invalid password or mfa code")
		return store.User{}, false
	}
	factor, hasFactor, err := store.GetMFAFactor(s.db, currentUser.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return store.User{}, false
	}
	if !hasFactor || factor.EnabledAt <= 0 {
		writeErr(w, http.StatusUnauthorized, "mfa required")
		return store.User{}, false
	}
	if !s.acceptMFAChallenge(w, mfaCompleteReq{Code: req.Code, RecoveryCode: req.RecoveryCode}, factor) {
		return store.User{}, false
	}
	return currentUser, true
}

func hashRecoveryCodes(codes []string) []string {
	hashes := make([]string, 0, len(codes))
	for _, code := range codes {
		hashes = append(hashes, auth.HashRecoveryCode(code))
	}
	return hashes
}

func isSixDigitCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	for _, digit := range code {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
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
	writeJSON(w, http.StatusOK, userJSON(u))
}

type verifyEmailReq struct {
	Token string `json:"token"`
}

func (s *Server) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var req verifyEmailReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" {
		writeErr(w, http.StatusBadRequest, "verification token is required")
		return
	}
	if err := store.ConsumeEmailVerification(s.db, req.Token); err != nil {
		writeErr(w, http.StatusBadRequest, "verification link is invalid or expired")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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
	// Ahead of the lookup on purpose: this refusal depends only on server
	// configuration, so it reads the same for a registered and an unregistered
	// address and can't be used to enumerate accounts.
	base, err := s.remoteBaseURL()
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, publicOriginRequired)
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
	s.emailPasswordReset(req.Email, base+"/reset-password/"+raw)
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
	reset, ok, err := store.ConsumePasswordReset(s.db, auth.HashToken(req.Token))
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
	uid, err := store.CreateUser(s.db, inv.Email, req.DisplayName, hash, inv.IsAdmin, true)
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
	writeJSON(w, http.StatusOK, map[string]any{"id": uid, "email": inv.Email, "is_admin": inv.IsAdmin, "email_verified": true})
}

// mediaAllowed reports whether the request carries a valid session or guest
// access is enabled. It deliberately does NOT trust the peer address: behind a
// same-host reverse proxy every request arrives from 127.0.0.1, so a loopback
// bypass would open the whole API to the internet. Cookie-less internal callers
// (each shared stream's ffmpeg source) and Sonos use signed URLs instead — see
// signedOK.
func (s *Server) mediaAllowed(r *http.Request) bool {
	return s.browserAccessAllowed(r)
}

func (s *Server) adminRouteAllowed(r *http.Request) bool {
	user, hasUser := s.currentUser(r)
	if !hasUser || !user.IsAdmin {
		return false
	}
	return s.routeRequiresAdmin(r.Method, r.URL.Path)
}

func (s *Server) routeRequiresAdmin(method, path string) bool {
	if strings.HasPrefix(path, "/api/admin/") {
		return true
	}
	if s.sharedStreamRouteRequiresAdmin(method, path) {
		return true
	}
	if path == "/api/streams" {
		return method == http.MethodPost
	}
	switch path {
	case "/api/sonos/cast", "/api/sonos/stop", "/api/sonos/volume", "/api/enrich":
		return method == http.MethodPost
	default:
		return false
	}
}

// sharedStreamRouteRequiresAdmin reports whether the path is a privileged
// control on a stream that is shared. It reads the stream's kind, so it covers
// every shared stream rather than only house; the handler-side gate
// (requireAdminShared) still does the real enforcement.
func (s *Server) sharedStreamRouteRequiresAdmin(method, path string) bool {
	rest, ok := strings.CutPrefix(path, "/api/streams/")
	if !ok {
		return false
	}
	id, suffix, hasSuffix := strings.Cut(rest, "/")
	if id == "" || !s.isSharedStream(id) {
		return false
	}
	if !hasSuffix { // /api/streams/{id}: rename and delete are privileged
		return method == http.MethodPatch || method == http.MethodDelete
	}
	switch suffix {
	case "next", "shuffle":
		return method == http.MethodPost
	case "requests":
		return method == http.MethodDelete
	case "station":
		return method == http.MethodPost || method == http.MethodDelete
	default:
		return method == http.MethodDelete && strings.HasPrefix(suffix, "requests/")
	}
}

// signedOK reports whether the request carries a path-scoped signed token valid
// for its own URL path (the Sonos cast and every shared stream's ffmpeg source
// fetch with no cookie). A forged or wrong-path token fails VerifyPath.
func (s *Server) signedOK(r *http.Request) bool {
	sig := r.URL.Query().Get("sig")
	return sig != "" && auth.VerifyPath(s.signingSecret, sig, r.URL.Path, time.Now().Unix())
}

// RequireAuthMiddleware gates the public listener's API routes. Anything not
// under /api/ (the static SPA shell, and /stream/ which self-guards) passes
// through; open auth/config endpoints pass; otherwise the request needs a valid
// session, the guest toggle, or a valid signed token for its path (a shared
// stream's ffmpeg source fetches /api/tracks/{id}/audio this way). This is the production
// gate; it wraps ONLY the public http.Server, never the federation MemberHandler.
func (s *Server) RequireAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") || isOpenPath(r.URL.Path) || s.adminRouteAllowed(r) || s.mediaAllowed(r) || s.signedOK(r) {
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
		"/api/auth/mfa/complete",
		"/api/auth/me", "/api/auth/invite/accept",
		"/api/auth/profiles", "/api/auth/profiles/select",
		"/api/auth/verify-email",
		"/api/auth/password-reset/forgot", "/api/auth/password-reset/redeem",
		"/api/config":
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
