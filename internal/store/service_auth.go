package store

import "database/sql"

// GetServiceAuth returns the persisted session key + username for service. ok is
// false when no row exists (the service was never authorized).
func GetServiceAuth(db *sql.DB, service string) (sessionKey, username string, ok bool, err error) {
	err = db.QueryRow(
		`SELECT session_key, username FROM service_auth WHERE service = ?`, service,
	).Scan(&sessionKey, &username)
	if err == sql.ErrNoRows {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return sessionKey, username, true, nil
}

// PutServiceAuth upserts the session key + username for service, stamping
// created_at with the current time.
func PutServiceAuth(db *sql.DB, service, sessionKey, username string) error {
	_, err := db.Exec(
		`INSERT INTO service_auth(service, session_key, username, created_at)
		 VALUES(?,?,?,strftime('%s','now'))
		 ON CONFLICT(service) DO UPDATE SET
		   session_key = excluded.session_key,
		   username    = excluded.username,
		   created_at  = excluded.created_at`,
		service, sessionKey, username)
	return err
}

// DeleteServiceAuth removes a service's persisted auth (e.g. after an invalid
// session key), reverting it to pending-auth.
func DeleteServiceAuth(db *sql.DB, service string) error {
	_, err := db.Exec(`DELETE FROM service_auth WHERE service = ?`, service)
	return err
}
