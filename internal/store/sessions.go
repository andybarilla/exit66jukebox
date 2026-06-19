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

func DeleteSessionsForUser(db *sql.DB, userID int64) error {
	_, err := db.Exec(`DELETE FROM session WHERE user_id = ?`, userID)
	return err
}

// PurgeExpiredSessions deletes timed-out rows. Called opportunistically at
// startup so the table doesn't grow unbounded.
func PurgeExpiredSessions(db *sql.DB) error {
	_, err := db.Exec(`DELETE FROM session WHERE expires_at <= strftime('%s','now')`)
	return err
}
