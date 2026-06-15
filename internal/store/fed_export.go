package store

import (
	"database/sql"
	"strings"
)

// CatalogRow is one local track flattened with the artist/album names a peer
// needs to upsert it. RemoteID is this peer's local track id — the receiving
// peer stores it as remote_id.
type CatalogRow struct {
	RemoteID    int64    `json:"remote_id"`
	Title       string   `json:"title"`
	ArtistName  string   `json:"artist"`
	AlbumArtist string   `json:"album_artist"`
	AlbumName   string   `json:"album"`
	TrackNo     int      `json:"track_no"`
	Genre       string   `json:"genre"`
	Duration    int      `json:"duration"`
	Links       []string `json:"links,omitempty"`
}

// ExportCatalog returns all local (non-remote) tracks as flattened rows for
// catalog sync. Remote rows are excluded — a peer only shares its own files.
func ExportCatalog(db *sql.DB) ([]CatalogRow, error) {
	rows, err := db.Query(
		`SELECT t.id, t.title, ta.name, aa.name, al.name, t.track_no, t.genre, t.duration, t.links
		 FROM track t
		 JOIN artist ta ON ta.id = t.artist_id
		 JOIN album  al ON al.id = t.album_id
		 JOIN artist aa ON aa.id = al.artist_id
		 WHERE t.source_peer = ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CatalogRow
	for rows.Next() {
		var c CatalogRow
		var links string
		if err := rows.Scan(&c.RemoteID, &c.Title, &c.ArtistName, &c.AlbumArtist,
			&c.AlbumName, &c.TrackNo, &c.Genre, &c.Duration, &links); err != nil {
			return nil, err
		}
		if links != "" {
			c.Links = strings.Split(links, "\n")
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
