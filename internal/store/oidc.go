package store

import "database/sql"

// UserByOIDCIdentity resolves an external identity to its local account. The key
// is issuer + subject, never the email: a provider's subject is stable for the
// life of the account, while an address can be renamed or handed to someone
// else, which would silently move the link to a different person.
func UserByOIDCIdentity(db *sql.DB, issuer, subject string) (User, bool, error) {
	var uid int64
	err := db.QueryRow(
		`SELECT user_id FROM oidc_identity WHERE issuer = ? AND subject = ?`,
		issuer, subject).Scan(&uid)
	if err == sql.ErrNoRows {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	return GetUserByID(db, uid)
}

// LinkOIDCIdentity records issuer + subject as an external identity of userID.
// The PRIMARY KEY surfaces as an error if that identity is already linked —
// re-pointing an existing link at another account is never an accident, so it
// is refused rather than upserted.
func LinkOIDCIdentity(db *sql.DB, issuer, subject string, userID int64) error {
	_, err := db.Exec(
		`INSERT INTO oidc_identity(issuer, subject, user_id, created_at)
		 VALUES(?,?,?,strftime('%s','now'))`,
		issuer, subject, userID)
	return err
}
