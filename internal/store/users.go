package store

import (
	"database/sql"

	"github.com/andybarilla/exit66jukebox/internal/auth"
)

// User is an account row. PasswordHash is the encoded pbkdf2 string from
// internal/auth; this package never hashes or compares passwords itself.
type User struct {
	ID              int64
	Email           string
	DisplayName     string
	PasswordHash    string
	IsAdmin         bool
	CreatedAt       int64
	EmailVerifiedAt int64
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
	emailVerifiedAtExpr := "0"
	if len(verified) == 0 || verified[0] {
		emailVerifiedAtExpr = "strftime('%s','now')"
	}
	res, err := db.Exec(
		`INSERT INTO user(email, display_name, password_hash, is_admin, created_at, email_verified_at)
		 VALUES(?,?,?,?,strftime('%s','now'),`+emailVerifiedAtExpr+`)`,
		email, displayName, passwordHash, boolToInt(isAdmin))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func CreateUnverifiedUserWithEmailVerification(db *sql.DB, email, displayName, passwordHash string, expiresAt int64) (int64, string, error) {
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
		`INSERT INTO user(email, display_name, password_hash, is_admin, created_at, email_verified_at)
		 VALUES(?,?,?,?,strftime('%s','now'),0)`,
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
	return scanUser(db.QueryRow(
		`SELECT id, email, display_name, password_hash, is_admin, created_at, email_verified_at
		 FROM user WHERE email = ?`, email))
}

// GetUserByID looks up an account by id (used to resolve a session's user).
func GetUserByID(db *sql.DB, id int64) (User, bool, error) {
	return scanUser(db.QueryRow(
		`SELECT id, email, display_name, password_hash, is_admin, created_at, email_verified_at
		 FROM user WHERE id = ?`, id))
}

// ListUsers returns all accounts ordered by creation.
func ListUsers(db *sql.DB) ([]User, error) {
	rows, err := db.Query(
		`SELECT id, email, display_name, password_hash, is_admin, created_at, email_verified_at
		 FROM user ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		var admin int
		if err := rows.Scan(&u.ID, &u.Email, &u.DisplayName, &u.PasswordHash, &admin, &u.CreatedAt, &u.EmailVerifiedAt); err != nil {
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
	err := row.Scan(&u.ID, &u.Email, &u.DisplayName, &u.PasswordHash, &admin, &u.CreatedAt, &u.EmailVerifiedAt)
	if err == sql.ErrNoRows {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	u.IsAdmin = admin != 0
	return u, true, nil
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
