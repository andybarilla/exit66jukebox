package store

import (
	"database/sql"
	"strings"

	"github.com/andybarilla/exit66jukebox/internal/model"
)

// albumRankCTE assigns every album its global crate-wall rank (0-based position
// in sort_key order) plus its artist name and track count. Filters are applied
// to the CTE's output, so a row's rank stays global even under a search.
const albumRankCTE = `
WITH track_counts AS (
    SELECT album_id, count(*) AS c FROM track GROUP BY album_id
),
ranked_album AS (
    SELECT a.id AS album_id, a.name AS album_name, a.artist_id,
           ar.name AS artist_name, coalesce(tc.c, 0) AS track_count,
           ROW_NUMBER() OVER (ORDER BY a.sort_key, a.id) - 1 AS rank
    FROM album a
    JOIN artist ar ON ar.id = a.artist_id
    LEFT JOIN track_counts tc ON tc.album_id = a.id
)`

const trackSelectCols = `t.id, t.title, t.artist_id, t.album_id, t.track_no,
	t.genre, t.duration, t.play_count, r.rank, r.album_name, r.artist_name`

// scanEnrichedTrack reads a track row joined to ranked_album and fills in the
// derived code/tone.
func scanEnrichedTrack(rows *sql.Rows) (model.EnrichedTrack, error) {
	var t model.EnrichedTrack
	var rank int
	if err := rows.Scan(&t.ID, &t.Title, &t.ArtistID, &t.AlbumID, &t.TrackNo,
		&t.Genre, &t.Duration, &t.PlayCount, &rank, &t.AlbumName, &t.ArtistName); err != nil {
		return model.EnrichedTrack{}, err
	}
	t.Code = slotCode(rank, t.TrackNo)
	t.Tone = tone(rank)
	return t, nil
}

func pageLimit(limit int) int {
	if limit <= 0 {
		return -1
	}
	return limit
}

// ListAlbumsEnriched returns albums matching search (name or artist name), each
// carrying its global crate-wall letter/tone, artist name and track count,
// ordered by global rank, paged by limit/offset.
func ListAlbumsEnriched(db *sql.DB, search string, limit, offset int) ([]model.EnrichedAlbum, error) {
	like := "%" + search + "%"
	rows, err := db.Query(albumRankCTE+`
		SELECT album_id, album_name, artist_id, artist_name, track_count, rank
		FROM ranked_album
		WHERE album_name LIKE ? OR artist_name LIKE ?
		ORDER BY rank LIMIT ? OFFSET ?`, like, like, pageLimit(limit), offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.EnrichedAlbum
	for rows.Next() {
		var a model.EnrichedAlbum
		var rank int
		if err := rows.Scan(&a.ID, &a.Name, &a.ArtistID, &a.ArtistName, &a.TrackCount, &rank); err != nil {
			return nil, err
		}
		a.Letter = slotLetter(rank)
		a.Tone = tone(rank)
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListTracksEnriched returns tracks ordered by their album's global rank then
// track number, so slot codes stay contiguous. Search matches title, artist
// name or album name; a bare slot code (e.g. "A3") resolves to that track.
func ListTracksEnriched(db *sql.DB, search string, limit, offset int) ([]model.EnrichedTrack, error) {
	where, args := trackFilter(search)
	args = append(args, pageLimit(limit), offset)
	rows, err := db.Query(albumRankCTE+`
		SELECT `+trackSelectCols+`
		FROM track t JOIN ranked_album r ON r.album_id = t.album_id
		`+where+`
		ORDER BY r.rank, t.track_no, t.title
		LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.EnrichedTrack
	for rows.Next() {
		t, err := scanEnrichedTrack(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// trackFilter builds the WHERE clause + args for a track search, branching on a
// bare slot code.
func trackFilter(search string) (string, []any) {
	if rank, trackNo, ok := parseCode(search); ok {
		return "WHERE r.rank = ? AND t.track_no = ?", []any{rank, trackNo}
	}
	like := "%" + search + "%"
	return "WHERE t.title LIKE ? OR r.artist_name LIKE ? OR r.album_name LIKE ?",
		[]any{like, like, like}
}

// TracksByAlbumEnriched returns one album's tracks (enriched) in track order.
func TracksByAlbumEnriched(db *sql.DB, albumID int64) ([]model.EnrichedTrack, error) {
	rows, err := db.Query(albumRankCTE+`
		SELECT `+trackSelectCols+`
		FROM track t JOIN ranked_album r ON r.album_id = t.album_id
		WHERE t.album_id = ?
		ORDER BY t.track_no, t.title`, albumID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.EnrichedTrack
	for rows.Next() {
		t, err := scanEnrichedTrack(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// EnrichTracks attaches code/tone/album_name/artist_name to already-fetched
// tracks (queue, now-playing, discover) using the same global album rank as the
// browse queries, so a track's code is identical wherever it is serialized.
func EnrichTracks(db *sql.DB, tracks []model.Track) ([]model.EnrichedTrack, error) {
	out := make([]model.EnrichedTrack, len(tracks))
	if len(tracks) == 0 {
		return out, nil
	}
	ranks, err := albumRanks(db, distinctAlbumIDs(tracks))
	if err != nil {
		return nil, err
	}
	for i, t := range tracks {
		e := model.EnrichedTrack{Track: t}
		if info, ok := ranks[t.AlbumID]; ok {
			e.Code = slotCode(info.rank, t.TrackNo)
			e.Tone = tone(info.rank)
			e.AlbumName = info.albumName
			e.ArtistName = info.artistName
		} else {
			e.Code = "··"
			e.Tone = tones[1]
			e.ArtistName = "Unknown"
		}
		out[i] = e
	}
	return out, nil
}

type rankInfo struct {
	rank       int
	albumName  string
	artistName string
}

func distinctAlbumIDs(tracks []model.Track) []int64 {
	seen := make(map[int64]bool)
	var ids []int64
	for _, t := range tracks {
		if !seen[t.AlbumID] {
			seen[t.AlbumID] = true
			ids = append(ids, t.AlbumID)
		}
	}
	return ids
}

// albumRanks returns each album's global rank (same sort_key, id ordering the
// window query uses) plus its names, for a set of album ids.
func albumRanks(db *sql.DB, albumIDs []int64) (map[int64]rankInfo, error) {
	out := make(map[int64]rankInfo, len(albumIDs))
	if len(albumIDs) == 0 {
		return out, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(albumIDs)), ",")
	args := make([]any, len(albumIDs))
	for i, id := range albumIDs {
		args[i] = id
	}
	rows, err := db.Query(`
		SELECT a.id, a.name, ar.name,
		       (SELECT count(*) FROM album b
		        WHERE b.sort_key < a.sort_key OR (b.sort_key = a.sort_key AND b.id < a.id)) AS rank
		FROM album a JOIN artist ar ON ar.id = a.artist_id
		WHERE a.id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var info rankInfo
		if err := rows.Scan(&id, &info.albumName, &info.artistName, &info.rank); err != nil {
			return nil, err
		}
		out[id] = info
	}
	return out, rows.Err()
}

// ListArtistsEnriched returns artists matching search, ordered by sort_key, each
// with its album and track counts.
func ListArtistsEnriched(db *sql.DB, search string, limit, offset int) ([]model.EnrichedArtist, error) {
	rows, err := db.Query(`
		SELECT ar.id, ar.name,
		       (SELECT count(*) FROM album al WHERE al.artist_id = ar.id),
		       (SELECT count(*) FROM track t WHERE t.artist_id = ar.id)
		FROM artist ar
		WHERE ar.name LIKE ?
		ORDER BY ar.sort_key, ar.id
		LIMIT ? OFFSET ?`, "%"+search+"%", pageLimit(limit), offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.EnrichedArtist
	for rows.Next() {
		var a model.EnrichedArtist
		if err := rows.Scan(&a.ID, &a.Name, &a.AlbumCount, &a.TrackCount); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CountAlbums counts albums matching search (name or artist name).
func CountAlbums(db *sql.DB, search string) (int, error) {
	like := "%" + search + "%"
	var n int
	err := db.QueryRow(`
		SELECT count(*) FROM album a JOIN artist ar ON ar.id = a.artist_id
		WHERE a.name LIKE ? OR ar.name LIKE ?`, like, like).Scan(&n)
	return n, err
}

// CountTracks counts tracks matching search (title/artist/album, or a slot code).
func CountTracks(db *sql.DB, search string) (int, error) {
	var n int
	if rank, trackNo, ok := parseCode(search); ok {
		err := db.QueryRow(albumRankCTE+`
			SELECT count(*) FROM track t JOIN ranked_album r ON r.album_id = t.album_id
			WHERE r.rank = ? AND t.track_no = ?`, rank, trackNo).Scan(&n)
		return n, err
	}
	like := "%" + search + "%"
	err := db.QueryRow(`
		SELECT count(*) FROM track t
		JOIN album a ON a.id = t.album_id
		JOIN artist ar ON ar.id = a.artist_id
		WHERE t.title LIKE ? OR ar.name LIKE ? OR a.name LIKE ?`, like, like, like).Scan(&n)
	return n, err
}

// CountArtists counts artists matching search.
func CountArtists(db *sql.DB, search string) (int, error) {
	var n int
	err := db.QueryRow(`SELECT count(*) FROM artist WHERE name LIKE ?`, "%"+search+"%").Scan(&n)
	return n, err
}
