package store

import "database/sql"

func UpdateUserPassword(db *sql.DB, userID int64, passwordHash string) error {
	_, err := db.Exec(`UPDATE user SET password_hash = ? WHERE id = ?`, passwordHash, userID)
	return err
}
