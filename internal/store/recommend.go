package store

import (
	"database/sql"
	"strings"

	"github.com/andybarilla/exit66jukebox/internal/model"
)

// trackColumns is the SELECT list matching scanTrack's Scan order.
const trackColumns = `t.id, t.title, t.artist_id, t.album_id, t.track_no, t.genre, t.duration, t.play_count`

func scanTrack(rows *sql.Rows) (model.Track, error) {
	var t model.Track
	err := rows.Scan(&t.ID, &t.Title, &t.ArtistID, &t.AlbumID,
		&t.TrackNo, &t.Genre, &t.Duration, &t.PlayCount)
	return t, err
}

// TracksByRecordingMBIDs maps a list of recording MBIDs (ListenBrainz
// recommendations, in descending-score order) to local tracks via track.mbid,
// preserving the input order and dropping MBIDs with no local match. The
// response carries MBIDs only, so there is no name fallback here.
func TracksByRecordingMBIDs(db *sql.DB, mbids []string) ([]model.Track, error) {
	if len(mbids) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(mbids)), ",")
	args := make([]any, len(mbids))
	for i, m := range mbids {
		args[i] = m
	}
	rows, err := db.Query(
		`SELECT `+trackColumns+`, t.mbid FROM track t WHERE t.mbid IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byMBID := make(map[string]model.Track)
	for rows.Next() {
		var t model.Track
		var mbid string
		if err := rows.Scan(&t.ID, &t.Title, &t.ArtistID, &t.AlbumID,
			&t.TrackNo, &t.Genre, &t.Duration, &t.PlayCount, &mbid); err != nil {
			return nil, err
		}
		byMBID[mbid] = t
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []model.Track
	for _, m := range mbids {
		if t, ok := byMBID[m]; ok {
			out = append(out, t)
		}
	}
	return out, nil
}

// TopArtists returns the most-played local artists (by summed track play_count,
// descending), capped at limit. Artists with zero plays are excluded — they
// make poor similarity seeds. Each carries its id, name, and mbid.
func TopArtists(db *sql.DB, limit int) ([]model.Artist, error) {
	rows, err := db.Query(`
		SELECT ar.id, ar.name, ar.mbid
		FROM artist ar
		JOIN track t ON t.artist_id = ar.id
		GROUP BY ar.id
		HAVING SUM(t.play_count) > 0
		ORDER BY SUM(t.play_count) DESC, ar.id ASC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Artist
	for rows.Next() {
		var a model.Artist
		if err := rows.Scan(&a.ID, &a.Name, &a.Mbid); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// TracksBySimilarArtists maps Last.fm similar-artist hits to local tracks. An
// artist matches by mbid (preferred) or case-insensitive name; each matched
// artist contributes up to perArtist of its tracks (lowest play_count first, to
// surface the less-heard). names should already be lowercased by the caller.
func TracksBySimilarArtists(db *sql.DB, names, mbids []string, perArtist int) ([]model.Track, error) {
	if len(names) == 0 && len(mbids) == 0 {
		return nil, nil
	}

	var clauses []string
	var args []any
	if len(mbids) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(mbids)), ",")
		clauses = append(clauses, "ar.mbid <> '' AND ar.mbid IN ("+ph+")")
		for _, m := range mbids {
			args = append(args, m)
		}
	}
	if len(names) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(names)), ",")
		clauses = append(clauses, "LOWER(ar.name) IN ("+ph+")")
		for _, n := range names {
			args = append(args, n)
		}
	}
	args = append(args, perArtist)

	// Per-artist cap via a window ROW_NUMBER, ordered by play_count then id.
	rows, err := db.Query(`
		SELECT id, title, artist_id, album_id, track_no, genre, duration, play_count FROM (
			SELECT t.id, t.title, t.artist_id, t.album_id, t.track_no, t.genre,
			       t.duration, t.play_count,
			       ROW_NUMBER() OVER (PARTITION BY t.artist_id
			                          ORDER BY t.play_count ASC, t.id ASC) AS rn
			FROM track t
			JOIN artist ar ON ar.id = t.artist_id
			WHERE `+strings.Join(clauses, " OR ")+`
		) WHERE rn <= ?
		ORDER BY artist_id ASC, play_count ASC, id ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Track
	for rows.Next() {
		t, err := scanTrack(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
