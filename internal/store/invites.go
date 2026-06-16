package store

import "database/sql"

// Invite is an outstanding (or historical) invitation. Email is optional (a
// hint / SMTP recipient). IsAdmin means redemption yields an admin account.
type Invite struct {
	ID         int64
	Email      string
	IsAdmin    bool
	CreatedBy  int64
	CreatedAt  int64
	ExpiresAt  int64
	AcceptedAt int64
}

// CreateInvite stores an invite keyed by its token hash and returns its id.
func CreateInvite(db *sql.DB, tokenHash, email string, isAdmin bool, createdBy, expiresAt int64) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO invite(token_hash, email, is_admin, created_by, created_at, expires_at)
		 VALUES(?,?,?,?,strftime('%s','now'),?)`,
		tokenHash, email, boolToInt(isAdmin), createdBy, expiresAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// PendingInvite returns the unexpired, unaccepted invite for tokenHash. ok is
// false when it is missing, already accepted, or expired.
func PendingInvite(db *sql.DB, tokenHash string) (Invite, bool, error) {
	var inv Invite
	var admin int
	err := db.QueryRow(
		`SELECT id, email, is_admin, created_by, created_at, expires_at, accepted_at
		 FROM invite
		 WHERE token_hash = ? AND accepted_at = 0 AND expires_at > strftime('%s','now')`,
		tokenHash).Scan(&inv.ID, &inv.Email, &admin, &inv.CreatedBy, &inv.CreatedAt, &inv.ExpiresAt, &inv.AcceptedAt)
	if err == sql.ErrNoRows {
		return Invite{}, false, nil
	}
	if err != nil {
		return Invite{}, false, err
	}
	inv.IsAdmin = admin != 0
	return inv, true, nil
}

// MarkInviteAccepted stamps accepted_at so the invite can't be reused.
func MarkInviteAccepted(db *sql.DB, id int64) error {
	_, err := db.Exec(`UPDATE invite SET accepted_at = strftime('%s','now') WHERE id = ?`, id)
	return err
}

// ListInvites returns all invites (pending and historical), newest first.
func ListInvites(db *sql.DB) ([]Invite, error) {
	rows, err := db.Query(
		`SELECT id, email, is_admin, created_by, created_at, expires_at, accepted_at
		 FROM invite ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Invite
	for rows.Next() {
		var inv Invite
		var admin int
		if err := rows.Scan(&inv.ID, &inv.Email, &admin, &inv.CreatedBy, &inv.CreatedAt, &inv.ExpiresAt, &inv.AcceptedAt); err != nil {
			return nil, err
		}
		inv.IsAdmin = admin != 0
		out = append(out, inv)
	}
	return out, rows.Err()
}

// DeleteInvite revokes an invite by id.
func DeleteInvite(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM invite WHERE id = ?`, id)
	return err
}
