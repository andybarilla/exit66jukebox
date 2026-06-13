package store

import (
	"database/sql"
	"strings"
)

// ScrobbleRow is one pending listen awaiting delivery to a service.
type ScrobbleRow struct {
	ID       int64
	Service  string
	TrackID  int64
	PlayedAt int64
	Attempts int
}

// ScrobbleMeta is the track identity a listen submission needs.
type ScrobbleMeta struct {
	ArtistName  string
	TrackName   string
	ReleaseName string
	Duration    int
}

// EnqueueScrobble writes one scrobble_queue row per service for a completed
// listen of trackID at playedAt (unix seconds).
func EnqueueScrobble(db *sql.DB, services []string, trackID, playedAt int64) error {
	for _, svc := range services {
		if _, err := db.Exec(
			`INSERT INTO scrobble_queue(service, track_id, played_at, created_at)
			 VALUES(?,?,?,strftime('%s','now'))`, svc, trackID, playedAt); err != nil {
			return err
		}
	}
	return nil
}

// ScrobbleBatch returns up to limit pending rows for service, oldest first.
func ScrobbleBatch(db *sql.DB, service string, limit int) ([]ScrobbleRow, error) {
	rows, err := db.Query(
		`SELECT id, service, track_id, played_at, attempts
		 FROM scrobble_queue WHERE service=? ORDER BY id LIMIT ?`, service, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ScrobbleRow
	for rows.Next() {
		var r ScrobbleRow
		if err := rows.Scan(&r.ID, &r.Service, &r.TrackID, &r.PlayedAt, &r.Attempts); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteScrobble removes a queue row by id (called after a successful submit).
func DeleteScrobble(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM scrobble_queue WHERE id=?`, id)
	return err
}

// BumpScrobbleAttempts increments attempts for the given rows after a failed
// delivery, so a poison row's growth is visible.
func BumpScrobbleAttempts(db *sql.DB, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	ph := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args[i] = id
	}
	_, err := db.Exec(
		`UPDATE scrobble_queue SET attempts = attempts + 1 WHERE id IN (`+strings.Join(ph, ",")+`)`, args...)
	return err
}

// ScrobbleMetadata resolves a track id to the artist/track/release names and
// duration a listen submission carries. ok is false if the track is gone.
func ScrobbleMetadata(db *sql.DB, trackID int64) (ScrobbleMeta, bool, error) {
	var m ScrobbleMeta
	err := db.QueryRow(
		`SELECT ar.name, t.title, al.name, t.duration
		 FROM track t
		 JOIN artist ar ON ar.id = t.artist_id
		 JOIN album  al ON al.id = t.album_id
		 WHERE t.id = ?`, trackID).Scan(&m.ArtistName, &m.TrackName, &m.ReleaseName, &m.Duration)
	if err == sql.ErrNoRows {
		return ScrobbleMeta{}, false, nil
	}
	if err != nil {
		return ScrobbleMeta{}, false, err
	}
	return m, true, nil
}
