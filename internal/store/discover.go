package store

import (
	"database/sql"
	"fmt"

	"github.com/andybarilla/exit66jukebox/internal/model"
)

// DiscoverOpts parameterizes the discovery selection query.
type DiscoverOpts struct {
	Genre         string // "" = all genres
	OrderBy       string // "rediscover" | "recent" | "random"
	ExcludeStream string // "" = no exclusion; otherwise skip this stream's recent history
	Window        int    // size of the recent-history window for ExcludeStream
	Limit, Offset int
}

// GenreCount is a genre and how many tracks carry it.
type GenreCount struct {
	Genre string `json:"genre"`
	Count int    `json:"count"`
}

// DiscoverTracks ranks/filters tracks by play stats for the discovery surfaces.
// last_played is MAX(history.played_at) across all streams (0 = never played).
func DiscoverTracks(db *sql.DB, opts DiscoverOpts) ([]model.Track, error) {
	var order string
	switch opts.OrderBy {
	case "recent":
		order = "t.added_at DESC, t.id DESC"
	case "random":
		order = "RANDOM()"
	default: // "rediscover"
		order = "t.play_count ASC, last_played ASC, t.id ASC"
	}

	args := []any{}
	where := "WHERE 1=1"
	if opts.Genre != "" {
		where += " AND t.genre = ?"
		args = append(args, opts.Genre)
	}
	if opts.ExcludeStream != "" {
		where += ` AND t.id NOT IN (
			SELECT track_id FROM history WHERE stream_id = ?
			ORDER BY played_at DESC LIMIT ?
		)`
		args = append(args, opts.ExcludeStream, opts.Window)
	}

	lim := opts.Limit
	if lim <= 0 {
		lim = -1
	}
	args = append(args, lim, opts.Offset)

	q := fmt.Sprintf(`
		SELECT t.id, t.title, t.artist_id, t.album_id, t.track_no, t.genre,
		       t.duration, t.play_count,
		       coalesce((SELECT MAX(h.played_at) FROM history h WHERE h.track_id = t.id), 0) AS last_played
		FROM track t
		%s
		ORDER BY %s
		LIMIT ? OFFSET ?`, where, order)

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Track
	for rows.Next() {
		var t model.Track
		var lastPlayed int64 // selected only for ORDER BY; no model field
		if err := rows.Scan(&t.ID, &t.Title, &t.ArtistID, &t.AlbumID, &t.TrackNo,
			&t.Genre, &t.Duration, &t.PlayCount, &lastPlayed); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GenreCounts returns non-empty genres with their track counts, ordered by name.
func GenreCounts(db *sql.DB) ([]GenreCount, error) {
	rows, err := db.Query(
		`SELECT genre, count(*) FROM track WHERE genre <> '' GROUP BY genre ORDER BY genre`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GenreCount
	for rows.Next() {
		var g GenreCount
		if err := rows.Scan(&g.Genre, &g.Count); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
