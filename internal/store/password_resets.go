package store

import "database/sql"

type PasswordReset struct {
	ID        int64
	UserID    int64
	CreatedAt int64
	ExpiresAt int64
	UsedAt    int64
}

func CreatePasswordReset(db *sql.DB, tokenHash string, userID, expiresAt int64) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO password_reset(token_hash, user_id, created_at, expires_at)
		 VALUES(?,?,strftime('%s','now'),?)`,
		tokenHash, userID, expiresAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func PendingPasswordReset(db *sql.DB, tokenHash string) (PasswordReset, bool, error) {
	var reset PasswordReset
	err := db.QueryRow(
		`SELECT id, user_id, created_at, expires_at, used_at
		 FROM password_reset
		 WHERE token_hash = ? AND used_at = 0 AND expires_at > strftime('%s','now')`,
		tokenHash).Scan(&reset.ID, &reset.UserID, &reset.CreatedAt, &reset.ExpiresAt, &reset.UsedAt)
	if err == sql.ErrNoRows {
		return PasswordReset{}, false, nil
	}
	if err != nil {
		return PasswordReset{}, false, err
	}
	return reset, true, nil
}

func MarkPasswordResetUsed(db *sql.DB, id int64) error {
	_, err := db.Exec(`UPDATE password_reset SET used_at = strftime('%s','now') WHERE id = ?`, id)
	return err
}

func ConsumePasswordReset(db *sql.DB, tokenHash string) (PasswordReset, bool, error) {
	tx, err := db.Begin()
	if err != nil {
		return PasswordReset{}, false, err
	}
	defer tx.Rollback()

	var reset PasswordReset
	err = tx.QueryRow(
		`SELECT id, user_id, created_at, expires_at, used_at
		 FROM password_reset
		 WHERE token_hash = ? AND used_at = 0 AND expires_at > strftime('%s','now')`,
		tokenHash).Scan(&reset.ID, &reset.UserID, &reset.CreatedAt, &reset.ExpiresAt, &reset.UsedAt)
	if err == sql.ErrNoRows {
		return PasswordReset{}, false, nil
	}
	if err != nil {
		return PasswordReset{}, false, err
	}

	res, err := tx.Exec(`UPDATE password_reset SET used_at = strftime('%s','now') WHERE id = ? AND used_at = 0`, reset.ID)
	if err != nil {
		return PasswordReset{}, false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return PasswordReset{}, false, err
	}
	if rows != 1 {
		return PasswordReset{}, false, nil
	}
	if err := tx.Commit(); err != nil {
		return PasswordReset{}, false, err
	}
	return reset, true, nil
}
