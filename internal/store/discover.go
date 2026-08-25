package store

import (
	"database/sql"
	"fmt"

	"github.com/andybarilla/exit66jukebox/internal/model"
)

// DiscoverOpts parameterizes the discovery selection query.
type DiscoverOpts struct {
	Genre          string // "" = all genres
	OrderBy        string // "rediscover" | "recent" | "random"
	ExcludeStream  string // "" = no exclusion; otherwise skip this stream's recent history
	Window         int    // size of the recent-history window for ExcludeStream
	PersonalStream string // caller's own personal stream id; "" = none. See DiscoverTracks.
	Limit, Offset  int
}

// GenreCount is a genre and how many tracks carry it.
type GenreCount struct {
	Genre string `json:"genre"`
	Count int    `json:"count"`
}

// callerPlaysJoin aggregates history over the streams the caller can be said to
// have heard — every shared stream, plus opts.PersonalStream when they have one.
// Both rediscover keys read this one grouped scan, so the count and the recency
// key cannot disagree about which streams count (#166). Takes two args: the
// shared kind and the caller's personal stream id.
const callerPlaysJoin = `
		LEFT JOIN (SELECT h.track_id,
		                  COUNT(*)          AS plays,
		                  MAX(h.played_at)  AS last_played
		             FROM history h
		             JOIN stream s ON s.id = h.stream_id
		            WHERE s.kind = ? OR s.id = ?
		            GROUP BY h.track_id) hp ON hp.track_id = t.id`

// DiscoverTracks ranks/filters tracks by play stats for the discovery surfaces.
//
// The rediscover order ranks on how many times the caller has heard a track,
// then on how long ago, both measured over the streams the caller can be said
// to have heard: every shared stream, plus opts.PersonalStream when they have
// one (a track they never heard counts 0 plays at time 0). Another user's
// personal stream never counts, so one listener's private plays cannot shape
// anyone else's rediscover ranking (#151, #166). A play on a stream whose row
// has since been deleted counts for nobody.
//
// The track's stored play_count is still selected and returned unchanged; it is
// the household-wide number every display site shows, and no order reads it.
func DiscoverTracks(db *sql.DB, opts DiscoverOpts) ([]model.Track, error) {
	var order, join string
	var args []any
	switch opts.OrderBy {
	case "recent":
		order = "t.added_at DESC, t.id DESC"
	case "random":
		order = "RANDOM()"
	default: // "rediscover"
		order = "coalesce(hp.plays, 0) ASC, coalesce(hp.last_played, 0) ASC, t.id ASC"
		// Only rediscover reads the aggregate; the other orders would pay for a
		// scan of history they discard.
		join = callerPlaysJoin
		args = append(args, KindShared, opts.PersonalStream)
	}

	// SQLite binds ? by position, so args follow the clauses in textual order:
	// the join's, then the genre's, then the exclusion's, then the limit's.
	where := "WHERE " + visibleTrackPredicate
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
		       t.duration, t.play_count
		FROM track t%s
		%s
		ORDER BY %s
		LIMIT ? OFFSET ?`, join, where, order)

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Track
	for rows.Next() {
		var t model.Track
		if err := rows.Scan(&t.ID, &t.Title, &t.ArtistID, &t.AlbumID, &t.TrackNo,
			&t.Genre, &t.Duration, &t.PlayCount); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GenreCounts returns non-empty genres with their track counts, ordered by name.
func GenreCounts(db *sql.DB) ([]GenreCount, error) {
	rows, err := db.Query(
		`SELECT t.genre, count(*) FROM track t WHERE t.genre <> '' AND ` + visibleTrackPredicate + ` GROUP BY t.genre ORDER BY t.genre`)
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
