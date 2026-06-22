package store

import (
	"database/sql"

	"github.com/andybarilla/exit66jukebox/internal/auth"
)

type EmailVerification struct {
	ID        int64
	UserID    int64
	CreatedAt int64
	ExpiresAt int64
	UsedAt    int64
}

func CreateEmailVerification(db *sql.DB, userID, expiresAt int64) (string, error) {
	raw, err := auth.GenerateToken()
	if err != nil {
		return "", err
	}
	_, err = db.Exec(
		`INSERT INTO email_verification(token_hash, user_id, created_at, expires_at)
		 VALUES(?,?,strftime('%s','now'),?)`,
		auth.HashToken(raw), userID, expiresAt)
	if err != nil {
		return "", err
	}
	return raw, nil
}

func PendingEmailVerification(db *sql.DB, rawToken string) (EmailVerification, bool, error) {
	if rawToken == "" {
		return EmailVerification{}, false, nil
	}
	var verification EmailVerification
	err := db.QueryRow(
		`SELECT id, user_id, created_at, expires_at, used_at
		 FROM email_verification
		 WHERE token_hash = ? AND used_at = 0 AND expires_at > strftime('%s','now')`,
		auth.HashToken(rawToken)).Scan(
		&verification.ID,
		&verification.UserID,
		&verification.CreatedAt,
		&verification.ExpiresAt,
		&verification.UsedAt)
	if err == sql.ErrNoRows {
		return EmailVerification{}, false, nil
	}
	if err != nil {
		return EmailVerification{}, false, err
	}
	return verification, true, nil
}

func ConsumeEmailVerification(db *sql.DB, rawToken string) error {
	if rawToken == "" {
		return sql.ErrNoRows
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var verification EmailVerification
	err = tx.QueryRow(
		`SELECT id, user_id, created_at, expires_at, used_at
		 FROM email_verification
		 WHERE token_hash = ? AND used_at = 0 AND expires_at > strftime('%s','now')`,
		auth.HashToken(rawToken)).Scan(
		&verification.ID,
		&verification.UserID,
		&verification.CreatedAt,
		&verification.ExpiresAt,
		&verification.UsedAt)
	if err != nil {
		return err
	}
	res, err := tx.Exec(`UPDATE email_verification SET used_at = strftime('%s','now') WHERE id = ? AND used_at = 0`, verification.ID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return sql.ErrNoRows
	}
	if _, err := tx.Exec(`UPDATE user SET email_verified_at = strftime('%s','now') WHERE id = ? AND email_verified_at = 0`, verification.UserID); err != nil {
		return err
	}
	return tx.Commit()
}

func RegenerateEmailVerification(db *sql.DB, userID, expiresAt int64) (string, error) {
	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE email_verification SET used_at = strftime('%s','now') WHERE user_id = ? AND used_at = 0`, userID); err != nil {
		return "", err
	}
	raw, err := auth.GenerateToken()
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(
		`INSERT INTO email_verification(token_hash, user_id, created_at, expires_at)
		 VALUES(?,?,strftime('%s','now'),?)`,
		auth.HashToken(raw), userID, expiresAt); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return raw, nil
}
