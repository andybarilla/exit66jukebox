package store

import (
	"crypto/rand"
	"database/sql"
)

const (
	keySignupEnabled    = "signup_enabled"
	keyGuestAccess      = "guest_access_enabled"
	keyAdminMFARequired = "admin_mfa_required"
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

// AdminMFARequired reports whether admin users must complete MFA (default off).
func AdminMFARequired(db *sql.DB) bool { return metaFlag(db, keyAdminMFARequired) }

// SetAdminMFARequired flips the admin MFA requirement toggle.
func SetAdminMFARequired(db *sql.DB, on bool) error { return setMetaFlag(db, keyAdminMFARequired, on) }

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
