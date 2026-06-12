package store

import (
	"database/sql"
	"path/filepath"
)

// Placeholder tag values the scanner writes for blank tags; enrichment may
// overwrite these but never a real tag. Track titles fall back to the
// filename, handled separately.
const (
	placeholderArtist = "Unknown Artist"
	placeholderAlbum  = "Unknown Album"
)

// EnrichTarget is a track lacking an MBID plus the current names enrichment
// needs to decide placeholder replacement without re-querying.
type EnrichTarget struct {
	TrackID  int64
	Title    string
	Genre    string
	Path     string
	ArtistID int64
	Artist   string
	AlbumID  int64
	Album    string
}

// TracksNeedingEnrichment returns every track with no MBID, joined to its
// artist and album. The pass is resumable: matched tracks (mbid != ”) are
// skipped on the next run.
func TracksNeedingEnrichment(db *sql.DB) ([]EnrichTarget, error) {
	rows, err := db.Query(
		`SELECT t.id, t.title, t.genre, t.path, ar.id, ar.name, al.id, al.name
		 FROM track t
		 JOIN artist ar ON ar.id = t.artist_id
		 JOIN album  al ON al.id = t.album_id
		 WHERE t.mbid = ''
		 ORDER BY t.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EnrichTarget
	for rows.Next() {
		var e EnrichTarget
		if err := rows.Scan(&e.TrackID, &e.Title, &e.Genre, &e.Path,
			&e.ArtistID, &e.Artist, &e.AlbumID, &e.Album); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Enrichment is one matched recording to apply: MBIDs plus the canonical names
// from MusicBrainz. Names only replace placeholders; MBIDs only fill blanks.
type Enrichment struct {
	TrackID  int64
	ArtistID int64
	AlbumID  int64
	Path     string // track file path, for filename-placeholder title comparison

	RecordingMBID string
	ArtistMBID    string
	ReleaseMBID   string

	NewTitle  string
	NewArtist string
	NewAlbum  string
}

// ApplyEnrichment records the match in one transaction. It always sets
// track.mbid; fills artist/album mbid only when blank; and replaces a name
// only when the current value is a placeholder and the rename won't collide
// with an existing unique name (on collision it keeps the MBID and skips the
// rename). Idempotent: a second apply for the same row changes nothing.
func ApplyEnrichment(db *sql.DB, e Enrichment) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE track SET mbid = ? WHERE id = ?`, e.RecordingMBID, e.TrackID); err != nil {
		return err
	}
	if e.ArtistMBID != "" {
		if _, err := tx.Exec(
			`UPDATE artist SET mbid = ? WHERE id = ? AND mbid = ''`, e.ArtistMBID, e.ArtistID); err != nil {
			return err
		}
	}
	if e.ReleaseMBID != "" {
		if _, err := tx.Exec(
			`UPDATE album SET mbid = ? WHERE id = ? AND mbid = ''`, e.ReleaseMBID, e.AlbumID); err != nil {
			return err
		}
	}

	// Artist rename: only if currently the placeholder and no name collision
	// (artist.name is UNIQUE).
	if e.NewArtist != "" {
		var cur string
		if err := tx.QueryRow(`SELECT name FROM artist WHERE id = ?`, e.ArtistID).Scan(&cur); err != nil {
			return err
		}
		if cur == placeholderArtist {
			var clash int
			if err := tx.QueryRow(
				`SELECT count(*) FROM artist WHERE name = ? AND id != ?`, e.NewArtist, e.ArtistID).Scan(&clash); err != nil {
				return err
			}
			if clash == 0 {
				if _, err := tx.Exec(`UPDATE artist SET name = ? WHERE id = ?`, e.NewArtist, e.ArtistID); err != nil {
					return err
				}
			}
		}
	}

	// Album rename: only if placeholder and no collision (UNIQUE(name, artist_id)).
	if e.NewAlbum != "" {
		var cur string
		if err := tx.QueryRow(`SELECT name FROM album WHERE id = ?`, e.AlbumID).Scan(&cur); err != nil {
			return err
		}
		if cur == placeholderAlbum {
			var clash int
			if err := tx.QueryRow(
				`SELECT count(*) FROM album WHERE name = ? AND artist_id = ? AND id != ?`,
				e.NewAlbum, e.ArtistID, e.AlbumID).Scan(&clash); err != nil {
				return err
			}
			if clash == 0 {
				if _, err := tx.Exec(`UPDATE album SET name = ? WHERE id = ?`, e.NewAlbum, e.AlbumID); err != nil {
					return err
				}
			}
		}
	}

	// Title: replace only when blank or still the filename placeholder.
	if e.NewTitle != "" {
		var cur string
		if err := tx.QueryRow(`SELECT title FROM track WHERE id = ?`, e.TrackID).Scan(&cur); err != nil {
			return err
		}
		if cur == "" || cur == filepath.Base(e.Path) {
			if _, err := tx.Exec(`UPDATE track SET title = ? WHERE id = ?`, e.NewTitle, e.TrackID); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// SetAlbumCover records a cached cover-image path on an album.
func SetAlbumCover(db *sql.DB, albumID int64, path string) error {
	_, err := db.Exec(`UPDATE album SET cover = ? WHERE id = ?`, path, albumID)
	return err
}

// AlbumCoverByTrack returns the cover path for a track's album. ok is false
// when the album has no cover recorded.
func AlbumCoverByTrack(db *sql.DB, trackID int64) (string, bool) {
	var cover string
	err := db.QueryRow(
		`SELECT al.cover FROM track t JOIN album al ON al.id = t.album_id WHERE t.id = ?`,
		trackID).Scan(&cover)
	if err != nil || cover == "" {
		return "", false
	}
	return cover, true
}
