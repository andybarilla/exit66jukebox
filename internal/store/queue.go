package store

import "database/sql"

// EnsureStream creates the stream row if absent.
func EnsureStream(db *sql.DB, id, name, kind string) error {
	_, err := db.Exec(
		`INSERT INTO stream(id, name, kind) VALUES(?,?,?)
		 ON CONFLICT(id) DO NOTHING`, id, name, kind)
	return err
}

// InQueue reports whether a track is already queued in the stream.
func InQueue(db *sql.DB, streamID string, trackID int64) (bool, error) {
	var n int
	err := db.QueryRow(
		`SELECT count(*) FROM queue_item WHERE stream_id=? AND track_id=?`,
		streamID, trackID).Scan(&n)
	return n > 0, err
}

// RecentlyPlayed reports whether a track is within the last `window` plays of
// the stream's history.
func RecentlyPlayed(db *sql.DB, streamID string, trackID int64, window int) (bool, error) {
	var n int
	err := db.QueryRow(
		`SELECT count(*) FROM (
		    SELECT track_id FROM history WHERE stream_id=?
		    ORDER BY played_at DESC LIMIT ?
		 ) WHERE track_id=?`, streamID, window, trackID).Scan(&n)
	return n > 0, err
}

// Enqueue appends a track to the end of the stream's queue.
func Enqueue(db *sql.DB, streamID string, trackID int64, addedBy string) error {
	var next int
	if err := db.QueryRow(
		`SELECT coalesce(max(play_order),0)+1 FROM queue_item WHERE stream_id=?`,
		streamID).Scan(&next); err != nil {
		return err
	}
	_, err := db.Exec(
		`INSERT INTO queue_item(stream_id, track_id, play_order, added_by) VALUES(?,?,?,?)`,
		streamID, trackID, next, addedBy)
	return err
}

// PopNext removes and returns the next track id in play order, records it in
// history, and bumps its play count — all atomically. Returns ok=false if the
// queue is empty or the transaction fails.
func PopNext(db *sql.DB, streamID string) (trackID int64, ok bool) {
	tx, err := db.Begin()
	if err != nil {
		return 0, false
	}
	defer tx.Rollback()

	if err := tx.QueryRow(
		`SELECT track_id FROM queue_item WHERE stream_id=? ORDER BY play_order LIMIT 1`,
		streamID).Scan(&trackID); err != nil {
		return 0, false
	}
	if _, err := tx.Exec(`DELETE FROM queue_item WHERE stream_id=? AND track_id=?`,
		streamID, trackID); err != nil {
		return 0, false
	}
	if _, err := tx.Exec(
		`INSERT INTO history(stream_id, track_id, played_at) VALUES(?,?,strftime('%s','now'))`,
		streamID, trackID); err != nil {
		return 0, false
	}
	if _, err := tx.Exec(`UPDATE track SET play_count = play_count + 1 WHERE id=?`,
		trackID); err != nil {
		return 0, false
	}
	if err := tx.Commit(); err != nil {
		return 0, false
	}
	return trackID, true
}

// RemoveFromQueue drops a single track from the stream's queue.
func RemoveFromQueue(db *sql.DB, streamID string, trackID int64) error {
	_, err := db.Exec(
		`DELETE FROM queue_item WHERE stream_id=? AND track_id=?`, streamID, trackID)
	return err
}

// ClearQueue empties the stream's queue.
func ClearQueue(db *sql.DB, streamID string) error {
	_, err := db.Exec(`DELETE FROM queue_item WHERE stream_id=?`, streamID)
	return err
}

// QueuedRow is a queued track id paired with who requested it, in play order.
type QueuedRow struct {
	TrackID     int64
	RequestedBy string
}

// QueueWithRequester returns the queued rows (track id + requester) in play order.
func QueueWithRequester(db *sql.DB, streamID string) ([]QueuedRow, error) {
	rows, err := db.Query(
		`SELECT track_id, added_by FROM queue_item WHERE stream_id=? ORDER BY play_order`, streamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []QueuedRow
	for rows.Next() {
		var r QueuedRow
		if err := rows.Scan(&r.TrackID, &r.RequestedBy); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// QueueTrackIDs returns the queued track ids in play order.
func QueueTrackIDs(db *sql.DB, streamID string) ([]int64, error) {
	rows, err := db.Query(
		`SELECT track_id FROM queue_item WHERE stream_id=? ORDER BY play_order`, streamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
