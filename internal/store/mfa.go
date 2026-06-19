package store

import "database/sql"

type MFAFactor struct {
	UserID           int64
	SecretCiphertext []byte
	SecretNonce      []byte
	KeyVersion       int
	EnabledAt        int64
	LastAcceptedStep int64
}

func GetMFAFactor(db *sql.DB, userID int64) (MFAFactor, bool, error) {
	var factor MFAFactor
	err := db.QueryRow(
		`SELECT user_id, secret_ciphertext, secret_nonce, key_version, enabled_at, last_accepted_step
		 FROM mfa_factor WHERE user_id = ?`,
		userID).Scan(
		&factor.UserID,
		&factor.SecretCiphertext,
		&factor.SecretNonce,
		&factor.KeyVersion,
		&factor.EnabledAt,
		&factor.LastAcceptedStep)
	if err == sql.ErrNoRows {
		return MFAFactor{}, false, nil
	}
	if err != nil {
		return MFAFactor{}, false, err
	}
	return factor, true, nil
}

func UpsertMFAFactor(db *sql.DB, factor MFAFactor) error {
	_, err := db.Exec(
		`INSERT INTO mfa_factor(user_id, secret_ciphertext, secret_nonce, key_version, enabled_at, last_accepted_step)
		 VALUES(?,?,?,?,?,?)
		 ON CONFLICT(user_id) DO UPDATE SET
			secret_ciphertext = excluded.secret_ciphertext,
			secret_nonce = excluded.secret_nonce,
			key_version = excluded.key_version,
			enabled_at = excluded.enabled_at,
			last_accepted_step = excluded.last_accepted_step`,
		factor.UserID,
		factor.SecretCiphertext,
		factor.SecretNonce,
		factor.KeyVersion,
		factor.EnabledAt,
		factor.LastAcceptedStep)
	return err
}

func DisableMFAFactor(db *sql.DB, userID int64) error {
	_, err := db.Exec(`DELETE FROM mfa_factor WHERE user_id = ?`, userID)
	return err
}

func UpdateMFALastAcceptedStep(db *sql.DB, userID int64, step int64) (bool, error) {
	res, err := db.Exec(
		`UPDATE mfa_factor
		 SET last_accepted_step = ?
		 WHERE user_id = ? AND last_accepted_step < ?`,
		step, userID, step)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

func CreateMFATicket(db *sql.DB, ticketHash string, userID, expiresAt int64) error {
	_, err := db.Exec(
		`INSERT INTO mfa_ticket(ticket_hash, user_id, created_at, expires_at)
		 VALUES(?,?,strftime('%s','now'),?)`,
		ticketHash, userID, expiresAt)
	return err
}

func ConsumeMFATicket(db *sql.DB, ticketHash string) (int64, bool, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()

	var userID int64
	err = tx.QueryRow(
		`SELECT user_id FROM mfa_ticket
		 WHERE ticket_hash = ? AND used_at = 0 AND expires_at > strftime('%s','now')`,
		ticketHash).Scan(&userID)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}

	res, err := tx.Exec(
		`UPDATE mfa_ticket
		 SET used_at = strftime('%s','now')
		 WHERE ticket_hash = ? AND used_at = 0 AND expires_at > strftime('%s','now')`,
		ticketHash)
	if err != nil {
		return 0, false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return 0, false, err
	}
	if rows != 1 {
		return 0, false, nil
	}
	if err := tx.Commit(); err != nil {
		return 0, false, err
	}
	return userID, true, nil
}

func ReplaceRecoveryCodes(db *sql.DB, userID int64, codeHashes []string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM mfa_recovery_code WHERE user_id = ?`, userID); err != nil {
		return err
	}
	for _, codeHash := range codeHashes {
		_, err := tx.Exec(
			`INSERT INTO mfa_recovery_code(user_id, code_hash, created_at)
			 VALUES(?,?,strftime('%s','now'))`,
			userID, codeHash)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func ListRecoveryCodeHashes(db *sql.DB, userID int64) ([]string, error) {
	rows, err := db.Query(
		`SELECT code_hash FROM mfa_recovery_code
		 WHERE user_id = ? AND used_at = 0
		 ORDER BY id`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var codeHashes []string
	for rows.Next() {
		var codeHash string
		if err := rows.Scan(&codeHash); err != nil {
			return nil, err
		}
		codeHashes = append(codeHashes, codeHash)
	}
	return codeHashes, rows.Err()
}

func MarkRecoveryCodeUsed(db *sql.DB, userID int64, codeHash string) (bool, error) {
	tx, err := db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`UPDATE mfa_recovery_code
		 SET used_at = strftime('%s','now')
		 WHERE user_id = ? AND code_hash = ? AND used_at = 0`,
		userID, codeHash)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if rows != 1 {
		return false, nil
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
