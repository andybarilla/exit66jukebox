package store

import (
	"database/sql"

	"github.com/andybarilla/exit66jukebox/internal/model"
)

type execQuerier interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
}

func upsertArtist(q execQuerier, name string) (int64, error) {
	if _, err := q.Exec(
		`INSERT INTO artist(name, sort_key) VALUES(?, ?) ON CONFLICT(name) DO NOTHING`,
		name, normalizeSortKey(name),
	); err != nil {
		return 0, err
	}
	var id int64
	err := q.QueryRow(`SELECT id FROM artist WHERE name = ?`, name).Scan(&id)
	return id, err
}

func upsertAlbum(q execQuerier, name string, artistID int64) (int64, error) {
	if _, err := q.Exec(
		`INSERT INTO album(name, artist_id, sort_key) VALUES(?, ?, ?)
		 ON CONFLICT(name, artist_id) DO NOTHING`, name, artistID, normalizeSortKey(name),
	); err != nil {
		return 0, err
	}
	var id int64
	err := q.QueryRow(
		`SELECT id FROM album WHERE name = ? AND artist_id = ?`, name, artistID,
	).Scan(&id)
	return id, err
}

// UpsertTrack inserts or updates a track by its path, creating the artist and
// album rows as needed. Returns the track id.
func UpsertTrack(db *sql.DB, t model.Track, artistName, albumName string) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	artistID, err := upsertArtist(tx, artistName)
	if err != nil {
		return 0, err
	}
	albumID, err := upsertAlbum(tx, albumName, artistID)
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec(
		`INSERT INTO track(path, mod_time, size, title, artist_id, album_id, track_no, genre, duration, added_at)
		 VALUES(?,?,?,?,?,?,?,?,?, strftime('%s','now'))
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
	if err = tx.QueryRow(`SELECT id FROM track WHERE path = ?`, t.Path).Scan(&id); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
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

// ListTracks returns tracks whose title matches the search substring (empty =
// all), ordered by title, paged by limit/offset. A limit <= 0 means no limit.
func ListTracks(db *sql.DB, search string, limit, offset int) ([]model.Track, error) {
	q := `SELECT id, title, artist_id, album_id, track_no, genre, duration, play_count
	      FROM track WHERE title LIKE ? ORDER BY title LIMIT ? OFFSET ?`
	lim := limit
	if lim <= 0 {
		lim = -1 // SQLite: no limit
	}
	rows, err := db.Query(q, "%"+search+"%", lim, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Track
	for rows.Next() {
		var t model.Track
		if err := rows.Scan(&t.ID, &t.Title, &t.ArtistID, &t.AlbumID,
			&t.TrackNo, &t.Genre, &t.Duration, &t.PlayCount); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListArtists returns artists matching search (empty = all), ordered by name.
func ListArtists(db *sql.DB, search string, limit, offset int) ([]model.Artist, error) {
	lim := limit
	if lim <= 0 {
		lim = -1
	}
	rows, err := db.Query(
		`SELECT id, name FROM artist WHERE name LIKE ? ORDER BY name LIMIT ? OFFSET ?`,
		"%"+search+"%", lim, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Artist
	for rows.Next() {
		var a model.Artist
		if err := rows.Scan(&a.ID, &a.Name); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListAlbums returns albums matching search (empty = all), ordered by name.
func ListAlbums(db *sql.DB, search string, limit, offset int) ([]model.Album, error) {
	lim := limit
	if lim <= 0 {
		lim = -1
	}
	rows, err := db.Query(
		`SELECT id, name, artist_id FROM album WHERE name LIKE ? ORDER BY name LIMIT ? OFFSET ?`,
		"%"+search+"%", lim, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Album
	for rows.Next() {
		var a model.Album
		if err := rows.Scan(&a.ID, &a.Name, &a.ArtistID); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// TrackIDsByAlbum returns track ids for an album in track-number order.
func TrackIDsByAlbum(db *sql.DB, albumID int64) ([]int64, error) {
	return scanIDs(db,
		`SELECT id FROM track WHERE album_id=? ORDER BY track_no, title`, albumID)
}

// TrackIDsByArtist returns track ids for an artist in title order.
func TrackIDsByArtist(db *sql.DB, artistID int64) ([]int64, error) {
	return scanIDs(db,
		`SELECT id FROM track WHERE artist_id=? ORDER BY title`, artistID)
}

// FirstTrackIDOfAlbum returns the lowest-numbered track id for an album, or
// ok=false if the album has no tracks.
func FirstTrackIDOfAlbum(db *sql.DB, albumID int64) (int64, bool) {
	var id int64
	err := db.QueryRow(
		`SELECT id FROM track WHERE album_id=? ORDER BY track_no, title LIMIT 1`,
		albumID).Scan(&id)
	if err != nil {
		return 0, false
	}
	return id, true
}

func scanIDs(db *sql.DB, q string, arg any) ([]int64, error) {
	rows, err := db.Query(q, arg)
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

// GetTrack returns a single track and its file path. ok=false if not found.
func GetTrack(db *sql.DB, id int64) (t model.Track, path string, ok bool) {
	err := db.QueryRow(
		`SELECT id, path, title, artist_id, album_id, track_no, genre, duration, play_count
		 FROM track WHERE id = ?`, id).Scan(
		&t.ID, &path, &t.Title, &t.ArtistID, &t.AlbumID,
		&t.TrackNo, &t.Genre, &t.Duration, &t.PlayCount)
	if err != nil {
		return model.Track{}, "", false
	}
	return t, path, true
}
