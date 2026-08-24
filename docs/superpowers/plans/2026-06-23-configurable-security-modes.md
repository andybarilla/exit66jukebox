# Configurable Security Modes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add persisted browser security modes so Exit66 can run as an open LAN jukebox, admin-locked public jukebox, passwordless household profile jukebox, or full-login deployment.

**Architecture:** Persist a parsed `store.SecurityMode` as the browser access source of truth, migrating from `guest_access_enabled` only when the new setting is absent. Keep browser gating in `RequireAuthMiddleware` around `srv.Handler()` so federation/member and peer handlers continue using raw `srv.Handler()`. Add passwordless profile account/session endpoints and let the SPA choose login, profile, admin, or open entry flows from `/api/config` without weakening existing admin checks.

**Tech Stack:** Go standard `net/http`, SQLite through `database/sql`, existing `internal/store` package, Svelte 5 runes, Vite, Vitest, mise-managed Go and npm commands.

## Global Constraints

- Security modes are exactly `open`, `open_admin_locked`, `household_profiles`, and `full_login`.
- Existing installs migrate as `guest_access_enabled = true` to `open_admin_locked` and `guest_access_enabled = false` to `full_login`.
- The mode is the source of truth after migration; keep or map `guest_access_enabled` only where compatibility requires it.
- Existing signup toggle remains and applies only to `full_login` account signup behavior.
- Passwordless profiles use the existing user table with a passwordless marker and create normal session cookies.
- `/admin` is the explicit admin entry point.
- Existing admin API checks remain authoritative.
- Protected shared house-stream controls require admin in `open_admin_locked` and `household_profiles`; in `open` normal queue controls remain open.
- `household_profiles` blocks the main UI until a profile session exists.
- Settings UI shows a trusted/private-network warning for `open`, `open_admin_locked`, and `household_profiles`; `full_login` is public-exposure-ready.
- Federation/member handler must remain raw `srv.Handler()`; browser auth must not move inside `Handler`.
- Verification commands must use `mise exec -- go test ./...`, `mise exec -- npm --prefix web run build`, and because `web/package.json` has `"test": "vitest run"`, `mise exec -- npm --prefix web run test`.

---

## File Structure

- `internal/store/settings.go`: Owns persisted security-mode parsing, migration from guest access, compatibility mapping for old guest access callers, and admin/signup/MFA flags.
- `internal/store/settings_test.go`: Verifies mode defaults, exact enum parsing, guest migration, and compatibility behavior.
- `internal/store/users.go`: Adds passwordless profile marker support on existing `user` rows and profile-specific list/create helpers.
- `internal/store/users_test.go`: Verifies passwordless profile persistence, listing, and normal-user compatibility.
- `internal/api/auth.go`: Uses `store.SecurityMode` for browser auth decisions, full-login signup restrictions, passwordless profile session creation, and `/api/auth/me` profile fields.
- `internal/api/auth_test.go`: Verifies auth middleware decisions, signup toggle behavior, passwordless profile sessions, and household profile blocking.
- `internal/api/admin.go`: Makes shared-house destructive controls mode-aware while preserving authoritative admin checks.
- `internal/api/adminusers.go`: Reads/writes `security_mode` from admin settings and reports passwordless profile fields in user lists.
- `internal/api/adminusers_test.go`: Verifies admin settings mode changes and mode-aware protected shared controls.
- `internal/api/config.go`: Exposes safe public mode metadata needed by the SPA.
- `internal/api/config_test.go`: Verifies `/api/config` output for all modes.
- `internal/api/server.go`: Registers `/api/auth/profiles`, `/api/auth/profiles/select`, and `/admin` SPA index route while keeping API admin routes under `requireAdmin`.
- `internal/api/server_test.go`: Verifies `/admin` serves the SPA entry and raw handler behavior used by federation/member paths remains unchanged.
- `main.go`: Keeps `MemberHandler: srv.Handler()` and `fed.PeerRoutes(db, srv.Handler())`, with browser auth only at `publicHandler := srv.RequireAuthMiddleware(srv.Handler())`.
- `web/src/lib/auth.js`: Adds profile list/create/select helpers.
- `web/src/lib/auth.test.js`: Verifies profile helper request paths and payloads.
- `web/src/lib/api.js`: Updates config shape comments for `security_mode` and entry-flow flags.
- `web/src/lib/store.svelte.js`: Stores security mode, profile requirement, admin route state, and profile session metadata from `/api/config` and `/api/auth/me`.
- `web/src/lib/store.test.js`: Verifies config parsing and derived access booleans.
- `web/src/lib/settingsPanelState.js`: Includes `securityMode` in the admin settings dirty-state snapshot.
- `web/src/lib/settingsPanelState.test.js`: Verifies mode participates in settings snapshots.
- `web/src/lib/components/ProfilePicker.svelte`: New household-profile entry component for listing, creating, and selecting passwordless profiles.
- `web/src/lib/components/ProfilePicker.test.js`: Source-level test for helper imports and submit/select wiring.
- `web/src/lib/components/AdminPanel.svelte`: Replaces guest-access toggle UI with mode choices, explanations, and trusted/private-network warning while keeping signup toggle visible for `full_login`.
- `web/src/lib/components/AdminPanel.test.js`: Verifies mode settings UI, warning copy, and settings payload wiring.
- `web/src/lib/components/TopBar.svelte`: Routes settings action to `/admin` and preserves login action for non-admin flows.
- `web/src/App.svelte`: Adds explicit `/admin` route behavior, household profile blocking before profile session, and full-login/open entry decisions.
- `web/src/App.test.js`: Source-level test for route and flow wiring.

## Tasks

### Task 1: Persist Security Mode and Migrate Guest Access

**Files:**
- Modify: `internal/store/settings.go`
- Modify: `internal/store/settings_test.go`

**Interfaces:**
- Consumes: Existing `meta` table helpers `metaFlag(db *sql.DB, key string) bool` and `setMetaFlag(db *sql.DB, key string, on bool) error`.
- Produces: `type SecurityMode string`, constants `SecurityModeOpen`, `SecurityModeOpenAdminLocked`, `SecurityModeHouseholdProfiles`, `SecurityModeFullLogin`, `ParseSecurityMode(value string) (SecurityMode, error)`, `SecurityModeSetting(db *sql.DB) SecurityMode`, `SetSecurityMode(db *sql.DB, mode SecurityMode) error`, `SecurityModeAllowsAnonymous(mode SecurityMode) bool`, and compatibility `GuestAccessEnabled(db *sql.DB) bool` mapped from mode.

- [ ] **Step 1: Write failing store tests for default mode and exact parsing**

  Add these tests to `internal/store/settings_test.go`:

  ```go
  func TestSecurityModeDefaultsToFullLogin(t *testing.T) {
      db := setupTestDB(t)

      if got := SecurityModeSetting(db); got != SecurityModeFullLogin {
          t.Fatalf("default security mode = %q, want %q", got, SecurityModeFullLogin)
      }
  }

  func TestParseSecurityModeAcceptsOnlyApprovedModes(t *testing.T) {
      valid := []SecurityMode{SecurityModeOpen, SecurityModeOpenAdminLocked, SecurityModeHouseholdProfiles, SecurityModeFullLogin}
      for _, mode := range valid {
          got, err := ParseSecurityMode(string(mode))
          if err != nil {
              t.Fatalf("ParseSecurityMode(%q) returned error: %v", mode, err)
          }
          if got != mode {
              t.Fatalf("ParseSecurityMode(%q) = %q", mode, got)
          }
      }

      if _, err := ParseSecurityMode("guest"); err == nil {
          t.Fatalf("ParseSecurityMode accepted an unsupported mode")
      }
  }
  ```

- [ ] **Step 2: Run store tests and verify the new symbols are missing**

  Run: `mise exec -- go test ./internal/store`

  Expected: FAIL with compile errors mentioning `SecurityModeSetting`, `SecurityModeFullLogin`, `ParseSecurityMode`, `SecurityModeOpen`, `SecurityModeOpenAdminLocked`, and `SecurityModeHouseholdProfiles` are undefined.

- [ ] **Step 3: Implement security mode type, parser, getter, and setter**

  Update `internal/store/settings.go` with these additions near the existing key constants and helpers:

  ```go
  import (
      "crypto/rand"
      "database/sql"
      "fmt"
  )

  const (
      keySignupEnabled    = "signup_enabled"
      keyGuestAccess      = "guest_access_enabled"
      keySecurityMode     = "security_mode"
      keyAdminMFARequired = "admin_mfa_required"
  )

  type SecurityMode string

  const (
      SecurityModeOpen              SecurityMode = "open"
      SecurityModeOpenAdminLocked   SecurityMode = "open_admin_locked"
      SecurityModeHouseholdProfiles SecurityMode = "household_profiles"
      SecurityModeFullLogin         SecurityMode = "full_login"
  )

  func ParseSecurityMode(value string) (SecurityMode, error) {
      switch SecurityMode(value) {
      case SecurityModeOpen, SecurityModeOpenAdminLocked, SecurityModeHouseholdProfiles, SecurityModeFullLogin:
          return SecurityMode(value), nil
      default:
          return "", fmt.Errorf("unsupported security mode %q", value)
      }
  }

  func metaText(db *sql.DB, key string) (string, bool) {
      var v string
      err := db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
      if err != nil {
          return "", false
      }
      return v, true
  }

  func setMetaText(db *sql.DB, key, value string) error {
      _, err := db.Exec(
          `INSERT INTO meta(key, value) VALUES(?, ?)
           ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
          key, value)
      return err
  }

  func SecurityModeSetting(db *sql.DB) SecurityMode {
      if raw, ok := metaText(db, keySecurityMode); ok {
          mode, err := ParseSecurityMode(raw)
          if err == nil {
              return mode
          }
      }
      if metaFlag(db, keyGuestAccess) {
          return SecurityModeOpenAdminLocked
      }
      return SecurityModeFullLogin
  }

  func SetSecurityMode(db *sql.DB, mode SecurityMode) error {
      parsed, err := ParseSecurityMode(string(mode))
      if err != nil {
          return err
      }
      return setMetaText(db, keySecurityMode, string(parsed))
  }

  func SecurityModeAllowsAnonymous(mode SecurityMode) bool {
      switch mode {
      case SecurityModeOpen, SecurityModeOpenAdminLocked:
          return true
      case SecurityModeHouseholdProfiles, SecurityModeFullLogin:
          return false
      default:
          return false
      }
  }
  ```

  Replace `GuestAccessEnabled` and `SetGuestAccessEnabled` with compatibility implementations:

  ```go
  func GuestAccessEnabled(db *sql.DB) bool {
      return SecurityModeAllowsAnonymous(SecurityModeSetting(db))
  }

  func SetGuestAccessEnabled(db *sql.DB, on bool) error {
      if on {
          return SetSecurityMode(db, SecurityModeOpenAdminLocked)
      }
      return SetSecurityMode(db, SecurityModeFullLogin)
  }
  ```

- [ ] **Step 4: Run store tests and verify the first tests pass**

  Run: `mise exec -- go test ./internal/store`

  Expected: PASS for `TestSecurityModeDefaultsToFullLogin` and `TestParseSecurityModeAcceptsOnlyApprovedModes`; existing guest-access tests may fail if they assert raw guest flag behavior.

- [ ] **Step 5: Add migration and compatibility tests**

  Add these tests to `internal/store/settings_test.go`:

  ```go
  func TestSecurityModeMigratesFromGuestAccessWhenModeMissing(t *testing.T) {
      db := setupTestDB(t)

      if err := setMetaFlag(db, keyGuestAccess, true); err != nil {
          t.Fatalf("set legacy guest access: %v", err)
      }
      if got := SecurityModeSetting(db); got != SecurityModeOpenAdminLocked {
          t.Fatalf("guest enabled migrated to %q, want %q", got, SecurityModeOpenAdminLocked)
      }

      db = setupTestDB(t)
      if err := setMetaFlag(db, keyGuestAccess, false); err != nil {
          t.Fatalf("set legacy guest access: %v", err)
      }
      if got := SecurityModeSetting(db); got != SecurityModeFullLogin {
          t.Fatalf("guest disabled migrated to %q, want %q", got, SecurityModeFullLogin)
      }
  }

  func TestSecurityModeOverridesLegacyGuestAccess(t *testing.T) {
      db := setupTestDB(t)
      if err := setMetaFlag(db, keyGuestAccess, true); err != nil {
          t.Fatalf("set legacy guest access: %v", err)
      }
      if err := SetSecurityMode(db, SecurityModeHouseholdProfiles); err != nil {
          t.Fatalf("set security mode: %v", err)
      }

      if got := SecurityModeSetting(db); got != SecurityModeHouseholdProfiles {
          t.Fatalf("security mode = %q, want %q", got, SecurityModeHouseholdProfiles)
      }
      if GuestAccessEnabled(db) {
          t.Fatalf("legacy guest access should map from household_profiles as false")
      }
  }
  ```

- [ ] **Step 6: Run store package tests**

  Run: `mise exec -- go test ./internal/store`

  Expected: PASS.

- [ ] **Step 7: Commit Task 1**

  ```bash
  git add internal/store/settings.go internal/store/settings_test.go
  git commit -m "feat: persist security mode setting"
  ```

### Task 2: Add Passwordless Profile Storage

**Files:**
- Modify: `internal/store/users.go`
- Modify: `internal/store/users_test.go`

**Interfaces:**
- Consumes: Existing `store.User`, `CreateUser`, `GetUserByEmail`, `GetUserByID`, `ListUsers`, and session helpers.
- Produces: `User.IsPasswordlessProfile bool`, `const PasswordlessPasswordHash = "passwordless"`, `CreatePasswordlessProfile(db *sql.DB, displayName string) (int64, error)`, and `ListPasswordlessProfiles(db *sql.DB) ([]User, error)`.

- [ ] **Step 1: Write failing tests for passwordless profile persistence**

  Create or update `internal/store/users_test.go` with:

  ```go
  func TestCreatePasswordlessProfileUsesExistingUserTable(t *testing.T) {
      db := setupTestDB(t)

      id, err := CreatePasswordlessProfile(db, "Casey")
      if err != nil {
          t.Fatalf("CreatePasswordlessProfile: %v", err)
      }

      user, ok, err := GetUserByID(db, id)
      if err != nil {
          t.Fatalf("GetUserByID: %v", err)
      }
      if !ok {
          t.Fatalf("created profile user was not found")
      }
      if user.Email != "profile-1@passwordless.local" {
          t.Fatalf("profile email = %q", user.Email)
      }
      if user.DisplayName != "Casey" {
          t.Fatalf("display name = %q", user.DisplayName)
      }
      if user.PasswordHash != PasswordlessPasswordHash {
          t.Fatalf("password hash = %q", user.PasswordHash)
      }
      if !user.IsPasswordlessProfile {
          t.Fatalf("profile marker was not loaded")
      }
      if user.IsAdmin {
          t.Fatalf("passwordless profile must not be admin")
      }
  }

  func TestListPasswordlessProfilesExcludesPasswordUsers(t *testing.T) {
      db := setupTestDB(t)

      if _, err := CreateUser(db, "admin@example.com", "Admin", "hash", true); err != nil {
          t.Fatalf("CreateUser: %v", err)
      }
      firstID, err := CreatePasswordlessProfile(db, "Avery")
      if err != nil {
          t.Fatalf("CreatePasswordlessProfile first: %v", err)
      }
      secondID, err := CreatePasswordlessProfile(db, "Blair")
      if err != nil {
          t.Fatalf("CreatePasswordlessProfile second: %v", err)
      }

      profiles, err := ListPasswordlessProfiles(db)
      if err != nil {
          t.Fatalf("ListPasswordlessProfiles: %v", err)
      }
      if len(profiles) != 2 {
          t.Fatalf("len(profiles) = %d, want 2", len(profiles))
      }
      if profiles[0].ID != firstID || profiles[1].ID != secondID {
          t.Fatalf("profile ids = %d,%d want %d,%d", profiles[0].ID, profiles[1].ID, firstID, secondID)
      }
  }
  ```

- [ ] **Step 2: Run user store tests and verify missing symbols**

  Run: `mise exec -- go test ./internal/store`

  Expected: FAIL with undefined `CreatePasswordlessProfile`, `PasswordlessPasswordHash`, `User.IsPasswordlessProfile`, and `ListPasswordlessProfiles`.

- [ ] **Step 3: Add passwordless profile marker to user storage**

  Update `internal/store/users.go`:

  ```go
  const PasswordlessPasswordHash = "passwordless"

  type User struct {
      ID                    int64
      Email                 string
      DisplayName           string
      PasswordHash          string
      IsAdmin               bool
      IsPasswordlessProfile bool
      CreatedAt             int64
      EmailVerifiedAt       int64
  }
  ```

  Change `CreateUser`, `CreateUnverifiedUserWithEmailVerification`, `GetUserByEmail`, `GetUserByID`, `ListUsers`, and `scanUser` SQL to include `is_passwordless_profile`, defaulting to `0` for normal users. Add a compatibility migration helper called at the top of each user function that touches the column:

  ```go
  func ensurePasswordlessProfileColumn(db *sql.DB) error {
      _, err := db.Exec(`ALTER TABLE user ADD COLUMN is_passwordless_profile INTEGER NOT NULL DEFAULT 0`)
      if err == nil {
          return nil
      }
      return nil
  }
  ```

  Use this exact profile creator:

  ```go
  func CreatePasswordlessProfile(db *sql.DB, displayName string) (int64, error) {
      if err := ensurePasswordlessProfileColumn(db); err != nil {
          return 0, err
      }
      res, err := db.Exec(
          `INSERT INTO user(email, display_name, password_hash, is_admin, is_passwordless_profile, created_at, email_verified_at)
           VALUES('', ?, ?, 0, 1, strftime('%s','now'), strftime('%s','now'))`,
          displayName, PasswordlessPasswordHash)
      if err != nil {
          return 0, err
      }
      id, err := res.LastInsertId()
      if err != nil {
          return 0, err
      }
      email := fmt.Sprintf("profile-%d@passwordless.local", id)
      if _, err := db.Exec(`UPDATE user SET email = ? WHERE id = ?`, email, id); err != nil {
          return 0, err
      }
      return id, nil
  }
  ```

  Add:

  ```go
  func ListPasswordlessProfiles(db *sql.DB) ([]User, error) {
      if err := ensurePasswordlessProfileColumn(db); err != nil {
          return nil, err
      }
      rows, err := db.Query(
          `SELECT id, email, display_name, password_hash, is_admin, is_passwordless_profile, created_at, email_verified_at
           FROM user WHERE is_passwordless_profile = 1 ORDER BY created_at, id`)
      if err != nil {
          return nil, err
      }
      defer rows.Close()
      var out []User
      for rows.Next() {
          u, err := scanUserRow(rows)
          if err != nil {
              return nil, err
          }
          out = append(out, u)
      }
      return out, rows.Err()
  }
  ```

  Add a private scanner used by `ListUsers` and `ListPasswordlessProfiles`:

  ```go
  type userScanner interface {
      Scan(dest ...any) error
  }

  func scanUserRow(row userScanner) (User, error) {
      var u User
      var admin int
      var passwordless int
      err := row.Scan(&u.ID, &u.Email, &u.DisplayName, &u.PasswordHash, &admin, &passwordless, &u.CreatedAt, &u.EmailVerifiedAt)
      if err != nil {
          return User{}, err
      }
      u.IsAdmin = admin != 0
      u.IsPasswordlessProfile = passwordless != 0
      return u, nil
  }
  ```

- [ ] **Step 4: Run store tests**

  Run: `mise exec -- go test ./internal/store`

  Expected: PASS.

- [ ] **Step 5: Commit Task 2**

  ```bash
  git add internal/store/users.go internal/store/users_test.go
  git commit -m "feat: store passwordless profiles"
  ```

### Task 3: Mode-Based Browser Auth and Signup Decisions

**Files:**
- Modify: `internal/api/auth.go`
- Modify: `internal/api/auth_test.go`
- Modify: `internal/api/server_test.go`
- Modify: `main.go`

**Interfaces:**
- Consumes: `store.SecurityModeSetting(db) store.SecurityMode`, `store.SecurityModeAllowsAnonymous(mode) bool`, `store.SecurityModeFullLogin`, `store.SecurityModeHouseholdProfiles`, and existing session helpers.
- Produces: `func (s *Server) browserAccessAllowed(r *http.Request) bool` behavior inside `requireAuth`; signup allowed after bootstrap only in `full_login` when `signup_enabled` is true; tests asserting `main.go` keeps federation handlers raw.

- [ ] **Step 1: Write failing auth middleware table tests**

  Add to `internal/api/auth_test.go`:

  ```go
  func TestRequireAuthUsesSecurityModeForBrowserAccess(t *testing.T) {
      cases := []struct {
          name string
          mode store.SecurityMode
          want int
      }{
          {"open", store.SecurityModeOpen, http.StatusOK},
          {"open admin locked", store.SecurityModeOpenAdminLocked, http.StatusOK},
          {"household profiles", store.SecurityModeHouseholdProfiles, http.StatusUnauthorized},
          {"full login", store.SecurityModeFullLogin, http.StatusUnauthorized},
      }

      for _, tc := range cases {
          t.Run(tc.name, func(t *testing.T) {
              db := setupAPITestDB(t)
              if err := store.SetSecurityMode(db, tc.mode); err != nil {
                  t.Fatalf("SetSecurityMode: %v", err)
              }
              s := NewServer(db, nil, nil)
              h := s.requireAuth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

              rec := httptest.NewRecorder()
              h(rec, httptest.NewRequest(http.MethodGet, "/api/tracks", nil))

              if rec.Code != tc.want {
                  t.Fatalf("status = %d, want %d", rec.Code, tc.want)
              }
          })
      }
  }
  ```

- [ ] **Step 2: Write failing signup mode tests**

  Add to `internal/api/auth_test.go`:

  ```go
  func TestSignupToggleAppliesOnlyToFullLogin(t *testing.T) {
      cases := []struct {
          name string
          mode store.SecurityMode
          want int
      }{
          {"open", store.SecurityModeOpen, http.StatusForbidden},
          {"open admin locked", store.SecurityModeOpenAdminLocked, http.StatusForbidden},
          {"household profiles", store.SecurityModeHouseholdProfiles, http.StatusForbidden},
          {"full login", store.SecurityModeFullLogin, http.StatusServiceUnavailable},
      }

      for _, tc := range cases {
          t.Run(tc.name, func(t *testing.T) {
              db := setupAPITestDB(t)
              if _, err := store.CreateUser(db, "admin@example.com", "Admin", "hash", true); err != nil {
                  t.Fatalf("CreateUser: %v", err)
              }
              if err := store.SetSignupEnabled(db, true); err != nil {
                  t.Fatalf("SetSignupEnabled: %v", err)
              }
              if err := store.SetSecurityMode(db, tc.mode); err != nil {
                  t.Fatalf("SetSecurityMode: %v", err)
              }
              s := NewServer(db, nil, nil)

              rec := httptest.NewRecorder()
              body := strings.NewReader(`{"email":"new@example.com","display_name":"New","password":"password123"}`)
              s.signup(rec, httptest.NewRequest(http.MethodPost, "/api/auth/signup", body))

              if rec.Code != tc.want {
                  t.Fatalf("status = %d, want %d body=%s", rec.Code, tc.want, rec.Body.String())
              }
          })
      }
  }
  ```

- [ ] **Step 3: Write raw handler preservation tests**

  Add to `internal/api/server_test.go`:

  ```go
  func TestRawHandlerDoesNotApplyBrowserAuth(t *testing.T) {
      db := setupAPITestDB(t)
      if err := store.SetSecurityMode(db, store.SecurityModeFullLogin); err != nil {
          t.Fatalf("SetSecurityMode: %v", err)
      }
      srv := NewServer(db, nil, nil)

      rec := httptest.NewRecorder()
      srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))

      if rec.Code != http.StatusOK {
          t.Fatalf("raw Handler status = %d, want 200", rec.Code)
      }
  }

  func TestPublicHandlerAppliesBrowserAuthAroundRawHandler(t *testing.T) {
      db := setupAPITestDB(t)
      if err := store.SetSecurityMode(db, store.SecurityModeFullLogin); err != nil {
          t.Fatalf("SetSecurityMode: %v", err)
      }
      srv := NewServer(db, nil, nil)

      rec := httptest.NewRecorder()
      srv.RequireAuthMiddleware(srv.Handler()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tracks", nil))

      if rec.Code != http.StatusUnauthorized {
          t.Fatalf("public handler status = %d, want 401", rec.Code)
      }
  }
  ```

- [ ] **Step 4: Run API tests and verify failures**

  Run: `mise exec -- go test ./internal/api`

  Expected: FAIL because `requireAuth` still uses `GuestAccessEnabled` and signup does not check the mode.

- [ ] **Step 5: Implement mode decisions in auth**

  Replace the top comment and body of `requireAuth` in `internal/api/auth.go` with:

  ```go
  // requireAuth gates browser API routes. A valid session always passes. Anonymous
  // browser access passes only in open modes; household_profiles and full_login
  // require a session before the main API is usable.
  func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
      return func(w http.ResponseWriter, r *http.Request) {
          if _, ok := s.currentUser(r); ok {
              next(w, r)
              return
          }
          mode := store.SecurityModeSetting(s.db)
          if store.SecurityModeAllowsAnonymous(mode) {
              next(w, r)
              return
          }
          writeErr(w, http.StatusUnauthorized, "login required")
      }
  }
  ```

  In `signup`, after `bootstrap := n == 0`, replace the signup toggle check with:

  ```go
      if !bootstrap && store.SecurityModeSetting(s.db) != store.SecurityModeFullLogin {
          writeErr(w, http.StatusForbidden, "signup is available only in full_login mode")
          return
      }
      if !bootstrap && !store.SignupEnabled(s.db) {
          writeErr(w, http.StatusForbidden, "signup is disabled")
          return
      }
  ```

- [ ] **Step 6: Confirm federation/member raw handler code is untouched**

  Inspect `main.go` and confirm these exact lines still exist:

  ```go
  MemberHandler: srv.Handler(),
  fm.PeerHandler = fed.PeerRoutes(db, srv.Handler())
  publicHandler := srv.RequireAuthMiddleware(srv.Handler())
  ```

  Do not move browser auth into `Server.Handler()`.

- [ ] **Step 7: Run API tests**

  Run: `mise exec -- go test ./internal/api`

  Expected: PASS.

- [ ] **Step 8: Commit Task 3**

  ```bash
  git add internal/api/auth.go internal/api/auth_test.go internal/api/server_test.go main.go
  git commit -m "feat: gate browser access by security mode"
  ```

### Task 4: Admin Settings, Public Config, and Mode-Aware Protected Controls

**Files:**
- Modify: `internal/api/config.go`
- Modify: `internal/api/config_test.go`
- Modify: `internal/api/admin.go`
- Modify: `internal/api/adminusers.go`
- Modify: `internal/api/adminusers_test.go`

**Interfaces:**
- Consumes: `store.SecurityModeSetting`, `store.SetSecurityMode`, `store.SecurityModeAllowsAnonymous`, existing `requireAdmin`, and current admin settings endpoints.
- Produces: `/api/config` fields `security_mode`, `requires_profile`, `requires_login`, and compatibility `guest_access`; admin settings `security_mode`; `requireAdminShared` that allows house destructive controls in `open` and requires admin in `open_admin_locked` and `household_profiles`.

- [ ] **Step 1: Write failing config tests for all modes**

  Update `internal/api/config_test.go` with:

  ```go
  func TestConfigExposesSecurityModeEntryFlow(t *testing.T) {
      cases := []struct {
          mode            store.SecurityMode
          guestAccess     bool
          requiresProfile bool
          requiresLogin   bool
      }{
          {store.SecurityModeOpen, true, false, false},
          {store.SecurityModeOpenAdminLocked, true, false, false},
          {store.SecurityModeHouseholdProfiles, false, true, false},
          {store.SecurityModeFullLogin, false, false, true},
      }

      for _, tc := range cases {
          t.Run(string(tc.mode), func(t *testing.T) {
              db := setupAPITestDB(t)
              if err := store.SetSecurityMode(db, tc.mode); err != nil {
                  t.Fatalf("SetSecurityMode: %v", err)
              }
              srv := NewServer(db, nil, nil)
              rec := httptest.NewRecorder()

              srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))

              if rec.Code != http.StatusOK {
                  t.Fatalf("status = %d", rec.Code)
              }
              var got map[string]any
              if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
                  t.Fatalf("json: %v", err)
              }
              if got["security_mode"] != string(tc.mode) {
                  t.Fatalf("security_mode = %v, want %s", got["security_mode"], tc.mode)
              }
              if got["guest_access"] != tc.guestAccess {
                  t.Fatalf("guest_access = %v, want %v", got["guest_access"], tc.guestAccess)
              }
              if got["requires_profile"] != tc.requiresProfile {
                  t.Fatalf("requires_profile = %v, want %v", got["requires_profile"], tc.requiresProfile)
              }
              if got["requires_login"] != tc.requiresLogin {
                  t.Fatalf("requires_login = %v, want %v", got["requires_login"], tc.requiresLogin)
              }
          })
      }
  }
  ```

- [ ] **Step 2: Write failing admin settings tests**

  Add to `internal/api/adminusers_test.go`:

  ```go
  func TestAdminSettingsReadsAndWritesSecurityMode(t *testing.T) {
      db := setupAPITestDB(t)
      s := NewServer(db, nil, nil)

      req := adminReq(t, db, http.MethodPost, "/api/admin/settings", `{"security_mode":"household_profiles"}`)
      rec := httptest.NewRecorder()
      s.Handler().ServeHTTP(rec, req)

      if rec.Code != http.StatusOK {
          t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
      }
      if got := store.SecurityModeSetting(db); got != store.SecurityModeHouseholdProfiles {
          t.Fatalf("security mode = %q", got)
      }
      if !strings.Contains(rec.Body.String(), `"security_mode":"household_profiles"`) {
          t.Fatalf("response missing security mode: %s", rec.Body.String())
      }
  }

  func TestAdminSettingsRejectsUnsupportedSecurityMode(t *testing.T) {
      db := setupAPITestDB(t)
      s := NewServer(db, nil, nil)

      req := adminReq(t, db, http.MethodPost, "/api/admin/settings", `{"security_mode":"guest"}`)
      rec := httptest.NewRecorder()
      s.Handler().ServeHTTP(rec, req)

      if rec.Code != http.StatusBadRequest {
          t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
      }
  }
  ```

- [ ] **Step 3: Write failing protected house control tests**

  Add to `internal/api/adminusers_test.go`:

  ```go
  func TestHouseControlsAreModeAware(t *testing.T) {
      cases := []struct {
          mode store.SecurityMode
          want int
      }{
          {store.SecurityModeOpen, http.StatusOK},
          {store.SecurityModeOpenAdminLocked, http.StatusUnauthorized},
          {store.SecurityModeHouseholdProfiles, http.StatusUnauthorized},
          {store.SecurityModeFullLogin, http.StatusUnauthorized},
      }

      for _, tc := range cases {
          t.Run(string(tc.mode), func(t *testing.T) {
              db := setupAPITestDB(t)
              if err := store.SetSecurityMode(db, tc.mode); err != nil {
                  t.Fatalf("SetSecurityMode: %v", err)
              }
              srv := NewServer(db, nil, nil)
              rec := httptest.NewRecorder()

              req := httptest.NewRequest(http.MethodPost, "/api/streams/house/shuffle", strings.NewReader(`{"shuffle":true}`))
              srv.Handler().ServeHTTP(rec, req)

              if rec.Code != tc.want {
                  t.Fatalf("status = %d, want %d body=%s", rec.Code, tc.want, rec.Body.String())
              }
          })
      }
  }
  ```

- [ ] **Step 4: Run API tests and verify failures**

  Run: `mise exec -- go test ./internal/api`

  Expected: FAIL on missing `security_mode` fields and current house controls being admin-gated in `open`.

- [ ] **Step 5: Implement public config mode output**

  In `internal/api/config.go`, compute mode once and add fields:

  ```go
      mode := store.SecurityModeSetting(s.db)
      writeJSON(w, http.StatusOK, map[string]any{
          "mute_local_on_cast": s.muteLocalOnCast,
          "fed_peers":          peers,
          "authenticated":      authed,
          "is_admin":           authed && u.IsAdmin,
          "security_mode":      string(mode),
          "guest_access":       store.SecurityModeAllowsAnonymous(mode),
          "requires_profile":   mode == store.SecurityModeHouseholdProfiles && !authed,
          "requires_login":     mode == store.SecurityModeFullLogin && !authed,
          "signup_enabled":     mode == store.SecurityModeFullLogin && store.SignupEnabled(s.db),
          "needs_bootstrap":    countUsersZero(s.db),
      })
  ```

- [ ] **Step 6: Implement admin settings mode fields**

  In `internal/api/adminusers.go`, add `"security_mode": string(store.SecurityModeSetting(s.db))` to `getAdminSettings`.

  Change `adminSettingsReq` to include:

  ```go
  SecurityMode *string `json:"security_mode"`
  ```

  In `setAdminSettings`, before signup/guest/MFA handling, add:

  ```go
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
  ```

  Keep `GuestAccessEnabled` request support for compatibility by mapping it through `store.SetGuestAccessEnabled`.

- [ ] **Step 7: Implement mode-aware shared-house control gate**

  Replace `requireAdminShared` in `internal/api/admin.go` with:

  ```go
  func (s *Server) requireAdminShared(next http.HandlerFunc) http.HandlerFunc {
      gated := s.requireAdmin(next)
      return func(w http.ResponseWriter, r *http.Request) {
          if r.PathValue("id") != sharedStreamID {
              next(w, r)
              return
          }
          mode := store.SecurityModeSetting(s.db)
          if mode == store.SecurityModeOpen {
              next(w, r)
              return
          }
          gated(w, r)
      }
  }
  ```

- [ ] **Step 8: Run API tests**

  Run: `mise exec -- go test ./internal/api`

  Expected: PASS.

- [ ] **Step 9: Commit Task 4**

  ```bash
  git add internal/api/config.go internal/api/config_test.go internal/api/admin.go internal/api/adminusers.go internal/api/adminusers_test.go
  git commit -m "feat: expose and enforce security modes"
  ```

### Task 5: Passwordless Profile Session API

**Files:**
- Modify: `internal/api/auth.go`
- Modify: `internal/api/auth_test.go`
- Modify: `internal/api/server.go`
- Modify: `internal/api/adminusers.go`
- Modify: `internal/api/adminusers_test.go`

**Interfaces:**
- Consumes: `store.CreatePasswordlessProfile`, `store.ListPasswordlessProfiles`, `store.GetUserByID`, `Server.setSessionCookie`, and `store.SecurityModeHouseholdProfiles`.
- Produces: `GET /api/auth/profiles`, `POST /api/auth/profiles`, `POST /api/auth/profiles/select`, profile response shape `{id, display_name}`, and `me`/admin user responses with `is_passwordless_profile`.

- [ ] **Step 1: Write failing passwordless profile API tests**

  Add to `internal/api/auth_test.go`:

  ```go
  func TestPasswordlessProfileCreationAndSelectionCreatesSession(t *testing.T) {
      db := setupAPITestDB(t)
      if err := store.SetSecurityMode(db, store.SecurityModeHouseholdProfiles); err != nil {
          t.Fatalf("SetSecurityMode: %v", err)
      }
      srv := NewServer(db, nil, nil)

      create := httptest.NewRecorder()
      srv.Handler().ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/auth/profiles", strings.NewReader(`{"display_name":"Casey"}`)))
      if create.Code != http.StatusOK {
          t.Fatalf("create status = %d body=%s", create.Code, create.Body.String())
      }
      var created map[string]any
      if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
          t.Fatalf("json: %v", err)
      }

      selectBody := fmt.Sprintf(`{"id":%.0f}`, created["id"].(float64))
      selectRec := httptest.NewRecorder()
      srv.Handler().ServeHTTP(selectRec, httptest.NewRequest(http.MethodPost, "/api/auth/profiles/select", strings.NewReader(selectBody)))
      if selectRec.Code != http.StatusOK {
          t.Fatalf("select status = %d body=%s", selectRec.Code, selectRec.Body.String())
      }
      if len(selectRec.Result().Cookies()) == 0 {
          t.Fatalf("select did not set a session cookie")
      }

      meReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
      meReq.AddCookie(selectRec.Result().Cookies()[0])
      meRec := httptest.NewRecorder()
      srv.Handler().ServeHTTP(meRec, meReq)
      if meRec.Code != http.StatusOK {
          t.Fatalf("me status = %d body=%s", meRec.Code, meRec.Body.String())
      }
      if !strings.Contains(meRec.Body.String(), `"is_passwordless_profile":true`) {
          t.Fatalf("me missing passwordless marker: %s", meRec.Body.String())
      }
  }

  func TestPasswordlessProfileEndpointsRequireHouseholdProfilesMode(t *testing.T) {
      db := setupAPITestDB(t)
      if err := store.SetSecurityMode(db, store.SecurityModeOpen); err != nil {
          t.Fatalf("SetSecurityMode: %v", err)
      }
      srv := NewServer(db, nil, nil)

      rec := httptest.NewRecorder()
      srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/auth/profiles", strings.NewReader(`{"display_name":"Casey"}`)))

      if rec.Code != http.StatusForbidden {
          t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
      }
  }
  ```

- [ ] **Step 2: Write failing admin user response marker test**

  Add to `internal/api/adminusers_test.go`:

  ```go
  func TestAdminUserListIncludesPasswordlessProfileMarker(t *testing.T) {
      db := setupAPITestDB(t)
      profileID, err := store.CreatePasswordlessProfile(db, "Casey")
      if err != nil {
          t.Fatalf("CreatePasswordlessProfile: %v", err)
      }
      s := NewServer(db, nil, nil)

      rec := httptest.NewRecorder()
      s.Handler().ServeHTTP(rec, adminReq(t, db, http.MethodGet, "/api/admin/users", ""))

      if rec.Code != http.StatusOK {
          t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
      }
      if !strings.Contains(rec.Body.String(), fmt.Sprintf(`"id":%d`, profileID)) {
          t.Fatalf("response missing profile id: %s", rec.Body.String())
      }
      if !strings.Contains(rec.Body.String(), `"is_passwordless_profile":true`) {
          t.Fatalf("response missing passwordless marker: %s", rec.Body.String())
      }
  }
  ```

- [ ] **Step 3: Run API tests and verify route failures**

  Run: `mise exec -- go test ./internal/api`

  Expected: FAIL with profile routes returning 404 or missing fields.

- [ ] **Step 4: Implement profile request/response handlers**

  Add to `internal/api/auth.go`:

  ```go
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
  ```

  If `userJSON` does not exist, add this helper and use it from `me`, login, and profile select responses:

  ```go
  func userJSON(u store.User) map[string]any {
      return map[string]any{
          "id": u.ID, "email": u.Email, "display_name": u.DisplayName,
          "is_admin": u.IsAdmin, "email_verified": u.EmailVerifiedAt != 0,
          "is_passwordless_profile": u.IsPasswordlessProfile,
      }
  }
  ```

- [ ] **Step 5: Register profile routes and admin route SPA entry**

  In `internal/api/server.go`, add before logout/me routes:

  ```go
  mux.HandleFunc("GET /api/auth/profiles", s.listPasswordlessProfiles)
  mux.HandleFunc("POST /api/auth/profiles", s.createPasswordlessProfile)
  mux.HandleFunc("POST /api/auth/profiles/select", s.selectPasswordlessProfile)
  ```

  In the `if s.ui != nil` block, add:

  ```go
  mux.HandleFunc("GET /admin", s.serveUIIndex)
  ```

- [ ] **Step 6: Add passwordless marker to admin list users**

  In `internal/api/adminusers.go` `listUsers`, add to each user map:

  ```go
  "is_passwordless_profile": u.IsPasswordlessProfile,
  ```

- [ ] **Step 7: Run API tests**

  Run: `mise exec -- go test ./internal/api`

  Expected: PASS.

- [ ] **Step 8: Commit Task 5**

  ```bash
  git add internal/api/auth.go internal/api/auth_test.go internal/api/server.go internal/api/adminusers.go internal/api/adminusers_test.go
  git commit -m "feat: add passwordless profile sessions"
  ```

### Task 6: Frontend API and Store Mode State

**Files:**
- Modify: `web/src/lib/auth.js`
- Modify: `web/src/lib/auth.test.js`
- Modify: `web/src/lib/api.js`
- Modify: `web/src/lib/store.svelte.js`
- Modify: `web/src/lib/store.test.js`
- Modify: `web/src/lib/settingsPanelState.js`
- Modify: `web/src/lib/settingsPanelState.test.js`

**Interfaces:**
- Consumes: Backend `/api/config`, `/api/auth/me`, `/api/auth/profiles`, `/api/auth/profiles/select`, and `/api/admin/settings` shapes from Tasks 4-5.
- Produces: `listProfiles()`, `createProfile(displayName)`, `selectProfile(id)`, store config fields `securityMode`, `requiresProfile`, `requiresLogin`, `guestAccess`, `signupEnabled`, and settings snapshot field `securityMode`.

- [ ] **Step 1: Write failing auth helper tests**

  Add to `web/src/lib/auth.test.js`:

  ```js
  it('lists passwordless profiles', async () => {
    fetch.mockReturnValue(jsonResp([{ id: 1, display_name: 'Casey' }]));
    const profiles = await listProfiles();
    expect(fetch.mock.calls[0][0]).toBe('/api/auth/profiles');
    expect(profiles[0].display_name).toBe('Casey');
  });

  it('creates a passwordless profile', async () => {
    fetch.mockReturnValue(jsonResp({ id: 2, display_name: 'Blair' }));
    await createProfile('Blair');
    const [url, options] = fetch.mock.calls[0];
    expect(url).toBe('/api/auth/profiles');
    expect(JSON.parse(options.body)).toEqual({ display_name: 'Blair' });
  });

  it('selects a passwordless profile', async () => {
    fetch.mockReturnValue(jsonResp({ id: 2, display_name: 'Blair', is_passwordless_profile: true }));
    await selectProfile(2);
    const [url, options] = fetch.mock.calls[0];
    expect(url).toBe('/api/auth/profiles/select');
    expect(JSON.parse(options.body)).toEqual({ id: 2 });
  });
  ```

  Update the import at the top of the test to include `listProfiles`, `createProfile`, and `selectProfile`.

- [ ] **Step 2: Write failing store config tests**

  Add to `web/src/lib/store.test.js`:

  ```js
  it('parses security mode entry-flow config', async () => {
    fetch.mockImplementation((url) => {
      if (url === '/api/config') return Promise.resolve({ ok: true, json: () => Promise.resolve({ security_mode: 'household_profiles', guest_access: false, requires_profile: true, requires_login: false, signup_enabled: false, needs_bootstrap: false, authenticated: false }) });
      if (url === '/api/auth/me') return Promise.resolve({ ok: false, json: () => Promise.resolve({}) });
      return Promise.resolve({ ok: true, json: () => Promise.resolve([]) });
    });
    const s = createStore();

    await s.bootstrap();

    expect(s.config.securityMode).toBe('household_profiles');
    expect(s.config.requiresProfile).toBe(true);
    expect(s.config.requiresLogin).toBe(false);
    expect(s.config.guestAccess).toBe(false);
  });
  ```

- [ ] **Step 3: Write failing settings snapshot test**

  Add to `web/src/lib/settingsPanelState.test.js`:

  ```js
  test('includes security mode in editable settings snapshot', () => {
    const snapshot = buildEditableSettingsSnapshot({
      signupEnabled: true,
      guestAccess: false,
      securityMode: 'open_admin_locked',
      adminMfaRequired: false,
      libraries: [],
      federation: { enabled: false, role: '', hub_addr: '', listen: '', token: '', peer_id: '', token_configured: false, restart_required: false },
      scan: { assume_same_title_folder_compilations: false },
    });

    expect(snapshot).toContain('open_admin_locked');
  });
  ```

- [ ] **Step 4: Run frontend tests and verify failures**

  Run: `mise exec -- npm --prefix web run test`

  Expected: FAIL due to missing exports and missing store/snapshot fields.

- [ ] **Step 5: Implement profile auth helpers**

  Add to `web/src/lib/auth.js`:

  ```js
  export const listProfiles = () => fetch('/api/auth/profiles').then((r) => r.json());
  export const createProfile = (display_name) => postJSON('/api/auth/profiles', { display_name });
  export const selectProfile = (id) => postJSON('/api/auth/profiles/select', { id });
  ```

- [ ] **Step 6: Update config API comment**

  In `web/src/lib/api.js`, replace the `getConfig` return comment with:

  ```js
  // return shape includes { mute_local_on_cast, fed_peers, authenticated, is_admin,
  // security_mode, guest_access, requires_profile, requires_login, signup_enabled,
  // needs_bootstrap }
  ```

- [ ] **Step 7: Update store config state parsing**

  In `web/src/lib/store.svelte.js`, change the initial config object to include:

  ```js
  let config = $state({
    muteLocalOnCast: true,
    fedPeers: [],
    securityMode: 'full_login',
    guestAccess: false,
    requiresProfile: false,
    requiresLogin: true,
    signupEnabled: false,
    needsBootstrap: false,
    authenticated: false,
  });
  ```

  In `bootstrap`, map config fields exactly:

  ```js
  config = {
    muteLocalOnCast: typeof c.mute_local_on_cast === 'boolean' ? c.mute_local_on_cast : config.muteLocalOnCast,
    fedPeers: Array.isArray(c.fed_peers) ? c.fed_peers : [],
    securityMode: c.security_mode || 'full_login',
    guestAccess: !!c.guest_access,
    requiresProfile: !!c.requires_profile,
    requiresLogin: !!c.requires_login,
    signupEnabled: !!c.signup_enabled,
    needsBootstrap: !!c.needs_bootstrap,
    authenticated: !!c.authenticated,
  };
  ```

  Ensure `me` objects can carry `is_passwordless_profile` without being stripped by `setMe`.

- [ ] **Step 8: Include securityMode in settings dirty snapshots**

  In `web/src/lib/settingsPanelState.js`, include `securityMode` alongside `signupEnabled`, `guestAccess`, and `adminMfaRequired` when building editable snapshots.

- [ ] **Step 9: Run frontend tests**

  Run: `mise exec -- npm --prefix web run test`

  Expected: PASS.

- [ ] **Step 10: Commit Task 6**

  ```bash
  git add web/src/lib/auth.js web/src/lib/auth.test.js web/src/lib/api.js web/src/lib/store.svelte.js web/src/lib/store.test.js web/src/lib/settingsPanelState.js web/src/lib/settingsPanelState.test.js
  git commit -m "feat: add frontend security mode state"
  ```

### Task 7: Frontend Settings, Admin Route, and Household Profile Flows

**Files:**
- Create: `web/src/lib/components/ProfilePicker.svelte`
- Create: `web/src/lib/components/ProfilePicker.test.js`
- Modify: `web/src/lib/components/AdminPanel.svelte`
- Modify: `web/src/lib/components/AdminPanel.test.js`
- Modify: `web/src/lib/components/TopBar.svelte`
- Modify: `web/src/App.svelte`
- Create: `web/src/App.test.js`

**Interfaces:**
- Consumes: Store config fields from Task 6, auth helpers `listProfiles`, `createProfile`, `selectProfile`, admin `setSettings({ security_mode })`, and existing `Login`, `Signup`, `AdminPanel`, and `TopBar` props.
- Produces: Admin mode choice UI, trusted/private-network warning, `/admin` route that opens admin login or settings, and `ProfilePicker` that blocks main UI until a profile session exists in `household_profiles`.

- [ ] **Step 1: Write failing ProfilePicker source test**

  Create `web/src/lib/components/ProfilePicker.test.js`:

  ```js
  import { describe, expect, test } from 'vitest';
  import { readFileSync } from 'node:fs';

  const source = readFileSync(new URL('./ProfilePicker.svelte', import.meta.url), 'utf8');

  describe('ProfilePicker wiring', () => {
    test('imports profile helpers', () => {
      expect(source).toMatch(/import\s*{[^}]*\blistProfiles\b[^}]*\bcreateProfile\b[^}]*\bselectProfile\b[^}]*}\s*from\s*'\.\.\/auth\.js'/s);
    });

    test('creates and selects passwordless profiles', () => {
      expect(source).toContain('await createProfile(displayName.trim())');
      expect(source).toContain('await selectProfile(profile.id)');
      expect(source).toContain('onLoggedIn?.(user)');
    });
  });
  ```

- [ ] **Step 2: Add failing AdminPanel mode UI tests**

  Add to `web/src/lib/components/AdminPanel.test.js`:

  ```js
  test('renders security mode choices and trusted network warning', () => {
    expect(source).toContain('Security mode');
    expect(source).toContain('open_admin_locked');
    expect(source).toContain('household_profiles');
    expect(source).toContain('full_login');
    expect(source).toContain('trusted/private networks');
  });

  test('saves security mode through admin settings endpoint', () => {
    expect(source).toContain('securityMode = $state');
    expect(functionBody('currentEditableSettingsState')).toMatch(/securityMode/);
    expect(functionBody('onChangeSecurityMode')).toMatch(/setSettings\s*\(\s*{\s*security_mode:\s*v\s*}\s*\)/);
  });
  ```

- [ ] **Step 3: Add failing App route/flow tests**

  Create `web/src/App.test.js`:

  ```js
  import { describe, expect, test } from 'vitest';
  import { readFileSync } from 'node:fs';

  const source = readFileSync(new URL('./App.svelte', import.meta.url), 'utf8');

  describe('App security mode routing', () => {
    test('imports and renders ProfilePicker for household profile blocking', () => {
      expect(source).toMatch(/import\s+ProfilePicker\s+from\s+'\.\/lib\/components\/ProfilePicker\.svelte'/);
      expect(source).toContain('s.config.requiresProfile');
      expect(source).toContain('<ProfilePicker');
    });

    test('treats /admin as explicit admin entry point', () => {
      expect(source).toContain("currentPath === '/admin'");
      expect(source).toContain('adminPanelOpen = true');
      expect(source).toContain("replaceRoute('/admin')");
    });
  });
  ```

- [ ] **Step 4: Run frontend tests and verify failures**

  Run: `mise exec -- npm --prefix web run test`

  Expected: FAIL because `ProfilePicker.svelte` and new mode/admin-route wiring do not exist.

- [ ] **Step 5: Create ProfilePicker component**

  Create `web/src/lib/components/ProfilePicker.svelte`:

  ```svelte
  <script>
    import { onMount } from 'svelte';
    import { listProfiles, createProfile, selectProfile } from '../auth.js';

    let { onLoggedIn } = $props();
    let profiles = $state([]);
    let displayName = $state('');
    let loading = $state(true);
    let busy = $state(false);
    let error = $state('');

    onMount(async () => {
      try {
        profiles = await listProfiles();
      } catch (err) {
        error = err.message || 'failed to load profiles';
      } finally {
        loading = false;
      }
    });

    async function chooseProfile(profile) {
      busy = true;
      error = '';
      try {
        const user = await selectProfile(profile.id);
        onLoggedIn?.(user);
      } catch (err) {
        error = err.message || 'failed to select profile';
      } finally {
        busy = false;
      }
    }

    async function submitProfile(event) {
      event.preventDefault();
      if (!displayName.trim()) {
        error = 'name is required';
        return;
      }
      busy = true;
      error = '';
      try {
        const profile = await createProfile(displayName.trim());
        profiles = [...profiles, profile];
        displayName = '';
        const user = await selectProfile(profile.id);
        onLoggedIn?.(user);
      } catch (err) {
        error = err.message || 'failed to create profile';
      } finally {
        busy = false;
      }
    }
  </script>

  <section style="min-height:100vh; display:flex; align-items:center; justify-content:center; padding:24px; background:var(--grid-glow), var(--bg-base); color:var(--text-body); box-sizing:border-box;">
    <div style="width:min(560px, 100%); border:1.5px solid var(--neon-cyan); border-radius:var(--radius-lg); background:var(--bg-surface); background-image:var(--scanline); box-shadow:var(--shadow-xl), var(--glow-soft-cyan); padding:24px;">
      <h1 style="font-family:var(--font-display); margin:0 0 8px; letter-spacing:0.06em; text-transform:uppercase;">Choose your profile</h1>
      <p style="margin:0 0 20px; color:var(--text-muted);">Pick a household profile before using the jukebox.</p>
      {#if error}<p style="color:var(--neon-magenta);">{error}</p>{/if}
      {#if loading}
        <p>Loading profiles…</p>
      {:else}
        <div style="display:grid; gap:10px; margin-bottom:20px;">
          {#each profiles as profile (profile.id)}
            <button disabled={busy} onclick={() => chooseProfile(profile)} style="padding:14px 16px; text-align:left; border:1px solid var(--border-strong); border-radius:var(--radius-md); background:var(--bg-surface-raised); color:var(--text-body); font-family:var(--font-display); cursor:pointer;">{profile.display_name}</button>
          {/each}
        </div>
        <form onsubmit={submitProfile} style="display:flex; gap:10px;">
          <input bind:value={displayName} placeholder="New profile name" style="flex:1; min-width:0; padding:12px; border-radius:var(--radius-sm); border:1px solid var(--border-strong); background:var(--bg-base); color:var(--text-body);" />
          <button disabled={busy} style="padding:0 16px; border:none; border-radius:var(--radius-sm); background:var(--neon-cyan); color:var(--text-on-accent); font-family:var(--font-display); font-weight:700;">Create</button>
        </form>
      {/if}
    </div>
  </section>
  ```

- [ ] **Step 6: Add AdminPanel security mode state and controls**

  In `web/src/lib/components/AdminPanel.svelte`:

  - Add `let securityMode = $state('full_login');` near `signupEnabled`.
  - Include `securityMode` in `currentEditableSettingsState()`.
  - When settings load, set `securityMode = settings.security_mode || 'full_login';`.
  - Add this handler:

  ```js
  async function onChangeSecurityMode(v) {
    accessError = '';
    securityMode = v;
    updateUnsavedState();
    const r = await setSettings({ security_mode: v });
    securityMode = r.security_mode || v;
    updateCleanSettingsSnapshot({ securityMode });
  }
  ```

  - Add a mode selector section in the settings/access area:

  ```svelte
  <section>
    <h3>Security mode</h3>
    <div style="display:grid; gap:8px;">
      {#each [
        { value: 'open', label: 'Open', help: 'Frictionless trusted household jukebox. Normal queue controls are open.' },
        { value: 'open_admin_locked', label: 'Open, admin locked', help: 'Public jukebox playback with settings and protected house controls behind /admin.' },
        { value: 'household_profiles', label: 'Household profiles', help: 'Visitors choose or create a passwordless profile before using the jukebox.' },
        { value: 'full_login', label: 'Full login', help: 'Password accounts are required before app access.' },
      ] as option}
        <label style="display:block; padding:10px; border:1px solid var(--border-default); border-radius:var(--radius-sm);">
          <input type="radio" name="security-mode" value={option.value} checked={securityMode === option.value} onchange={() => onChangeSecurityMode(option.value)} />
          <strong>{option.label}</strong>
          <span style="display:block; color:var(--text-muted);">{option.help}</span>
        </label>
      {/each}
    </div>
    {#if securityMode !== 'full_login'}
      <p style="color:var(--neon-amber);">Open, admin-locked, and household profile modes are intended for trusted/private networks. Use full_login for public exposure.</p>
    {/if}
  </section>
  ```

  Keep the signup toggle visible but label it as applying to `full_login`; disable it outside `full_login` with explanatory text if the component already has a disabled switch pattern.

- [ ] **Step 7: Wire `/admin` explicit route and profile blocking in App**

  In `web/src/App.svelte`:

  - Import `ProfilePicker`.
  - Add `const onAdminPath = $derived(currentPath === '/admin');`.
  - After `await s.bootstrap();`, start the store only when `s.me || s.config.guestAccess` and `!s.config.requiresProfile`.
  - Add an effect:

  ```js
  $effect(() => {
    if (onAdminPath && s.authChecked && s.isAdmin) adminPanelOpen = true;
  });
  ```

  - Add a handler:

  ```js
  function openAdminRoute() {
    replaceRoute('/admin');
    if (s.isAdmin) adminPanelOpen = true;
    else showAuth = true;
  }
  ```

  - In the render branch, place household profile blocking before login blocking:

  ```svelte
  {:else if s.config.requiresProfile && !s.me}
    <ProfilePicker onLoggedIn={afterLogin} />
  {:else if onAdminPath && !s.isAdmin}
    <Login canSignup={false} onSwitchToSignup={() => (showSignup = false)} onLoggedIn={afterLogin} />
  {:else if (!s.me && s.config.requiresLogin) || showAuth}
  ```

  - Change `TopBar` settings prop to `onOpenSettings={openAdminRoute}`.
  - When closing `AdminPanel`, use `onClose={() => { adminPanelOpen = false; if (onAdminPath) replaceRoute('/'); }}`.

- [ ] **Step 8: Update TopBar settings link behavior if needed**

  In `web/src/lib/components/TopBar.svelte`, keep the existing `onOpenSettings` prop but ensure the settings button title or accessible label says `Admin settings` and calls `onOpenSettings` without directly opening an overlay. The exact existing button handler should remain:

  ```svelte
  <button onclick={onOpenSettings} title="Admin settings"
  ```

- [ ] **Step 9: Run frontend tests**

  Run: `mise exec -- npm --prefix web run test`

  Expected: PASS.

- [ ] **Step 10: Run frontend build**

  Run: `mise exec -- npm --prefix web run build`

  Expected: PASS with Vite build output ending successfully and no Svelte compile errors.

- [ ] **Step 11: Commit Task 7**

  ```bash
  git add web/src/lib/components/ProfilePicker.svelte web/src/lib/components/ProfilePicker.test.js web/src/lib/components/AdminPanel.svelte web/src/lib/components/AdminPanel.test.js web/src/lib/components/TopBar.svelte web/src/App.svelte web/src/App.test.js
  git commit -m "feat: add security mode frontend flows"
  ```

### Task 8: Full Verification and Regression Pass

**Files:**
- Modify only files needed to fix regressions discovered by the commands below.

**Interfaces:**
- Consumes: All interfaces produced by Tasks 1-7.
- Produces: Passing backend tests, frontend tests, frontend build, and preserved federation/member raw handler behavior.

- [ ] **Step 1: Run all Go tests**

  Run: `mise exec -- go test ./...`

  Expected: PASS for every Go package.

- [ ] **Step 2: Run frontend tests**

  Run: `mise exec -- npm --prefix web run test`

  Expected: PASS with Vitest completing all test files.

- [ ] **Step 3: Run frontend build**

  Run: `mise exec -- npm --prefix web run build`

  Expected: PASS with Vite build output and no Svelte compile errors.

- [ ] **Step 4: Verify raw federation/member handler behavior by inspection**

  Run: `mise exec -- go test ./internal/api -run 'TestRawHandlerDoesNotApplyBrowserAuth|TestPublicHandlerAppliesBrowserAuthAroundRawHandler'`

  Expected: PASS, proving raw `srv.Handler()` remains ungated while `RequireAuthMiddleware(srv.Handler())` gates browser traffic.

  Inspect `main.go` and confirm these exact lines still exist:

  ```go
  MemberHandler: srv.Handler(),
  fm.PeerHandler = fed.PeerRoutes(db, srv.Handler())
  publicHandler := srv.RequireAuthMiddleware(srv.Handler())
  ```

- [ ] **Step 5: Fix any regressions with targeted tests first**

  For each failing command, write or update the smallest relevant test from the failing package, rerun that package command, then rerun the full failing command. Do not change public mode names, migration mapping, admin authority, or raw federation/member handler placement.

- [ ] **Step 6: Commit verification fixes if any files changed**

  If Step 5 changed files:

  ```bash
  git add <changed files from Step 5>
  git commit -m "fix: resolve security mode regressions"
  ```

  If no files changed, do not create an empty commit.

### Task 9: PR Body and Project Board Final Workflow

**Files:**
- No code files.

**Interfaces:**
- Consumes: Verified branch from Tasks 1-8 and issue `#122`.
- Produces: PR body including `Closes #122` and project board item moved to In Review after PR creation.

- [ ] **Step 1: Inspect final status and recent commits**

  Run:

  ```bash
  git status --short
  git log --oneline -10
  ```

  Expected: working tree is clean except for intentional uncommitted changes already handled by Task 8; recent commits include Tasks 1-7 and optional Task 8 fix commit.

- [ ] **Step 2: Prepare PR body**

  Use this PR body content exactly as the starting point, editing only if verification output names differ:

  ```markdown
  ## Summary
  - add persisted security modes with migration from guest access
  - add mode-based browser auth, admin settings/config output, and passwordless household profiles
  - add frontend settings, /admin, and profile-selection flows

  ## Verification
  - mise exec -- go test ./...
  - mise exec -- npm --prefix web run test
  - mise exec -- npm --prefix web run build

  Closes #122
  ```

- [ ] **Step 3: Open PR when requested by the orchestrator**

  Run only when the orchestrator authorizes PR creation:

  ```bash
  gh pr create --fill --body-file <file containing the PR body from Step 2>
  ```

  Expected: command prints a GitHub PR URL.

- [ ] **Step 4: Move project board item to In Review after PR creation**

  After the PR exists, use the repository's project-board workflow tooling or `gh project item-edit` commands to move issue `#122` to `In Review`.

  Expected: issue `#122` board status is `In Review`.

## Self-Review

- **Spec coverage:** Tasks cover persisted modes, exact enum values, guest-access migration, source-of-truth mode decisions, signup limited to `full_login`, passwordless profiles on existing users/sessions, `/admin` entry, authoritative admin APIs, protected house controls, `/api/config`, frontend settings warning, household profile blocking, raw federation/member handler preservation, verification commands, PR body `Closes #122`, and board In Review workflow.
- **Placeholder scan:** This plan contains no `TBD`, `TODO`, “implement later”, “add tests” without test content, or “similar to” shortcuts. Each task lists exact files, commands, expected outputs, and commit steps.
- **Type consistency:** Backend mode names use `store.SecurityMode` and constants defined in Task 1; frontend mode strings match the approved exact values. Produced route names and JSON fields are reused consistently across backend tests, frontend helpers, store state, and UI tasks.
