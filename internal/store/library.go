package store

import (
	"database/sql"

	"github.com/andybarilla/exit66jukebox/internal/model"
)

func upsertArtist(db *sql.DB, name string) (int64, error) {
	if _, err := db.Exec(
		`INSERT INTO artist(name) VALUES(?) ON CONFLICT(name) DO NOTHING`, name,
	); err != nil {
		return 0, err
	}
	var id int64
	err := db.QueryRow(`SELECT id FROM artist WHERE name = ?`, name).Scan(&id)
	return id, err
}

func upsertAlbum(db *sql.DB, name string, artistID int64) (int64, error) {
	if _, err := db.Exec(
		`INSERT INTO album(name, artist_id) VALUES(?, ?)
		 ON CONFLICT(name, artist_id) DO NOTHING`, name, artistID,
	); err != nil {
		return 0, err
	}
	var id int64
	err := db.QueryRow(
		`SELECT id FROM album WHERE name = ? AND artist_id = ?`, name, artistID,
	).Scan(&id)
	return id, err
}

// UpsertTrack inserts or updates a track by its path, creating the artist and
// album rows as needed. Returns the track id.
func UpsertTrack(db *sql.DB, t model.Track, artistName, albumName string) (int64, error) {
	artistID, err := upsertArtist(db, artistName)
	if err != nil {
		return 0, err
	}
	albumID, err := upsertAlbum(db, albumName, artistID)
	if err != nil {
		return 0, err
	}
	_, err = db.Exec(
		`INSERT INTO track(path, mod_time, size, title, artist_id, album_id, track_no, genre, duration)
		 VALUES(?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(path) DO UPDATE SET
		   mod_time=excluded.mod_time, size=excluded.size, title=excluded.title,
		   artist_id=excluded.artist_id, album_id=excluded.album_id,
		   track_no=excluded.track_no, genre=excluded.genre, duration=excluded.duration`,
		t.Path, t.ModTime, t.Size, t.Title, artistID, albumID, t.TrackNo, t.Genre, t.Duration,
	)
	if err != nil {
		return 0, err
	}
	var id int64
	err = db.QueryRow(`SELECT id FROM track WHERE path = ?`, t.Path).Scan(&id)
	return id, err
}

// TrackStamp returns the stored mod_time and size for a path, or ok=false if
// the path is not indexed. Used by the scanner to skip unchanged files.
func TrackStamp(db *sql.DB, path string) (modTime, size int64, ok bool) {
	err := db.QueryRow(
		`SELECT mod_time, size FROM track WHERE path = ?`, path,
	).Scan(&modTime, &size)
	if err != nil {
		return 0, 0, false
	}
	return modTime, size, true
}
