package store

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/andybarilla/exit66jukebox/internal/auth"
)

const PasswordlessPasswordHash = "passwordless"

// ErrBootstrapAlreadyClaimed reports that CreateFirstAdmin lost the race: a user
// row already existed, so no admin was inserted.
var ErrBootstrapAlreadyClaimed = errors.New("bootstrap already claimed")

// User is an account row. PasswordHash is the encoded pbkdf2 string from
// internal/auth; this package never hashes or compares passwords itself.
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

// CountUsers returns the number of accounts. Zero means the instance is
// uninitialized: the next signup bootstraps the first admin.
func CountUsers(db *sql.DB) (int, error) {
	var n int
	err := db.QueryRow(`SELECT count(*) FROM user`).Scan(&n)
	return n, err
}

// CreateUser inserts an account and returns its id. The UNIQUE(email)
// constraint surfaces as an error on a duplicate.

func CreateUser(db *sql.DB, email, displayName, passwordHash string, isAdmin bool, verified ...bool) (int64, error) {
	if err := ensurePasswordlessProfileColumn(db); err != nil {
		return 0, err
	}
	emailVerifiedAtExpr := "0"
	if len(verified) == 0 || verified[0] {
		emailVerifiedAtExpr = "strftime('%s','now')"
	}
	res, err := db.Exec(
		`INSERT INTO user(email, display_name, password_hash, is_admin, is_passwordless_profile, created_at, email_verified_at)
		 VALUES(?,?,?,?,0,strftime('%s','now'),`+emailVerifiedAtExpr+`)`,
		email, displayName, passwordHash, boolToInt(isAdmin))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// CreateFirstAdmin inserts the bootstrap admin in a single statement, so
// concurrent first-signup attempts can't both observe an empty table: the
// INSERT ... SELECT ... WHERE NOT EXISTS makes exactly one of them affect a row
// and the losers get ErrBootstrapAlreadyClaimed.
//
// is_passwordless_profile is deliberately not named here, and this is the one
// writer in this file that does not call ensurePasswordlessProfileColumn:
// schema.sql does not define that column and migrate() never adds it, so on a
// fresh database it does not exist until some other writer's lazy ALTER runs.
// Naming it would fail outright, and running the ALTER from here would put N
// concurrent bootstrap attempts into exactly the write-lock contention the
// atomic insert exists to avoid. The column's NOT NULL DEFAULT 0 covers the row
// once the ALTER does land.
func CreateFirstAdmin(db *sql.DB, email, displayName, passwordHash string) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO user(email, display_name, password_hash, is_admin, created_at, email_verified_at)
		 SELECT ?, ?, ?, 1, strftime('%s','now'), strftime('%s','now')
		 WHERE NOT EXISTS (SELECT 1 FROM user)`,
		email, displayName, passwordHash)
	if err != nil {
		return 0, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if rows == 0 {
		return 0, ErrBootstrapAlreadyClaimed
	}
	return res.LastInsertId()
}

func CreateUnverifiedUserWithEmailVerification(db *sql.DB, email, displayName, passwordHash string, expiresAt int64) (int64, string, error) {
	if err := ensurePasswordlessProfileColumn(db); err != nil {
		return 0, "", err
	}
	raw, err := auth.GenerateToken()
	if err != nil {
		return 0, "", err
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, "", err
	}
	defer tx.Rollback()
	res, err := tx.Exec(
		`INSERT INTO user(email, display_name, password_hash, is_admin, is_passwordless_profile, created_at, email_verified_at)
		 VALUES(?,?,?,?,0,strftime('%s','now'),0)`,
		email, displayName, passwordHash, 0)
	if err != nil {
		return 0, "", err
	}
	userID, err := res.LastInsertId()
	if err != nil {
		return 0, "", err
	}
	_, err = tx.Exec(
		`INSERT INTO email_verification(token_hash, user_id, created_at, expires_at)
		 VALUES(?,?,strftime('%s','now'),?)`,
		auth.HashToken(raw), userID, expiresAt)
	if err != nil {
		return 0, "", err
	}
	if err := tx.Commit(); err != nil {
		return 0, "", err
	}
	return userID, raw, nil
}

// GetUserByEmail looks up an account by email. ok is false when none exists.
func GetUserByEmail(db *sql.DB, email string) (User, bool, error) {
	if err := ensurePasswordlessProfileColumn(db); err != nil {
		return User{}, false, err
	}
	return scanUser(db.QueryRow(
		`SELECT id, email, display_name, password_hash, is_admin, is_passwordless_profile, created_at, email_verified_at
		 FROM user WHERE email = ?`, email))
}

// GetUserByID looks up an account by id (used to resolve a session's user).
func GetUserByID(db *sql.DB, id int64) (User, bool, error) {
	if err := ensurePasswordlessProfileColumn(db); err != nil {
		return User{}, false, err
	}
	return scanUser(db.QueryRow(
		`SELECT id, email, display_name, password_hash, is_admin, is_passwordless_profile, created_at, email_verified_at
		 FROM user WHERE id = ?`, id))
}

// ListUsers returns all accounts ordered by creation.
func ListUsers(db *sql.DB) ([]User, error) {
	if err := ensurePasswordlessProfileColumn(db); err != nil {
		return nil, err
	}
	rows, err := db.Query(
		`SELECT id, email, display_name, password_hash, is_admin, is_passwordless_profile, created_at, email_verified_at
		 FROM user ORDER BY created_at, id`)
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

// DeleteUser removes an account; its sessions cascade away (ON DELETE CASCADE).
// The user's personal stream goes with them in the same transaction: nothing
// else deletes it, because rename and delete refuse private streams outright,
// so without this the row and its queued tracks would be stranded behind a
// foreign key with no API path left to reach them.
//
// history rows for that stream are deliberately left, matching DeleteStream:
// they carry no foreign key, and the fairness window still reads them. Discovery
// no longer does — it counts a play only on a stream row that still exists, so a
// deleted user's plays stop shaping anyone's rediscover ranking (#151).
func DeleteUser(db *sql.DB, id int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := deleteStreamTx(tx, PersonalStreamID(id)); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM user WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func scanUser(row *sql.Row) (User, bool, error) {
	u, err := scanUserRow(row)
	if err == sql.ErrNoRows {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	return u, true, nil
}

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

func ensurePasswordlessProfileColumn(db *sql.DB) error {
	_, err := db.Exec(`ALTER TABLE user ADD COLUMN is_passwordless_profile INTEGER NOT NULL DEFAULT 0`)
	if err == nil {
		return nil
	}
	return nil
}

func MarkUserEmailVerified(db *sql.DB, userID int64) error {
	_, err := db.Exec(`UPDATE user SET email_verified_at = strftime('%s','now') WHERE id = ? AND email_verified_at = 0`, userID)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
