package store

import "database/sql"

// Station is a continuous genre radio attached to one stream. When the stream's
// queue falls below Threshold, Batch more tracks of Genre are enqueued.
type Station struct {
	StreamID  string `json:"stream_id"`
	Genre     string `json:"genre"`
	Threshold int    `json:"threshold"`
	Batch     int    `json:"batch"`
}

// GetStation returns the station for a stream, ok=false if none is set.
func GetStation(db *sql.DB, streamID string) (Station, bool) {
	var s Station
	err := db.QueryRow(
		`SELECT stream_id, genre, threshold, batch FROM station WHERE stream_id = ?`,
		streamID).Scan(&s.StreamID, &s.Genre, &s.Threshold, &s.Batch)
	if err != nil {
		return Station{}, false
	}
	return s, true
}

// UpsertStation creates or replaces the station for a stream.
func UpsertStation(db *sql.DB, s Station) error {
	_, err := db.Exec(
		`INSERT INTO station(stream_id, genre, threshold, batch) VALUES(?,?,?,?)
		 ON CONFLICT(stream_id) DO UPDATE SET
		   genre=excluded.genre, threshold=excluded.threshold, batch=excluded.batch`,
		s.StreamID, s.Genre, s.Threshold, s.Batch)
	return err
}

// DeleteStation removes a stream's station, halting future refills.
func DeleteStation(db *sql.DB, streamID string) error {
	_, err := db.Exec(`DELETE FROM station WHERE stream_id = ?`, streamID)
	return err
}

// QueueLen returns the number of tracks currently queued on a stream.
func QueueLen(db *sql.DB, streamID string) (int, error) {
	var n int
	err := db.QueryRow(
		`SELECT count(*) FROM queue_item WHERE stream_id = ?`, streamID).Scan(&n)
	return n, err
}
