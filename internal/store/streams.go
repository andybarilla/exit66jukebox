package store

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/andybarilla/exit66jukebox/internal/model"
)

// Stream kinds. A shared stream has its own broadcast pipeline and is
// admin-gated; a private stream is one listener's own queue and is not.
const (
	KindShared  = "shared"
	KindPrivate = "private"
)

// MaxSharedStreams caps how many shared streams can exist at once, house
// included. Each one owns a broadcast pipeline and (once it has a listener) an
// ffmpeg process, so the limit is a resource bound, not a policy.
const MaxSharedStreams = 4

var (
	// ErrStreamCapReached is returned by CreateSharedStream when MaxSharedStreams
	// shared streams already exist. Handlers map it to 409.
	ErrStreamCapReached = fmt.Errorf("shared stream limit reached (%d)", MaxSharedStreams)
	// ErrStreamExists is returned when the id is already taken.
	ErrStreamExists = errors.New("stream already exists")
)

// EnsurePrivateStream creates the stream row as private if it does not exist.
// This is the only implicit-create path: it can never produce a shared stream,
// which is what keeps the kind-based authorization gate from being bypassed by
// inventing a stream URL.
func EnsurePrivateStream(db *sql.DB, id string) error {
	_, err := db.Exec(
		`INSERT INTO stream(id, name, kind) VALUES(?,'',?)
		 ON CONFLICT(id) DO NOTHING`, id, KindPrivate)
	return err
}

// EnsureSharedStream creates a shared stream row if absent, without consulting
// the cap. It exists for the always-on house stream, which must come up at boot
// even on an instance that is already at (or over) the limit.
func EnsureSharedStream(db *sql.DB, id, name string) error {
	_, err := db.Exec(
		`INSERT INTO stream(id, name, kind) VALUES(?,?,?)
		 ON CONFLICT(id) DO NOTHING`, id, name, KindShared)
	return err
}

// CreateSharedStream creates a named shared stream, enforcing MaxSharedStreams.
// The count and the insert share one transaction so the cap cannot be exceeded
// by two concurrent creates, and so no caller can opt out of it.
func CreateSharedStream(db *sql.DB, id, name string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var existing int
	if err := tx.QueryRow(`SELECT count(*) FROM stream WHERE id=?`, id).Scan(&existing); err != nil {
		return err
	}
	if existing > 0 {
		return ErrStreamExists
	}
	var n int
	if err := tx.QueryRow(`SELECT count(*) FROM stream WHERE kind=?`, KindShared).Scan(&n); err != nil {
		return err
	}
	if n >= MaxSharedStreams {
		return ErrStreamCapReached
	}
	if _, err := tx.Exec(`INSERT INTO stream(id, name, kind) VALUES(?,?,?)`,
		id, name, KindShared); err != nil {
		return err
	}
	return tx.Commit()
}

// GetStream returns one stream row; ok is false when the id is unknown.
func GetStream(db *sql.DB, id string) (model.Stream, bool, error) {
	var st model.Stream
	err := db.QueryRow(`SELECT id, name, kind FROM stream WHERE id=?`, id).
		Scan(&st.ID, &st.Name, &st.Kind)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Stream{}, false, nil
	}
	if err != nil {
		return model.Stream{}, false, err
	}
	return st, true, nil
}

// ListStreams returns the streams of one kind, id-ordered. An empty kind lists
// every stream.
func ListStreams(db *sql.DB, kind string) ([]model.Stream, error) {
	var rows *sql.Rows
	var err error
	if kind == "" {
		rows, err = db.Query(`SELECT id, name, kind FROM stream ORDER BY id`)
	} else {
		rows, err = db.Query(`SELECT id, name, kind FROM stream WHERE kind=? ORDER BY id`, kind)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Stream
	for rows.Next() {
		var st model.Stream
		if err := rows.Scan(&st.ID, &st.Name, &st.Kind); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// RenameStream changes the display name only; the id is the stable handle and
// names may collide.
func RenameStream(db *sql.DB, id, name string) error {
	_, err := db.Exec(`UPDATE stream SET name=? WHERE id=?`, name, id)
	return err
}

// DeleteStream removes the stream and the rows that reference it. queue_item
// and station both carry a foreign key to stream(id), so they go first or the
// delete fails. history rows are left in place: they carry no foreign key and
// dropping them would discard play data the fairness window and Discovery read.
func DeleteStream(db *sql.DB, id string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, q := range []string{
		`DELETE FROM queue_item WHERE stream_id=?`,
		`DELETE FROM station WHERE stream_id=?`,
		`DELETE FROM stream WHERE id=?`,
	} {
		if _, err := tx.Exec(q, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
