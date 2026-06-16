# User Authentication Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Put the internet-exposed hub behind real per-user accounts (local email + password) with cookie sessions, an invite system, enable/disable signup, and a Sonos signed-URL carve-out — replacing the shared-password admin gate.

**Architecture:** Three credential paths — browser uses an httpOnly session cookie, Sonos uses a short-lived HMAC-signed media URL, federation keeps its existing peer-token handshake. Pure crypto helpers live in a new `internal/auth` package; user/session/invite/settings persistence lives in `internal/store`; HTTP middleware + handlers live in `internal/api`. Auth middleware wraps **only** the public listener in `main.go`, never the federation `MemberHandler`.

**Tech Stack:** Go 1.26 stdlib only (`crypto/pbkdf2`, `crypto/hmac`, `crypto/sha256`, `crypto/rand`, `net/smtp`), modernc SQLite, Svelte 5 + Vite + Vitest frontend.

**Design:** `docs/superpowers/specs/2026-06-16-user-authentication-design.md`

---

## File Structure

**New files:**
- `internal/auth/password.go` — `HashPassword`/`VerifyPassword` (pbkdf2-sha256, encoded string).
- `internal/auth/password_test.go`
- `internal/auth/token.go` — `GenerateToken`/`HashToken` (random session/invite tokens + sha256 storage hash).
- `internal/auth/token_test.go`
- `internal/auth/sign.go` — `SignMedia`/`VerifyMedia` (HMAC-signed, expiring, track-scoped media token).
- `internal/auth/sign_test.go`
- `internal/store/users.go` — user CRUD.
- `internal/store/users_test.go`
- `internal/store/sessions.go` — session CRUD.
- `internal/store/sessions_test.go`
- `internal/store/invites.go` — invite CRUD.
- `internal/store/invites_test.go`
- `internal/store/settings.go` — `signup_enabled` / `guest_access_enabled` flags + media signing secret (via `meta`).
- `internal/store/settings_test.go`
- `internal/api/auth.go` — cookie helpers, session middleware, login/signup/logout/me/invite-accept, login throttle.
- `internal/api/auth_test.go`
- `internal/api/adminusers.go` — admin settings/invites/users handlers.
- `internal/api/adminusers_test.go`
- `internal/email/email.go` — optional SMTP invite sender.
- `internal/email/email_test.go`
- `web/src/lib/auth.js` — frontend auth API calls.
- `web/src/lib/auth.test.js`
- `web/src/lib/components/Login.svelte`, `Signup.svelte`, `InviteAccept.svelte`, `AdminPanel.svelte`.

**Modified files:**
- `internal/store/schema.sql` — add `user`, `session`, `invite` tables.
- `internal/api/server.go` — replace admin-password fields with a store-backed session model; add signing secret + settings accessors.
- `internal/api/admin.go` — repoint `requireAdmin`/`requireAdminShared`/`isAdmin` to sessions + guest toggle; remove shared-password login.
- `internal/api/config.go` — surface `is_admin`, `authenticated`, `guest_access`, `signup_enabled`.
- `internal/api/audio.go` — accept a signed media token as an alternative to a session.
- `internal/api/sonos.go` — build signed audio URLs for casts.
- `internal/config/config.go` — drop `AdminPassword`; add SMTP env config.
- `main.go` — wire store-backed auth, signing secret, SMTP; wrap only the public handler; remove `SetAdminPassword`.
- `web/src/lib/api.js` — drop bearer-token machinery; rely on cookies.
- `web/src/lib/store.svelte.js` — auth state (current user, authenticated, settings).
- `web/src/App.svelte` — gate the app on auth; route to Login/Signup/InviteAccept; mount AdminPanel.

---

## Phase A — Pure crypto helpers (`internal/auth`)

### Task 1: Password hashing

**Files:**
- Create: `internal/auth/password.go`
- Test: `internal/auth/password_test.go`

- [ ] **Step 1: Write the failing test**

```go
package auth

import (
	"strings"
	"testing"
)

func TestHashVerifyRoundTrip(t *testing.T) {
	h, err := HashPassword("hunter2")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(h, "pbkdf2-sha256$") {
		t.Fatalf("unexpected format: %s", h)
	}
	if !VerifyPassword("hunter2", h) {
		t.Fatal("correct password rejected")
	}
	if VerifyPassword("wrong", h) {
		t.Fatal("wrong password accepted")
	}
}

func TestHashSaltsDiffer(t *testing.T) {
	a, _ := HashPassword("same")
	b, _ := HashPassword("same")
	if a == b {
		t.Fatal("identical hashes — salt not random")
	}
}

func TestVerifyMalformed(t *testing.T) {
	if VerifyPassword("x", "garbage") {
		t.Fatal("malformed hash accepted")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/auth/ -run TestHash -v`
Expected: FAIL — `undefined: HashPassword`.

- [ ] **Step 3: Write minimal implementation**

```go
// Package auth holds pure, dependency-free credential helpers: password
// hashing, random token generation, and signed media URLs. No DB or HTTP here.
package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// pbkdf2Iter is the work factor. Stored in each hash so it can be raised later
// without invalidating existing hashes.
const pbkdf2Iter = 600_000

// HashPassword returns a self-describing pbkdf2-sha256 hash:
// "pbkdf2-sha256$<iter>$<salt-b64>$<dk-b64>".
func HashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	dk, err := pbkdf2.Key(sha256.New, pw, salt, pbkdf2Iter, 32)
	if err != nil {
		return "", err
	}
	b64 := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pbkdf2Iter, b64(salt), b64(dk)), nil
}

// VerifyPassword reports whether pw matches the encoded hash. A malformed hash
// returns false rather than erroring — callers treat both as auth failure.
func VerifyPassword(pw, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/auth/ -run TestHash -v && go test ./internal/auth/ -run TestVerify -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/password.go internal/auth/password_test.go
git commit -m "feat(auth): pbkdf2-sha256 password hashing"
```

---

### Task 2: Token generation + storage hash

**Files:**
- Create: `internal/auth/token.go`
- Test: `internal/auth/token_test.go`

- [ ] **Step 1: Write the failing test**

```go
package auth

import "testing"

func TestGenerateTokenUnique(t *testing.T) {
	a, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := GenerateToken()
	if a == b || len(a) < 32 {
		t.Fatalf("weak/duplicate token: %q %q", a, b)
	}
}

func TestHashTokenStable(t *testing.T) {
	if HashToken("abc") != HashToken("abc") {
		t.Fatal("hash not stable")
	}
	if HashToken("abc") == HashToken("abd") {
		t.Fatal("hash collision")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/auth/ -run TestGenerateToken -v`
Expected: FAIL — `undefined: GenerateToken`.

- [ ] **Step 3: Write minimal implementation**

```go
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// GenerateToken returns a 256-bit random hex token for sessions and invites.
func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// HashToken returns the sha256 hex of a raw token. Only the hash is stored, so a
// DB leak can't be replayed into a live session or invite.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/auth/ -run "TestGenerateToken|TestHashToken" -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/token.go internal/auth/token_test.go
git commit -m "feat(auth): random tokens + sha256 storage hash"
```

---

### Task 3: Signed media URLs

**Files:**
- Create: `internal/auth/sign.go`
- Test: `internal/auth/sign_test.go`

- [ ] **Step 1: Write the failing test**

```go
package auth

import "testing"

func TestSignVerifyMedia(t *testing.T) {
	secret := []byte("server-secret")
	tok := SignMedia(secret, 42, 1_000_000)
	if id, ok := VerifyMedia(secret, tok, 999_000); !ok || id != 42 {
		t.Fatalf("valid token rejected: id=%d ok=%v", id, ok)
	}
}

func TestVerifyMediaExpired(t *testing.T) {
	secret := []byte("s")
	tok := SignMedia(secret, 1, 100)
	if _, ok := VerifyMedia(secret, tok, 101); ok {
		t.Fatal("expired token accepted")
	}
}

func TestVerifyMediaTampered(t *testing.T) {
	secret := []byte("s")
	tok := SignMedia(secret, 1, 1_000_000)
	if _, ok := VerifyMedia([]byte("other"), tok, 1); ok {
		t.Fatal("forged token accepted under wrong secret")
	}
	if _, ok := VerifyMedia(secret, tok+"x", 1); ok {
		t.Fatal("mutated token accepted")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/auth/ -run TestSign -v`
Expected: FAIL — `undefined: SignMedia`.

- [ ] **Step 3: Write minimal implementation**

```go
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
)

// SignMedia returns a token authorizing access to trackID until expUnix (unix
// seconds). Format: "<trackID>.<exp>.<sig-b64>", where sig = HMAC(secret,
// "<trackID>.<exp>"). Used for Sonos casts, which fetch audio with no cookie.
func SignMedia(secret []byte, trackID, expUnix int64) string {
	msg := strconv.FormatInt(trackID, 10) + "." + strconv.FormatInt(expUnix, 10)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(msg))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return msg + "." + sig
}

// VerifyMedia checks a token against secret and the current time nowUnix,
// returning the authorized trackID. ok is false for malformed, forged, or
// expired tokens.
func VerifyMedia(secret []byte, token string, nowUnix int64) (trackID int64, ok bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return 0, false
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, false
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, false
	}
	msg := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(msg))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(parts[2])) {
		return 0, false
	}
	if nowUnix >= exp {
		return 0, false
	}
	return id, true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/auth/ -v`
Expected: PASS (all auth tests).

- [ ] **Step 5: Commit**

```bash
git add internal/auth/sign.go internal/auth/sign_test.go
git commit -m "feat(auth): HMAC-signed expiring media URLs for casts"
```

---

## Phase B — Store layer (`internal/store`)

### Task 4: Schema — user/session/invite tables

**Files:**
- Modify: `internal/store/schema.sql`
- Test: `internal/store/auth_schema_test.go` (create)

- [ ] **Step 1: Write the failing test**

```go
package store

import "testing"

func TestAuthTablesExist(t *testing.T) {
	db := openTestDB(t) // existing test helper; see store_test.go
	for _, tbl := range []string{"user", "session", "invite"} {
		var n int
		err := db.QueryRow(
			`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, tbl,
		).Scan(&n)
		if err != nil || n != 1 {
			t.Fatalf("table %q missing (n=%d err=%v)", tbl, n, err)
		}
	}
}
```

Note: confirm the test DB helper name in `internal/store/store_test.go`; if it differs from `openTestDB`, use the existing one (likely `store.Open(":memory:")`).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestAuthTablesExist -v`
Expected: FAIL — tables missing.

- [ ] **Step 3: Append to `internal/store/schema.sql`**

```sql
CREATE TABLE IF NOT EXISTS user (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    email         TEXT NOT NULL UNIQUE,
    display_name  TEXT NOT NULL DEFAULT '',
    password_hash TEXT NOT NULL,
    is_admin      INTEGER NOT NULL DEFAULT 0,
    created_at    INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS session (
    token_hash TEXT PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES user(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_session_user ON session(user_id);
CREATE TABLE IF NOT EXISTS invite (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    token_hash  TEXT NOT NULL UNIQUE,
    email       TEXT NOT NULL DEFAULT '',
    is_admin    INTEGER NOT NULL DEFAULT 0,
    created_by  INTEGER REFERENCES user(id),
    created_at  INTEGER NOT NULL,
    expires_at  INTEGER NOT NULL,
    accepted_at INTEGER NOT NULL DEFAULT 0
);
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestAuthTablesExist -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/schema.sql internal/store/auth_schema_test.go
git commit -m "feat(store): user/session/invite tables"
```

---

### Task 5: User CRUD

**Files:**
- Create: `internal/store/users.go`
- Test: `internal/store/users_test.go`

- [ ] **Step 1: Write the failing test**

```go
package store

import "testing"

func TestUserCreateGetCount(t *testing.T) {
	db := mustOpenMem(t) // store.Open(":memory:"); add helper if absent
	if n, _ := CountUsers(db); n != 0 {
		t.Fatalf("want 0 users, got %d", n)
	}
	id, err := CreateUser(db, "a@b.com", "Alice", "hash1", true)
	if err != nil {
		t.Fatal(err)
	}
	u, ok, err := GetUserByEmail(db, "a@b.com")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if u.ID != id || u.DisplayName != "Alice" || !u.IsAdmin || u.PasswordHash != "hash1" {
		t.Fatalf("bad user: %+v", u)
	}
	if n, _ := CountUsers(db); n != 1 {
		t.Fatalf("want 1 user, got %d", n)
	}
}

func TestUserDuplicateEmail(t *testing.T) {
	db := mustOpenMem(t)
	if _, err := CreateUser(db, "a@b.com", "A", "h", false); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateUser(db, "a@b.com", "A2", "h", false); err == nil {
		t.Fatal("duplicate email allowed")
	}
}

func TestListAndDeleteUser(t *testing.T) {
	db := mustOpenMem(t)
	id, _ := CreateUser(db, "a@b.com", "A", "h", true)
	if us, _ := ListUsers(db); len(us) != 1 {
		t.Fatalf("want 1, got %d", len(us))
	}
	if err := DeleteUser(db, id); err != nil {
		t.Fatal(err)
	}
	if n, _ := CountUsers(db); n != 0 {
		t.Fatalf("want 0 after delete, got %d", n)
	}
}
```

Add a shared helper at the top of `users_test.go` if the package lacks one:

```go
func mustOpenMem(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
```

(Check `store_test.go` first — reuse an existing helper instead of redefining if one exists, to avoid a duplicate-symbol compile error.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestUser -v`
Expected: FAIL — `undefined: CreateUser`.

- [ ] **Step 3: Write minimal implementation**

```go
package store

import "database/sql"

// User is an account row. PasswordHash is the encoded pbkdf2 string from
// internal/auth; this package never hashes or compares passwords itself.
type User struct {
	ID           int64
	Email        string
	DisplayName  string
	PasswordHash string
	IsAdmin      bool
	CreatedAt    int64
}

// CountUsers returns the number of accounts. Zero means the instance is
// uninitialized: the next signup bootstraps the first admin.
func CountUsers(db *sql.DB) (int, error) {
	var n int
	err := db.QueryRow(`SELECT count(*) FROM user`).Scan(&n)
	return n, err
}

// CreateUser inserts an account and returns its id. The UNIQUE(email)
// constraint surfaces as an error on a duplicate.
func CreateUser(db *sql.DB, email, displayName, passwordHash string, isAdmin bool) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO user(email, display_name, password_hash, is_admin, created_at)
		 VALUES(?,?,?,?,strftime('%s','now'))`,
		email, displayName, passwordHash, boolToInt(isAdmin))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetUserByEmail looks up an account by email. ok is false when none exists.
func GetUserByEmail(db *sql.DB, email string) (User, bool, error) {
	return scanUser(db.QueryRow(
		`SELECT id, email, display_name, password_hash, is_admin, created_at
		 FROM user WHERE email = ?`, email))
}

// GetUserByID looks up an account by id (used to resolve a session's user).
func GetUserByID(db *sql.DB, id int64) (User, bool, error) {
	return scanUser(db.QueryRow(
		`SELECT id, email, display_name, password_hash, is_admin, created_at
		 FROM user WHERE id = ?`, id))
}

// ListUsers returns all accounts ordered by creation.
func ListUsers(db *sql.DB) ([]User, error) {
	rows, err := db.Query(
		`SELECT id, email, display_name, password_hash, is_admin, created_at
		 FROM user ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		var admin int
		if err := rows.Scan(&u.ID, &u.Email, &u.DisplayName, &u.PasswordHash, &admin, &u.CreatedAt); err != nil {
			return nil, err
		}
		u.IsAdmin = admin != 0
		out = append(out, u)
	}
	return out, rows.Err()
}

// DeleteUser removes an account; its sessions cascade away (ON DELETE CASCADE).
func DeleteUser(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM user WHERE id = ?`, id)
	return err
}

func scanUser(row *sql.Row) (User, bool, error) {
	var u User
	var admin int
	err := row.Scan(&u.ID, &u.Email, &u.DisplayName, &u.PasswordHash, &admin, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	u.IsAdmin = admin != 0
	return u, true, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestUser -v && go test ./internal/store/ -run "TestListAndDeleteUser" -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/users.go internal/store/users_test.go
git commit -m "feat(store): user CRUD"
```

---

### Task 6: Session CRUD

**Files:**
- Create: `internal/store/sessions.go`
- Test: `internal/store/sessions_test.go`

- [ ] **Step 1: Write the failing test**

```go
package store

import "testing"

func TestSessionLifecycle(t *testing.T) {
	db := mustOpenMem(t)
	uid, _ := CreateUser(db, "a@b.com", "A", "h", true)
	// far-future expiry
	if err := CreateSession(db, "hash-token", uid, 4_000_000_000); err != nil {
		t.Fatal(err)
	}
	got, ok, err := UserBySession(db, "hash-token")
	if err != nil || !ok || got.ID != uid {
		t.Fatalf("lookup: ok=%v err=%v id=%d", ok, err, got.ID)
	}
	if err := DeleteSession(db, "hash-token"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := UserBySession(db, "hash-token"); ok {
		t.Fatal("session survived delete")
	}
}

func TestExpiredSessionRejected(t *testing.T) {
	db := mustOpenMem(t)
	uid, _ := CreateUser(db, "a@b.com", "A", "h", false)
	if err := CreateSession(db, "old", uid, 100); err != nil { // expired in 1970+
		t.Fatal(err)
	}
	if _, ok, _ := UserBySession(db, "old"); ok {
		t.Fatal("expired session accepted")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestSession -v`
Expected: FAIL — `undefined: CreateSession`.

- [ ] **Step 3: Write minimal implementation**

```go
package store

import "database/sql"

// CreateSession stores a session keyed by the sha256 hash of its token (raw
// token lives only in the user's cookie). expiresAt is unix seconds.
func CreateSession(db *sql.DB, tokenHash string, userID, expiresAt int64) error {
	_, err := db.Exec(
		`INSERT INTO session(token_hash, user_id, created_at, expires_at)
		 VALUES(?,?,strftime('%s','now'),?)`,
		tokenHash, userID, expiresAt)
	return err
}

// UserBySession resolves the (unexpired) session identified by tokenHash to its
// user. ok is false when the session is missing or expired.
func UserBySession(db *sql.DB, tokenHash string) (User, bool, error) {
	var uid int64
	err := db.QueryRow(
		`SELECT user_id FROM session
		 WHERE token_hash = ? AND expires_at > strftime('%s','now')`, tokenHash).Scan(&uid)
	if err == sql.ErrNoRows {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	return GetUserByID(db, uid)
}

// DeleteSession removes a session (logout). Idempotent.
func DeleteSession(db *sql.DB, tokenHash string) error {
	_, err := db.Exec(`DELETE FROM session WHERE token_hash = ?`, tokenHash)
	return err
}

// PurgeExpiredSessions deletes timed-out rows. Called opportunistically at
// startup so the table doesn't grow unbounded.
func PurgeExpiredSessions(db *sql.DB) error {
	_, err := db.Exec(`DELETE FROM session WHERE expires_at <= strftime('%s','now')`)
	return err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestSession -v && go test ./internal/store/ -run TestExpired -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/sessions.go internal/store/sessions_test.go
git commit -m "feat(store): session CRUD with expiry"
```

---

### Task 7: Invite CRUD

**Files:**
- Create: `internal/store/invites.go`
- Test: `internal/store/invites_test.go`

- [ ] **Step 1: Write the failing test**

```go
package store

import "testing"

func TestInviteCreateConsume(t *testing.T) {
	db := mustOpenMem(t)
	admin, _ := CreateUser(db, "a@b.com", "A", "h", true)
	id, err := CreateInvite(db, "thash", "new@b.com", true, admin, 4_000_000_000)
	if err != nil {
		t.Fatal(err)
	}
	inv, ok, err := PendingInvite(db, "thash")
	if err != nil || !ok || inv.ID != id || !inv.IsAdmin || inv.Email != "new@b.com" {
		t.Fatalf("pending: %+v ok=%v err=%v", inv, ok, err)
	}
	if err := MarkInviteAccepted(db, id); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := PendingInvite(db, "thash"); ok {
		t.Fatal("accepted invite still pending (not single-use)")
	}
}

func TestExpiredInviteNotPending(t *testing.T) {
	db := mustOpenMem(t)
	admin, _ := CreateUser(db, "a@b.com", "A", "h", true)
	CreateInvite(db, "old", "", false, admin, 100)
	if _, ok, _ := PendingInvite(db, "old"); ok {
		t.Fatal("expired invite pending")
	}
}

func TestListRevokeInvite(t *testing.T) {
	db := mustOpenMem(t)
	admin, _ := CreateUser(db, "a@b.com", "A", "h", true)
	id, _ := CreateInvite(db, "t", "x@y.com", false, admin, 4_000_000_000)
	if invs, _ := ListInvites(db); len(invs) != 1 {
		t.Fatalf("want 1 invite, got %d", len(invs))
	}
	if err := DeleteInvite(db, id); err != nil {
		t.Fatal(err)
	}
	if invs, _ := ListInvites(db); len(invs) != 0 {
		t.Fatalf("want 0 after revoke, got %d", len(invs))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestInvite -v`
Expected: FAIL — `undefined: CreateInvite`.

- [ ] **Step 3: Write minimal implementation**

```go
package store

import "database/sql"

// Invite is an outstanding (or historical) invitation. Email is optional (a
// hint / SMTP recipient). IsAdmin means redemption yields an admin account.
type Invite struct {
	ID         int64
	Email      string
	IsAdmin    bool
	CreatedBy  int64
	CreatedAt  int64
	ExpiresAt  int64
	AcceptedAt int64
}

// CreateInvite stores an invite keyed by its token hash and returns its id.
func CreateInvite(db *sql.DB, tokenHash, email string, isAdmin bool, createdBy, expiresAt int64) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO invite(token_hash, email, is_admin, created_by, created_at, expires_at)
		 VALUES(?,?,?,?,strftime('%s','now'),?)`,
		tokenHash, email, boolToInt(isAdmin), createdBy, expiresAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// PendingInvite returns the unexpired, unaccepted invite for tokenHash. ok is
// false when it is missing, already accepted, or expired.
func PendingInvite(db *sql.DB, tokenHash string) (Invite, bool, error) {
	var inv Invite
	var admin int
	err := db.QueryRow(
		`SELECT id, email, is_admin, created_by, created_at, expires_at, accepted_at
		 FROM invite
		 WHERE token_hash = ? AND accepted_at = 0 AND expires_at > strftime('%s','now')`,
		tokenHash).Scan(&inv.ID, &inv.Email, &admin, &inv.CreatedBy, &inv.CreatedAt, &inv.ExpiresAt, &inv.AcceptedAt)
	if err == sql.ErrNoRows {
		return Invite{}, false, nil
	}
	if err != nil {
		return Invite{}, false, err
	}
	inv.IsAdmin = admin != 0
	return inv, true, nil
}

// MarkInviteAccepted stamps accepted_at so the invite can't be reused.
func MarkInviteAccepted(db *sql.DB, id int64) error {
	_, err := db.Exec(`UPDATE invite SET accepted_at = strftime('%s','now') WHERE id = ?`, id)
	return err
}

// ListInvites returns all invites (pending and historical), newest first.
func ListInvites(db *sql.DB) ([]Invite, error) {
	rows, err := db.Query(
		`SELECT id, email, is_admin, created_by, created_at, expires_at, accepted_at
		 FROM invite ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Invite
	for rows.Next() {
		var inv Invite
		var admin int
		if err := rows.Scan(&inv.ID, &inv.Email, &admin, &inv.CreatedBy, &inv.CreatedAt, &inv.ExpiresAt, &inv.AcceptedAt); err != nil {
			return nil, err
		}
		inv.IsAdmin = admin != 0
		out = append(out, inv)
	}
	return out, rows.Err()
}

// DeleteInvite revokes an invite by id.
func DeleteInvite(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM invite WHERE id = ?`, id)
	return err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestInvite -v && go test ./internal/store/ -run "TestExpiredInvite|TestListRevoke" -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/invites.go internal/store/invites_test.go
git commit -m "feat(store): invite CRUD (single-use, expiring)"
```

---

### Task 8: Settings + media signing secret (via `meta`)

**Files:**
- Create: `internal/store/settings.go`
- Test: `internal/store/settings_test.go`

The `meta` table is `(key TEXT PRIMARY KEY, value INTEGER NOT NULL)`. Boolean flags fit directly. The signing secret needs bytes, not an int, so it is stored in a tiny dedicated `app_secret` table created here via `CREATE TABLE IF NOT EXISTS` (executed lazily on first access, mirroring how the rest of the package treats the DB).

- [ ] **Step 1: Write the failing test**

```go
package store

import "testing"

func TestSettingsDefaults(t *testing.T) {
	db := mustOpenMem(t)
	// Defaults: signup off, guest access off.
	if SignupEnabled(db) {
		t.Fatal("signup should default off")
	}
	if GuestAccessEnabled(db) {
		t.Fatal("guest access should default off")
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	db := mustOpenMem(t)
	if err := SetSignupEnabled(db, true); err != nil {
		t.Fatal(err)
	}
	if err := SetGuestAccessEnabled(db, true); err != nil {
		t.Fatal(err)
	}
	if !SignupEnabled(db) || !GuestAccessEnabled(db) {
		t.Fatal("settings not persisted")
	}
}

func TestSigningSecretStable(t *testing.T) {
	db := mustOpenMem(t)
	a, err := MediaSigningSecret(db)
	if err != nil || len(a) < 32 {
		t.Fatalf("secret: len=%d err=%v", len(a), err)
	}
	b, _ := MediaSigningSecret(db)
	if string(a) != string(b) {
		t.Fatal("secret changed across calls — not persisted")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestSetting -v`
Expected: FAIL — `undefined: SignupEnabled`.

- [ ] **Step 3: Write minimal implementation**

```go
package store

import (
	"crypto/rand"
	"database/sql"
)

const (
	keySignupEnabled = "signup_enabled"
	keyGuestAccess   = "guest_access_enabled"
)

// metaFlag reads a boolean meta flag, defaulting to false when the row is
// absent (the secure default for an exposed host).
func metaFlag(db *sql.DB, key string) bool {
	var v int
	db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	return v != 0
}

func setMetaFlag(db *sql.DB, key string, on bool) error {
	_, err := db.Exec(
		`INSERT INTO meta(key, value) VALUES(?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, boolToInt(on))
	return err
}

// SignupEnabled reports whether open signup is allowed (default off).
func SignupEnabled(db *sql.DB) bool { return metaFlag(db, keySignupEnabled) }

// SetSignupEnabled flips the open-signup toggle.
func SetSignupEnabled(db *sql.DB, on bool) error { return setMetaFlag(db, keySignupEnabled, on) }

// GuestAccessEnabled reports whether anonymous visitors may use non-admin
// routes (default off — everything behind login).
func GuestAccessEnabled(db *sql.DB) bool { return metaFlag(db, keyGuestAccess) }

// SetGuestAccessEnabled flips the guest-access toggle.
func SetGuestAccessEnabled(db *sql.DB, on bool) error { return setMetaFlag(db, keyGuestAccess, on) }

// MediaSigningSecret returns the persistent HMAC secret for signed media URLs,
// generating and storing a random one on first use so casts survive restarts.
func MediaSigningSecret(db *sql.DB) ([]byte, error) {
	if _, err := db.Exec(
		`CREATE TABLE IF NOT EXISTS app_secret (id INTEGER PRIMARY KEY CHECK (id = 1), secret BLOB NOT NULL)`,
	); err != nil {
		return nil, err
	}
	var secret []byte
	err := db.QueryRow(`SELECT secret FROM app_secret WHERE id = 1`).Scan(&secret)
	if err == nil {
		return secret, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	secret = make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	if _, err := db.Exec(`INSERT INTO app_secret(id, secret) VALUES(1, ?)`, secret); err != nil {
		return nil, err
	}
	return secret, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run "TestSetting|TestSigning" -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/settings.go internal/store/settings_test.go
git commit -m "feat(store): signup/guest toggles + media signing secret"
```

---

## Phase C — HTTP auth core (`internal/api`)

### Task 9: Rework Server fields for store-backed auth

This replaces the in-memory admin-password model. It touches `server.go`, `admin.go`, and `config.go` together because they share the same fields; do them as one compile-clean change, then the handler tests in later tasks exercise behavior.

**Files:**
- Modify: `internal/api/server.go:43-48` (field block), `:69-88` (constructor), `:120-122` (`SetAdminPassword`)
- Modify: `internal/api/admin.go` (gate functions)
- Modify: `internal/api/config.go`

- [ ] **Step 1: Replace the admin fields in `server.go`**

Remove the `adminMu` / `adminPassword` / `adminTokens` block (lines 43-48) and add:

```go
	// auth holds everything needed to authenticate requests against the store:
	// the DB (users/sessions/invites/settings) and the media-URL signing secret.
	// db is already a field; signingSecret is loaded once at construction by main.
	signingSecret []byte
	// loginAttempts throttles the password form per client IP (soft brute-force
	// guard); guarded by loginMu.
	loginMu       sync.Mutex
	loginAttempts map[string][]int64 // ip -> recent attempt unix-millis
```

- [ ] **Step 2: Update the constructor** (`NewServer`, lines 69-88)

Remove `adminTokens: make(map[string]bool),` and add `loginAttempts: make(map[string][]int64),`.

- [ ] **Step 3: Replace `SetAdminPassword` (lines 120-122)** with:

```go
// SetSigningSecret records the HMAC secret used to sign Sonos media URLs.
// Loaded once at startup from the store (store.MediaSigningSecret).
func (s *Server) SetSigningSecret(secret []byte) { s.signingSecret = secret }
```

- [ ] **Step 4: Rewrite the gate in `admin.go`.** Replace `adminOpen`, `validToken`, `isAdmin`, and `adminLogin`/`adminLogout` with session-based equivalents (these are fully replaced in Task 10/11). For now, to keep the package compiling, replace the body of `admin.go` above `requireAdmin` with:

```go
const sharedStreamID = "house"

// currentUser resolves the request's session cookie to a user. ok is false for
// anonymous requests (no/invalid/expired cookie).
func (s *Server) currentUser(r *http.Request) (store.User, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return store.User{}, false
	}
	u, ok, err := store.UserBySession(s.db, auth.HashToken(c.Value))
	if err != nil || !ok {
		return store.User{}, false
	}
	return u, true
}

// isAdmin reports whether the request carries a valid admin session.
func (s *Server) isAdmin(r *http.Request) bool {
	u, ok := s.currentUser(r)
	return ok && u.IsAdmin
}
```

Keep `requireAdmin`/`requireAdminShared` but change their failure path: 401 when not authenticated at all, 403 when authenticated-but-not-admin. `bearerToken`/`randomToken` are no longer used here — delete `bearerToken`; move `randomToken` out (it's superseded by `auth.GenerateToken`). Add imports for `store` and `auth`. Define `const sessionCookie = "exit66_session"` here or in `auth.go` (Task 10 defines it; if doing admin.go first, declare it here and remove from auth.go).

- [ ] **Step 5: Update `config.go`** `getConfig` to the new shape:

```go
func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	peers := []string{}
	if s.fedPeers != nil {
		peers = s.fedPeers()
	}
	u, authed := s.currentUser(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"mute_local_on_cast": s.muteLocalOnCast,
		"fed_peers":          peers,
		"authenticated":      authed,
		"is_admin":           authed && u.IsAdmin,
		"guest_access":       store.GuestAccessEnabled(s.db),
		"signup_enabled":     store.SignupEnabled(s.db),
		"needs_bootstrap":    countUsersZero(s.db),
	})
}

// countUsersZero reports whether no accounts exist yet (first signup bootstraps
// the admin). Errors are treated as "not zero" so we don't accidentally reopen
// bootstrap on a transient DB error.
func countUsersZero(db *sql.DB) bool {
	n, err := store.CountUsers(db)
	return err == nil && n == 0
}
```

Add the necessary imports (`database/sql`, the `store` package).

- [ ] **Step 6: Verify the package compiles**

Run: `go build ./internal/api/`
Expected: builds (handlers `adminLogin`/`adminLogout` routes in `server.go` will be removed in Task 11; if the build fails only on those two missing funcs, that's expected — proceed to Task 11 before re-running, or temporarily comment those two route lines). Prefer doing Tasks 9–11 back-to-back.

- [ ] **Step 7: Commit**

```bash
git add internal/api/server.go internal/api/admin.go internal/api/config.go
git commit -m "refactor(api): session-based admin gate (replaces shared password)"
```

---

### Task 10: Auth middleware + cookie helpers

**Files:**
- Create: `internal/api/auth.go`
- Test: `internal/api/auth_test.go`

- [ ] **Step 1: Write the failing test**

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/auth"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

func newTestServer(t *testing.T) (*Server, *sql.DB) {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := NewServer(db, nil, nil)
	s.SetSigningSecret([]byte("test-secret"))
	return s, db
}

func TestMiddlewareBlocksAnonymous(t *testing.T) {
	s, _ := newTestServer(t)
	h := s.requireAuth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest("GET", "/api/tracks", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestMiddlewareAllowsSession(t *testing.T) {
	s, db := newTestServer(t)
	uid, _ := store.CreateUser(db, "a@b.com", "A", "h", false)
	raw, _ := auth.GenerateToken()
	store.CreateSession(db, auth.HashToken(raw), uid, 4_000_000_000)
	h := s.requireAuth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/tracks", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: raw})
	h(rec, req)
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d", rec.Code)
	}
}

func TestMiddlewareGuestToggle(t *testing.T) {
	s, db := newTestServer(t)
	store.SetGuestAccessEnabled(db, true)
	h := s.requireAuth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest("GET", "/api/tracks", nil))
	if rec.Code != 200 {
		t.Fatalf("guest access on: want 200, got %d", rec.Code)
	}
}
```

Add `import "database/sql"` to the test file.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestMiddleware -v`
Expected: FAIL — `undefined: requireAuth`.

- [ ] **Step 3: Write minimal implementation**

```go
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

// isHTTPS reports whether the original request was HTTPS, honoring a
// reverse proxy's X-Forwarded-Proto.
func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// clientIP extracts a throttling key from the request (proxy-aware).
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/ -run TestMiddleware -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/auth.go internal/api/auth_test.go
git commit -m "feat(api): session cookie helpers + requireAuth middleware"
```

---

### Task 11: Login / signup / logout / me / invite-accept handlers

**Files:**
- Modify: `internal/api/auth.go` (add handlers)
- Modify: `internal/api/auth_test.go` (add tests)
- Modify: `internal/api/server.go` Handler() routes; delete old `adminLogin`/`adminLogout` from `admin.go`

- [ ] **Step 1: Write the failing tests**

```go
func TestSignupBootstrapsAdmin(t *testing.T) {
	s, db := newTestServer(t)
	// signup disabled, but empty user table → first signup allowed + admin
	rec := httptest.NewRecorder()
	body := `{"email":"a@b.com","display_name":"A","password":"pw123456"}`
	s.signup(rec, httptest.NewRequest("POST", "/api/auth/signup", strings.NewReader(body)))
	if rec.Code != 200 {
		t.Fatalf("bootstrap signup: want 200, got %d (%s)", rec.Code, rec.Body)
	}
	u, _, _ := store.GetUserByEmail(db, "a@b.com")
	if !u.IsAdmin {
		t.Fatal("first user not admin")
	}
	// second signup blocked while signup disabled
	rec2 := httptest.NewRecorder()
	body2 := `{"email":"c@d.com","display_name":"C","password":"pw123456"}`
	s.signup(rec2, httptest.NewRequest("POST", "/api/auth/signup", strings.NewReader(body2)))
	if rec2.Code == 200 {
		t.Fatal("second signup allowed while disabled")
	}
}

func TestLoginSetsCookieAndMe(t *testing.T) {
	s, db := newTestServer(t)
	h, _ := auth.HashPassword("pw123456")
	store.CreateUser(db, "a@b.com", "A", h, true)
	rec := httptest.NewRecorder()
	s.login(rec, httptest.NewRequest("POST", "/api/auth/login",
		strings.NewReader(`{"email":"a@b.com","password":"pw123456"}`)))
	if rec.Code != 200 {
		t.Fatalf("login: want 200, got %d", rec.Code)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != sessionCookie {
		t.Fatal("no session cookie set")
	}
	// /me with the cookie returns the user
	meReq := httptest.NewRequest("GET", "/api/auth/me", nil)
	meReq.AddCookie(cookies[0])
	meRec := httptest.NewRecorder()
	s.me(meRec, meReq)
	if meRec.Code != 200 || !strings.Contains(meRec.Body.String(), "a@b.com") {
		t.Fatalf("me: %d %s", meRec.Code, meRec.Body)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	s, db := newTestServer(t)
	h, _ := auth.HashPassword("right")
	store.CreateUser(db, "a@b.com", "A", h, false)
	rec := httptest.NewRecorder()
	s.login(rec, httptest.NewRequest("POST", "/api/auth/login",
		strings.NewReader(`{"email":"a@b.com","password":"wrong"}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestInviteAcceptCreatesUser(t *testing.T) {
	s, db := newTestServer(t)
	admin, _ := store.CreateUser(db, "admin@b.com", "Ad", "h", true)
	raw, _ := auth.GenerateToken()
	store.CreateInvite(db, auth.HashToken(raw), "inv@b.com", true, admin, 4_000_000_000)
	rec := httptest.NewRecorder()
	body := `{"token":"` + raw + `","display_name":"Inv","password":"pw123456"}`
	s.inviteAccept(rec, httptest.NewRequest("POST", "/api/auth/invite/accept", strings.NewReader(body)))
	if rec.Code != 200 {
		t.Fatalf("accept: want 200, got %d (%s)", rec.Code, rec.Body)
	}
	u, ok, _ := store.GetUserByEmail(db, "inv@b.com")
	if !ok || !u.IsAdmin {
		t.Fatalf("invited admin not created: ok=%v admin=%v", ok, u.IsAdmin)
	}
	// single-use: second accept fails
	rec2 := httptest.NewRecorder()
	s.inviteAccept(rec2, httptest.NewRequest("POST", "/api/auth/invite/accept", strings.NewReader(body)))
	if rec2.Code == 200 {
		t.Fatal("invite reused")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run "TestSignup|TestLogin|TestInvite" -v`
Expected: FAIL — `undefined: (*Server).signup`.

- [ ] **Step 3: Add the handlers to `auth.go`**

```go
type signupReq struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

const minPasswordLen = 8

// signup creates an account. Rules: an empty user table always allows the
// signup and makes that first account an admin (bootstrap); otherwise signup is
// allowed only when the signup toggle is on, and the account is non-admin.
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

// login validates credentials and issues a session cookie. Throttled per IP.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.allowLogin(ip) {
		writeErr(w, http.StatusTooManyRequests, "too many attempts; wait a minute")
		return
	}
	var req loginReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
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
// (admin if the invite granted it), marks the invite used, and logs in.
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

// allowLogin records an attempt for ip and reports whether it's under the limit
// (10 attempts / 60s sliding window).
func (s *Server) allowLogin(ip string) bool {
	const window = 60 * 1000
	const maxAttempts = 10
	now := time.Now().UnixMilli()
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	cutoff := now - window
	kept := s.loginAttempts[ip][:0]
	for _, t := range s.loginAttempts[ip] {
		if t > cutoff {
			kept = append(kept, t)
		}
	}
	kept = append(kept, now)
	s.loginAttempts[ip] = kept
	return len(kept) <= maxAttempts
}
```

Note: the invite flow uses the invite's stored email (set by the admin) rather than trusting client input, so an invite link can't be redirected to a different address.

- [ ] **Step 4: Wire routes + remove the old gate.** In `server.go` `Handler()`, delete the two lines:

```go
mux.HandleFunc("POST /api/admin/login", s.adminLogin)
mux.HandleFunc("POST /api/admin/logout", s.adminLogout)
```

and add:

```go
	mux.HandleFunc("POST /api/auth/signup", s.signup)
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.HandleFunc("GET /api/auth/me", s.me)
	mux.HandleFunc("POST /api/auth/invite/accept", s.inviteAccept)
```

Delete `adminLogin`, `adminLogout`, and `randomToken` from `admin.go` (superseded). Whole-handler auth wrapping happens in Task 16 (`main.go`), not per-route here — `requireAdmin`/`requireAdminShared` stay on the privileged routes as today.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/api/ -run "TestSignup|TestLogin|TestInvite|TestMiddleware" -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/api/auth.go internal/api/auth_test.go internal/api/server.go internal/api/admin.go
git commit -m "feat(api): login/signup/logout/me/invite-accept handlers"
```

---

### Task 12: Signed-URL carve-out on the audio handler

**Files:**
- Modify: `internal/api/audio.go`
- Test: `internal/api/audio_signed_test.go` (create)

First read `internal/api/audio.go` to see the `trackAudio` signature and how it currently serves bytes.

- [ ] **Step 1: Write the failing test**

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/auth"
)

func TestAudioAcceptsSignedToken(t *testing.T) {
	s, _ := newTestServer(t) // signing secret "test-secret"
	// A valid signed token for track 7 should pass the auth check even with no
	// session cookie. (Track 7 won't exist in the empty DB, so we assert the
	// handler got *past* auth — i.e. not 401/403 — by expecting 404.)
	tok := auth.SignMedia([]byte("test-secret"), 7, 4_000_000_000)
	req := httptest.NewRequest("GET", "/api/tracks/7/audio?sig="+tok, nil)
	req.SetPathValue("id", "7")
	rec := httptest.NewRecorder()
	s.trackAudioGuarded(rec, req)
	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
		t.Fatalf("signed token rejected: %d", rec.Code)
	}
}

func TestAudioRejectsForgedToken(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/tracks/7/audio?sig=7.4000000000.bogus", nil)
	req.SetPathValue("id", "7")
	rec := httptest.NewRecorder()
	s.trackAudioGuarded(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("forged token: want 401, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestAudio -v`
Expected: FAIL — `undefined: (*Server).trackAudioGuarded`.

- [ ] **Step 3: Add the guarded wrapper in `audio.go`**

```go
// trackAudioGuarded authorizes an audio request by EITHER a valid session
// (browser, via requireAuth) OR a valid signed media token (Sonos cast, which
// fetches with no cookie), then serves the track. The signed-token path checks
// the token's track id matches the path id so a token can't be reused for
// another track.
func (s *Server) trackAudioGuarded(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.currentUser(r); ok {
		s.trackAudio(w, r)
		return
	}
	if store.GuestAccessEnabled(s.db) {
		s.trackAudio(w, r)
		return
	}
	if sig := r.URL.Query().Get("sig"); sig != "" {
		if id, ok := auth.VerifyMedia(s.signingSecret, sig, time.Now().Unix()); ok {
			if strconv.FormatInt(id, 10) == r.PathValue("id") {
				s.trackAudio(w, r)
				return
			}
		}
	}
	writeErr(w, http.StatusUnauthorized, "login required")
}
```

Add imports as needed (`time`, `strconv`, `store`, `auth`). In `server.go` `Handler()`, change the audio route from `s.trackAudio` to `s.trackAudioGuarded`:

```go
mux.HandleFunc("GET /api/tracks/{id}/audio", s.trackAudioGuarded)
```

(Cover routes follow the same pattern in Task 13.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/ -run TestAudio -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/audio.go internal/api/audio_signed_test.go internal/api/server.go
git commit -m "feat(api): signed-URL carve-out for audio (Sonos casts)"
```

---

### Task 13: Sonos cast builds signed audio URLs + cover carve-out

**Files:**
- Modify: `internal/api/sonos.go`
- Modify: `internal/api/cover.go` (apply the same guard to track/album cover handlers)
- Modify: `internal/api/server.go` (route the cover handlers through guarded wrappers)

First read `internal/api/sonos.go` to find where the cast hands the speaker a track/stream URL (look for how the audio URL is constructed from `s.listenAddr`).

- [ ] **Step 1: Add a cover guard mirroring Task 12.** In `cover.go` add `trackCoverGuarded` and `albumCoverGuarded` wrappers that call `currentUser` / guest toggle / signed-`sig` exactly like `trackAudioGuarded` (album covers verify the token's id against the album path id). Repoint the cover routes in `server.go`:

```go
mux.HandleFunc("GET /api/tracks/{id}/cover", s.trackCoverGuarded)
mux.HandleFunc("GET /api/albums/{id}/cover", s.albumCoverGuarded)
```

- [ ] **Step 2: In `sonos.go`, sign the URLs the cast hands the speaker.** Where the cast currently builds the track audio URL (and cover URL, if it sends one in DIDL metadata), append a signed token valid for the cast's lifetime:

```go
// signedMediaURL builds an absolute, signed URL the speaker can fetch without a
// cookie. ttl should comfortably exceed a track's length.
func (s *Server) signedMediaURL(path string, trackID int64) string {
	exp := time.Now().Add(6 * time.Hour).Unix()
	sig := auth.SignMedia(s.signingSecret, trackID, exp)
	return "http://" + s.castHost() + path + "?sig=" + sig
}
```

Use it for the track audio URL passed to the Sonos device. (`castHost()` is the existing helper/derivation that builds the server-reachable host:port from `s.listenAddr` + detected IP — reuse whatever `sonos.go` already does; only the `?sig=` suffix is new.)

- [ ] **Step 3: Manual smoke (no unit test — needs a real device).** Document in the commit body that cast was verified against a real Sonos, or defer to the end-to-end check in Task 18. Add a regression test only if `sonos.go` already has injectable URL-building seams; otherwise rely on Task 12's signed-URL tests plus the manual cast check.

- [ ] **Step 4: Verify build + existing sonos tests**

Run: `go test ./internal/api/ -run "Sonos|Audio|Cover" -v`
Expected: PASS (existing Sonos tests still green; new cover guard tests if added).

- [ ] **Step 5: Commit**

```bash
git add internal/api/sonos.go internal/api/cover.go internal/api/server.go
git commit -m "feat(api): casts use signed media URLs; cover carve-out"
```

---

### Task 14: Admin endpoints — settings, invites, users

**Files:**
- Create: `internal/api/adminusers.go`
- Test: `internal/api/adminusers_test.go`
- Modify: `internal/api/server.go` (routes)

- [ ] **Step 1: Write the failing tests**

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/auth"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

// adminReq builds a request carrying a valid admin session cookie.
func adminReq(t *testing.T, s *Server, db *sql.DB, method, path, body string) *http.Request {
	t.Helper()
	uid, _ := store.CreateUser(db, "admin@b.com", "Ad", "h", true)
	raw, _ := auth.GenerateToken()
	store.CreateSession(db, auth.HashToken(raw), uid, 4_000_000_000)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: raw})
	return req
}

func TestAdminSettingsToggle(t *testing.T) {
	s, db := newTestServer(t)
	req := adminReq(t, s, db, "POST", "/api/admin/settings", `{"signup_enabled":true,"guest_access_enabled":true}`)
	rec := httptest.NewRecorder()
	s.requireAdmin(s.setAdminSettings)(rec, req)
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body)
	}
	if !store.SignupEnabled(db) || !store.GuestAccessEnabled(db) {
		t.Fatal("toggles not applied")
	}
}

func TestAdminCreateInviteReturnsLink(t *testing.T) {
	s, db := newTestServer(t)
	req := adminReq(t, s, db, "POST", "/api/admin/invites", `{"email":"x@y.com","is_admin":false}`)
	rec := httptest.NewRecorder()
	s.requireAdmin(s.createInvite)(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "/invite/") {
		t.Fatalf("invite link missing: %d %s", rec.Code, rec.Body)
	}
	if invs, _ := store.ListInvites(db); len(invs) != 1 {
		t.Fatalf("invite not stored")
	}
}

func TestAdminNonAdminForbidden(t *testing.T) {
	s, db := newTestServer(t)
	uid, _ := store.CreateUser(db, "u@b.com", "U", "h", false) // not admin
	raw, _ := auth.GenerateToken()
	store.CreateSession(db, auth.HashToken(raw), uid, 4_000_000_000)
	req := httptest.NewRequest("GET", "/api/admin/users", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: raw})
	rec := httptest.NewRecorder()
	s.requireAdmin(s.listUsers)(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rec.Code)
	}
}
```

Add `import "database/sql"`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestAdmin -v`
Expected: FAIL — `undefined: (*Server).setAdminSettings`.

- [ ] **Step 3: Write the handlers**

```go
package api

import (
	"net/http"
	"strconv"
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
```

Add the `emailInvite` field + setter to `server.go`:

```go
	// emailInvite, when non-nil (SMTP configured), sends an invite link to an
	// address. Best-effort: called in a goroutine, errors logged not surfaced.
	emailInvite func(to, link string)
```

```go
// SetInviteEmailer attaches the optional SMTP invite sender.
func (s *Server) SetInviteEmailer(fn func(to, link string)) { s.emailInvite = fn }
```

- [ ] **Step 4: Wire routes in `server.go` `Handler()`**

```go
	mux.HandleFunc("GET /api/admin/settings", s.requireAdmin(s.getAdminSettings))
	mux.HandleFunc("POST /api/admin/settings", s.requireAdmin(s.setAdminSettings))
	mux.HandleFunc("POST /api/admin/invites", s.requireAdmin(s.createInvite))
	mux.HandleFunc("GET /api/admin/invites", s.requireAdmin(s.listInvites))
	mux.HandleFunc("DELETE /api/admin/invites/{id}", s.requireAdmin(s.deleteInvite))
	mux.HandleFunc("GET /api/admin/users", s.requireAdmin(s.listUsers))
	mux.HandleFunc("DELETE /api/admin/users/{id}", s.requireAdmin(s.deleteUser))
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/api/ -run TestAdmin -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/api/adminusers.go internal/api/adminusers_test.go internal/api/server.go
git commit -m "feat(api): admin settings/invites/users endpoints"
```

---

## Phase D — Optional SMTP (`internal/email`)

### Task 15: SMTP invite sender

**Files:**
- Create: `internal/email/email.go`
- Test: `internal/email/email_test.go`
- Modify: `internal/config/config.go` (SMTP env)

- [ ] **Step 1: Write the failing test** (test message construction, not real sending)

```go
package email

import (
	"strings"
	"testing"
)

func TestInviteMessage(t *testing.T) {
	msg := inviteMessage("from@host", "to@host", "https://hub/invite/abc")
	s := string(msg)
	if !strings.Contains(s, "To: to@host") || !strings.Contains(s, "From: from@host") {
		t.Fatalf("headers missing: %s", s)
	}
	if !strings.Contains(s, "https://hub/invite/abc") {
		t.Fatal("link missing from body")
	}
	if !strings.Contains(s, "Subject:") {
		t.Fatal("subject missing")
	}
}

func TestSenderDisabledWhenUnconfigured(t *testing.T) {
	s := New(Config{}) // no host
	if s.Enabled() {
		t.Fatal("sender should be disabled with no host")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/email/ -v`
Expected: FAIL — `undefined: inviteMessage`.

- [ ] **Step 3: Write minimal implementation**

```go
// Package email sends invite emails over SMTP. It is entirely optional: with no
// SMTP host configured the Sender is disabled and the app surfaces invite links
// for manual sharing instead.
package email

import (
	"fmt"
	"net/smtp"
)

// Config holds SMTP settings, read from the environment by main.
type Config struct {
	Host string // e.g. "smtp.example.com"
	Port string // e.g. "587"
	User string
	Pass string
	From string // envelope + header From
}

// Sender sends invite emails. The zero/unconfigured value is disabled.
type Sender struct {
	cfg  Config
	send func(addr string, a smtp.Auth, from string, to []string, msg []byte) error // injectable for tests
}

// New builds a Sender from cfg. When Host is empty the Sender is disabled.
func New(cfg Config) *Sender {
	return &Sender{cfg: cfg, send: smtp.SendMail}
}

// Enabled reports whether SMTP is configured.
func (s *Sender) Enabled() bool { return s.cfg.Host != "" }

// SendInvite emails the invite link to addr. No-op error when disabled.
func (s *Sender) SendInvite(to, link string) error {
	if !s.Enabled() {
		return nil
	}
	var auth smtp.Auth
	if s.cfg.User != "" {
		auth = smtp.PlainAuth("", s.cfg.User, s.cfg.Pass, s.cfg.Host)
	}
	addr := s.cfg.Host + ":" + s.cfg.Port
	return s.send(addr, auth, s.cfg.From, []string{to}, inviteMessage(s.cfg.From, to, link))
}

// inviteMessage builds an RFC 5322 message.
func inviteMessage(from, to, link string) []byte {
	return []byte(fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: You're invited to Exit 66 Jukebox\r\n\r\n"+
			"You've been invited. Open this link to set up your account:\r\n\r\n%s\r\n",
		from, to, link))
}
```

- [ ] **Step 4: Add SMTP config to `config.go`.** Add an `SMTP` struct + `smtpFromEnv()` reading `EXIT66_SMTP_HOST/_PORT/_USER/_PASS/_FROM`, defaulting port to `587`, and add `SMTP SMTP` to `Config` populated in `Parse`. Follow the `servicesFromEnv()` pattern (env only, never flags).

```go
// SMTP holds optional invite-email settings (env only). Host empty = disabled.
type SMTP struct {
	Host, Port, User, Pass, From string
}

func smtpFromEnv() SMTP {
	port := os.Getenv("EXIT66_SMTP_PORT")
	if port == "" {
		port = "587"
	}
	return SMTP{
		Host: os.Getenv("EXIT66_SMTP_HOST"),
		Port: port,
		User: os.Getenv("EXIT66_SMTP_USER"),
		Pass: os.Getenv("EXIT66_SMTP_PASS"),
		From: os.Getenv("EXIT66_SMTP_FROM"),
	}
}
```

Add `SMTP SMTP` to the `Config` struct and `c.SMTP = smtpFromEnv()` in `Parse`. Also **remove** `AdminPassword` from `Config`, the `-admin-password` flag, and its env fallback (superseded by accounts). Update `config_test.go` accordingly (drop any admin-password assertions).

- [ ] **Step 5: Run tests**

Run: `go test ./internal/email/ ./internal/config/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/email/ internal/config/config.go internal/config/config_test.go
git commit -m "feat(email): optional SMTP invite sender; drop admin-password config"
```

---

## Phase E — Wiring (`main.go`)

### Task 16: Wire store-backed auth and wrap only the public handler

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Load the signing secret + settings; purge expired sessions.** After `db` is opened (around line 51), add:

```go
	if err := store.PurgeExpiredSessions(db); err != nil {
		log.Printf("purge sessions: %v", err)
	}
	signingSecret, err := store.MediaSigningSecret(db)
	if err != nil {
		log.Fatalf("signing secret: %v", err)
	}
```

- [ ] **Step 2: Replace admin-password wiring.** Delete `srv.SetAdminPassword(cfg.AdminPassword)` (line 220) and add:

```go
	srv.SetSigningSecret(signingSecret)
	mailer := email.New(email.Config{
		Host: cfg.SMTP.Host, Port: cfg.SMTP.Port, User: cfg.SMTP.User,
		Pass: cfg.SMTP.Pass, From: cfg.SMTP.From,
	})
	if mailer.Enabled() {
		srv.SetInviteEmailer(func(to, link string) {
			if err := mailer.SendInvite(to, link); err != nil {
				log.Printf("invite email to %s: %v", to, err)
			}
		})
		log.Print("SMTP invite email enabled")
	}
```

Add `"github.com/andybarilla/exit66jukebox/internal/email"` to imports.

- [ ] **Step 3: Wrap ONLY the public handler.** The federation `MemberHandler` is set to `srv.Handler()` at line 261 and must stay raw. The public server at line 281 currently uses `srv.Handler()` too. Build the public handler once and wrap it:

```go
	publicHandler := srv.RequireAuthMiddleware(srv.Handler())
	httpServer := &http.Server{Addr: cfg.Addr, Handler: publicHandler}
```

- [ ] **Step 4: Add the top-level middleware to `api`.** In `auth.go`, add a handler-wrapping middleware that enforces auth on `/api/*` (except the open auth endpoints and static assets), leaving static files and signed-media requests to their own per-route guards. Because `requireAuth`/`requireAdmin` already wrap individual privileged routes, this top-level layer covers the remaining read routes (browse/list/config/etc.) when guest access is off:

```go
// RequireAuthMiddleware enforces a session on browse/read API routes when guest
// access is off. Open paths (auth endpoints, the SPA static shell, and
// signed-media routes which self-authorize) pass through; everything else under
// /api/ requires a session or the guest toggle.
func (s *Server) RequireAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") || isOpenPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if _, ok := s.currentUser(r); ok {
			next.ServeHTTP(w, r)
			return
		}
		if store.GuestAccessEnabled(s.db) {
			next.ServeHTTP(w, r)
			return
		}
		// Signed-media routes carry their own ?sig= authorization; let them reach
		// the route's own guard (Task 12/13) to validate.
		if r.URL.Query().Get("sig") != "" {
			next.ServeHTTP(w, r)
			return
		}
		writeErr(w, http.StatusUnauthorized, "login required")
	})
}

// isOpenPath lists API routes reachable without a session.
func isOpenPath(p string) bool {
	switch p {
	case "/api/auth/login", "/api/auth/signup", "/api/auth/logout",
		"/api/auth/me", "/api/auth/invite/accept", "/api/config":
		return true
	}
	return false
}
```

`/api/config` stays open so the unauthenticated SPA can learn whether to show login vs signup vs bootstrap. Static assets (everything not under `/api/`) are always served so the login page can load.

- [ ] **Step 5: Build + run the whole backend suite**

Run: `go build ./... && go test ./...`
Expected: builds; all tests PASS. Fix any remaining references to removed symbols (`AdminPassword`, `adminLogin`, etc.).

- [ ] **Step 6: Commit**

```bash
git add main.go internal/api/auth.go
git commit -m "feat: wire store-backed auth; wrap only the public listener"
```

---

## Phase F — Frontend (Svelte)

> The UI is built by vite into `internal/web/dist` and embedded. After UI changes, **rebuild and commit the dist** (see Task 20) — `git add web/` alone ships a stale UI.

### Task 17: Frontend auth API module

**Files:**
- Create: `web/src/lib/auth.js`
- Test: `web/src/lib/auth.test.js`
- Modify: `web/src/lib/api.js` (remove bearer-token machinery)

- [ ] **Step 1: Write the failing test** (vitest, mocking `fetch`)

```js
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { login, signup, logout, fetchMe, acceptInvite } from './auth.js';

beforeEach(() => { global.fetch = vi.fn(); });

function jsonResp(body, ok = true, status = 200) {
  return Promise.resolve({ ok, status, json: () => Promise.resolve(body) });
}

describe('auth api', () => {
  it('login posts credentials and returns user', async () => {
    fetch.mockReturnValue(jsonResp({ id: 1, email: 'a@b.com', is_admin: true }));
    const u = await login('a@b.com', 'pw');
    expect(u.email).toBe('a@b.com');
    const [url, opts] = fetch.mock.calls[0];
    expect(url).toBe('/api/auth/login');
    expect(opts.method).toBe('POST');
    expect(JSON.parse(opts.body)).toEqual({ email: 'a@b.com', password: 'pw' });
  });

  it('login throws on bad credentials', async () => {
    fetch.mockReturnValue(jsonResp({ error: 'nope' }, false, 401));
    await expect(login('a@b.com', 'x')).rejects.toThrow();
  });

  it('fetchMe returns null when unauthenticated', async () => {
    fetch.mockReturnValue(jsonResp({ error: 'no' }, false, 401));
    expect(await fetchMe()).toBeNull();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/lib/auth.test.js`
Expected: FAIL — cannot resolve `./auth.js`.

- [ ] **Step 3: Write `auth.js`**

```js
// Auth API calls. Sessions are cookie-based (httpOnly, set by the server), so
// there are no tokens to manage client-side — fetch sends the cookie
// automatically for same-origin requests.

async function postJSON(url, body) {
  const r = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!r.ok) {
    const e = await r.json().catch(() => ({}));
    throw new Error(e.error || 'request failed');
  }
  return r.json();
}

export const login = (email, password) => postJSON('/api/auth/login', { email, password });
export const signup = (email, display_name, password) =>
  postJSON('/api/auth/signup', { email, display_name, password });
export const acceptInvite = (token, display_name, password) =>
  postJSON('/api/auth/invite/accept', { token, display_name, password });

export async function logout() {
  await fetch('/api/auth/logout', { method: 'POST' });
}

// fetchMe returns the current user, or null when not logged in.
export async function fetchMe() {
  const r = await fetch('/api/auth/me');
  if (!r.ok) return null;
  return r.json();
}
```

- [ ] **Step 4: Strip bearer-token machinery from `api.js`.** Remove `ADMIN_TOKEN_KEY`, `adminToken`, `authHeaders`, `clearAdminToken`, `adminLogin`, `adminLogout`, and every `headers: authHeaders()` (cookies authenticate now). Update `api.test.js` if it referenced those. Endpoints keep their paths; just drop the `headers: authHeaders()` option object where it was the only option, or replace with `{ method: 'POST', body }`.

- [ ] **Step 5: Run tests**

Run: `cd web && npx vitest run src/lib/auth.test.js src/lib/api.test.js`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/lib/auth.js web/src/lib/auth.test.js web/src/lib/api.js web/src/lib/api.test.js
git commit -m "feat(web): cookie-based auth api; drop bearer tokens"
```

---

### Task 18: Login / Signup / InviteAccept components

**Files:**
- Create: `web/src/lib/components/Login.svelte`, `Signup.svelte`, `InviteAccept.svelte`

These are presentational forms calling the Task 17 helpers. Match existing component style (see `CastPanel.svelte`, `SearchInput.svelte` for the project's Svelte 5 conventions: `$state`, `$props`, scoped `<style>`, the `tokens.css` design vars). Use a dark-friendly palette consistent with the app.

- [ ] **Step 1: `Login.svelte`** — email + password inputs, submit button, error line. Props: `onLoggedIn(user)`, `canSignup` (bool), `onSwitchToSignup()`.

```svelte
<script>
  import { login } from '../auth.js';
  let { onLoggedIn, canSignup = false, onSwitchToSignup } = $props();
  let email = $state('');
  let password = $state('');
  let error = $state('');
  let busy = $state(false);

  async function submit(e) {
    e.preventDefault();
    busy = true; error = '';
    try {
      onLoggedIn(await login(email, password));
    } catch (err) {
      error = err.message || 'login failed';
    } finally {
      busy = false;
    }
  }
</script>

<form class="auth" onsubmit={submit}>
  <h1>Exit 66 Jukebox</h1>
  <input type="email" placeholder="Email" bind:value={email} autocomplete="username" required />
  <input type="password" placeholder="Password" bind:value={password} autocomplete="current-password" required />
  {#if error}<p class="err">{error}</p>{/if}
  <button disabled={busy} type="submit">Log in</button>
  {#if canSignup}
    <button type="button" class="link" onclick={onSwitchToSignup}>Create an account</button>
  {/if}
</form>

<style>
  .auth { max-width: 22rem; margin: 12vh auto; display: flex; flex-direction: column; gap: .75rem; }
  input, button { padding: .6rem .75rem; font-size: 1rem; }
  .err { color: var(--danger, #ff6b6b); margin: 0; }
  .link { background: none; border: none; color: var(--accent, #6cf); cursor: pointer; }
</style>
```

- [ ] **Step 2: `Signup.svelte`** — same shape with a display-name field, calling `signup(email, displayName, password)`; props `onLoggedIn(user)`, `onSwitchToLogin()`. Reuse the `.auth` style.

- [ ] **Step 3: `InviteAccept.svelte`** — reads the token from the URL path (`/invite/<token>`), shows display-name + password fields, calls `acceptInvite(token, displayName, password)`, then `onLoggedIn(user)`. Extract the token in the component:

```js
const token = window.location.pathname.replace(/^\/invite\//, '');
```

- [ ] **Step 4: Smoke-render check.** These are simple forms; if the project has no component test harness, verify they compile by running the dev build in Task 20. (No vitest component tests required — the codebase tests logic modules, not `.svelte` files.)

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/components/Login.svelte web/src/lib/components/Signup.svelte web/src/lib/components/InviteAccept.svelte
git commit -m "feat(web): login, signup, and invite-accept forms"
```

---

### Task 19: Auth gate in App + store; AdminPanel

**Files:**
- Modify: `web/src/lib/store.svelte.js` (auth state)
- Modify: `web/src/App.svelte` (gate + routing)
- Create: `web/src/lib/components/AdminPanel.svelte`

- [ ] **Step 1: Add auth state to the store.** In `createStore()` replace the admin-token state (`adminRequired`, `adminTokenPresent`) with:

```js
  let me = $state(null);            // current user {id,email,display_name,is_admin} | null
  let authChecked = $state(false);  // becomes true after the first /me round-trip
```

In `init()`, call `fetchMe()` and set `me`/`authChecked`. Expose `me`, `authChecked`, `isAdmin` (`me?.is_admin === true`), a `setMe(user)` setter (used after login), and a `signOut()` that calls `logout()` then clears `me`. The existing `config` load already returns `authenticated`, `guest_access`, `signup_enabled`, `needs_bootstrap`; store those so App can choose which auth screen to show. Update `store.test.js` for the new fields, removing admin-token assertions.

- [ ] **Step 2: Gate the app in `App.svelte`.** Before rendering the main UI, branch on auth state:

```svelte
{#if !store.authChecked}
  <!-- first paint: nothing or a spinner -->
{:else if window.location.pathname.startsWith('/invite/')}
  <InviteAccept onLoggedIn={(u) => { store.setMe(u); window.history.replaceState(null, '', '/'); }} />
{:else if !store.me && !store.config.guestAccess}
  {#if showSignup}
    <Signup onLoggedIn={(u) => store.setMe(u)} onSwitchToLogin={() => showSignup = false} />
  {:else}
    <Login canSignup={store.config.signupEnabled || store.config.needsBootstrap}
           onSwitchToSignup={() => showSignup = true}
           onLoggedIn={(u) => store.setMe(u)} />
  {/if}
{:else}
  <!-- existing app UI here -->
{/if}
```

Add `let showSignup = $state(false)` and the component imports. When `guestAccess` is on, the app renders for anonymous users (current behavior) and shows a "Log in" affordance in `TopBar`.

- [ ] **Step 3: `AdminPanel.svelte`.** A panel (reachable from `TopBar` when `store.isAdmin`) with: two toggles (signup, guest access) backed by `GET/POST /api/admin/settings`; an invite creator (email + "admin?" checkbox) that calls `POST /api/admin/invites` and shows the returned link with a copy button (reuse `IconLink`/`IconCheck`); a list of invites (`GET /api/admin/invites`) with revoke buttons; and a user list (`GET /api/admin/users`) with delete buttons. Add the fetch calls to `auth.js` or a new `admin.js` module:

```js
export const getSettings = () => fetch('/api/admin/settings').then((r) => r.json());
export const setSettings = (s) => postJSON('/api/admin/settings', s);
export const createInvite = (email, is_admin) => postJSON('/api/admin/invites', { email, is_admin });
export const listInvites = () => fetch('/api/admin/invites').then((r) => r.json());
export const deleteInvite = (id) => fetch(`/api/admin/invites/${id}`, { method: 'DELETE' });
export const listUsers = () => fetch('/api/admin/users').then((r) => r.json());
export const deleteUser = (id) => fetch(`/api/admin/users/${id}`, { method: 'DELETE' });
```

- [ ] **Step 4: Replace the old admin-login affordance.** Wherever `TopBar`/`store` previously triggered `adminLogin`, swap to: show the logged-in user's name + a "Log out" button (calls `store.signOut()`), and a "Settings" button opening `AdminPanel` when `store.isAdmin`.

- [ ] **Step 5: Run the frontend unit tests**

Run: `cd web && npx vitest run`
Expected: PASS (logic modules; `.svelte` components aren't unit-tested here).

- [ ] **Step 6: Commit**

```bash
git add web/src/lib/store.svelte.js web/src/App.svelte web/src/lib/components/AdminPanel.svelte web/src/lib/auth.js
git commit -m "feat(web): auth gate, login/logout in UI, admin panel"
```

---

### Task 20: Build + embed the UI; full verification

**Files:**
- Modify: `internal/web/dist/**` (built output)

- [ ] **Step 1: Build the UI**

Run: `cd web && npm run build`
Expected: vite writes to `../internal/web/dist`.

- [ ] **Step 2: Build + test the whole project**

Run: `go build ./... && go test ./...`
Expected: builds; all PASS.

- [ ] **Step 3: End-to-end smoke on a fresh DB**

```bash
rm -f /tmp/e66auth.db
go run . -db /tmp/e66auth.db -addr :8066 &
```

Verify:
1. `curl -i localhost:8066/api/tracks` → 401 (login required).
2. Open `http://localhost:8066/` → login page. The bootstrap signup link is offered (empty DB).
3. Sign up the first account → becomes admin, lands in the app.
4. In the admin panel: create an invite → a `/invite/<token>` link appears; visiting it in a private window lets a second person set a password and land in the app.
5. Toggle guest access on → an anonymous private window can browse without logging in; admin routes still 403.
6. `curl -i 'localhost:8066/api/tracks/1/audio'` with guest access off and no cookie → 401; with a valid `?sig=` token → not 401.
7. Restart the process → still logged in (session persisted), casts still work.

Stop the server when done (`kill %1`).

- [ ] **Step 4: Commit the built UI**

```bash
git add internal/web/dist
git commit -m "build(web): rebuild embedded UI with auth screens"
```

---

## Self-Review

**Spec coverage:**
- Three credential paths → Tasks 3, 10, 12, 13 (browser cookie / signed URL), federation left raw in Task 16.
- Default-deny + guest toggle → Tasks 8, 10, 16.
- Local email+password, pbkdf2 → Tasks 1, 5, 11.
- First-signup bootstrap admin → Task 11.
- Signup toggle (default off) → Tasks 8, 11, 14.
- Invites (link always, SMTP optional, single-use, expiring, admin-granting) → Tasks 7, 14, 15.
- Sessions persisted in SQLite, hashed tokens → Tasks 2, 6.
- Replace shared-password gate → Tasks 9, 11, 15 (config), 16.
- API surface → Tasks 11, 14.
- Frontend (login/signup/invite/admin, rebuild dist) → Tasks 17–20.
- Login throttle → Task 11.

**Type consistency:** `auth.HashToken`/`GenerateToken`/`SignMedia`/`VerifyMedia`, `store.User`/`Invite`, `sessionCookie`, `currentUser`, `requireAuth`/`RequireAuthMiddleware`, `trackAudioGuarded`, `emailInvite`/`SetInviteEmailer`, `SetSigningSecret` are defined once and referenced consistently.

**Open items the worker must confirm against the live code (not placeholders — verification points):**
- The exact test-DB helper name in `internal/store/store_test.go` (Task 4/5) — reuse it instead of redefining.
- The `trackAudio` / `trackCover` / `albumCover` handler signatures and how `sonos.go` builds the cast URL + host (Tasks 12, 13) — read those files before editing.
- Whether `api.test.js` / `store.test.js` reference removed admin-token symbols (Tasks 17, 19) — update them.
