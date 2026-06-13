package store

import (
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/model"
)

// TestUpsertTrackKeysAlbumByAlbumArtist verifies a compilation collapses to one
// album keyed by its album-artist, while each track keeps its own track artist.
func TestUpsertTrackKeysAlbumByAlbumArtist(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	id1, _ := UpsertTrack(db, model.Track{Path: "/m/1.mp3", Title: "One"}, "Artist A", VariousArtists, "Comp")
	id2, _ := UpsertTrack(db, model.Track{Path: "/m/2.mp3", Title: "Two"}, "Artist B", VariousArtists, "Comp")

	var albums int
	db.QueryRow(`SELECT count(*) FROM album WHERE name = 'Comp'`).Scan(&albums)
	if albums != 1 {
		t.Fatalf("expected 1 album for the compilation, got %d", albums)
	}

	// The album's artist is the album-artist (Various Artists).
	var albumArtist string
	db.QueryRow(`SELECT ar.name FROM album a JOIN artist ar ON ar.id = a.artist_id
	             WHERE a.name = 'Comp'`).Scan(&albumArtist)
	if albumArtist != VariousArtists {
		t.Fatalf("expected album keyed by %q, got %q", VariousArtists, albumArtist)
	}

	// Each track keeps its own track artist.
	for _, tc := range []struct {
		id   int64
		want string
	}{{id1, "Artist A"}, {id2, "Artist B"}} {
		var name string
		db.QueryRow(`SELECT ar.name FROM track t JOIN artist ar ON ar.id = t.artist_id
		             WHERE t.id = ?`, tc.id).Scan(&name)
		if name != tc.want {
			t.Fatalf("track %d: expected artist %q, got %q", tc.id, tc.want, name)
		}
	}
}

// TestUpsertTrackEmptyAlbumArtistFallsBack verifies an empty album-artist keys
// the album by the track artist, preserving pre-#32 behavior.
func TestUpsertTrackEmptyAlbumArtistFallsBack(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	UpsertTrack(db, model.Track{Path: "/m/a.mp3", Title: "A"}, "Solo", "", "LP")

	var albumArtist string
	db.QueryRow(`SELECT ar.name FROM album a JOIN artist ar ON ar.id = a.artist_id
	             WHERE a.name = 'LP'`).Scan(&albumArtist)
	if albumArtist != "Solo" {
		t.Fatalf("expected album keyed by track artist %q, got %q", "Solo", albumArtist)
	}
}

// TestPruneOrphans removes albums with no tracks and artists backing neither a
// track nor an album, while leaving referenced rows intact.
func TestPruneOrphans(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()

	// A real track keeps its artist and album alive.
	UpsertTrack(db, model.Track{Path: "/m/a.mp3", Title: "A"}, "Keep Artist", "", "Keep Album")
	// An orphan album with a dedicated orphan artist, no tracks.
	db.Exec(`INSERT INTO artist(name) VALUES('Orphan Artist')`)
	db.Exec(`INSERT INTO album(name, artist_id) VALUES('Orphan Album',
	         (SELECT id FROM artist WHERE name='Orphan Artist'))`)

	if err := PruneOrphans(db); err != nil {
		t.Fatalf("PruneOrphans: %v", err)
	}

	var n int
	db.QueryRow(`SELECT count(*) FROM album WHERE name='Orphan Album'`).Scan(&n)
	if n != 0 {
		t.Fatalf("expected orphan album pruned, got %d", n)
	}
	db.QueryRow(`SELECT count(*) FROM artist WHERE name='Orphan Artist'`).Scan(&n)
	if n != 0 {
		t.Fatalf("expected orphan artist pruned, got %d", n)
	}
	db.QueryRow(`SELECT count(*) FROM album WHERE name='Keep Album'`).Scan(&n)
	if n != 1 {
		t.Fatalf("expected referenced album kept, got %d", n)
	}
	db.QueryRow(`SELECT count(*) FROM artist WHERE name='Keep Artist'`).Scan(&n)
	if n != 1 {
		t.Fatalf("expected referenced artist kept, got %d", n)
	}
}

// TestPruneKeepsAlbumArtistBackingOnlyAlbums verifies an album-artist that backs
// an album but no track (a compilation's "Various Artists") survives pruning.
func TestPruneKeepsAlbumArtistBackingOnlyAlbums(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	UpsertTrack(db, model.Track{Path: "/m/1.mp3", Title: "One"}, "Artist A", VariousArtists, "Comp")

	if err := PruneOrphans(db); err != nil {
		t.Fatalf("PruneOrphans: %v", err)
	}
	var n int
	db.QueryRow(`SELECT count(*) FROM artist WHERE name=?`, VariousArtists).Scan(&n)
	if n != 1 {
		t.Fatalf("expected Various Artists kept (backs the album), got %d", n)
	}
}

// TestReupsertReusesTrackIDPreservingQueue verifies a forced re-scan re-points a
// track without changing its id, so queue_item / history rows survive.
func TestReupsertReusesTrackIDPreservingQueue(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	id1, _ := UpsertTrack(db, model.Track{Path: "/m/1.mp3", Title: "One"}, "Artist A", "", "Album")
	if err := EnsureStream(db, "s", "", "private"); err != nil {
		t.Fatalf("EnsureStream: %v", err)
	}
	if err := Enqueue(db, "s", id1, ""); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Re-scan re-points the same path to an album-artist-keyed album.
	id2, _ := UpsertTrack(db, model.Track{Path: "/m/1.mp3", Title: "One"}, "Artist A", VariousArtists, "Album")
	if id1 != id2 {
		t.Fatalf("expected stable track id across re-scan, got %d then %d", id1, id2)
	}
	if err := PruneOrphans(db); err != nil {
		t.Fatalf("PruneOrphans: %v", err)
	}

	q, err := QueueTrackIDs(db, "s")
	if err != nil {
		t.Fatalf("QueueTrackIDs: %v", err)
	}
	if len(q) != 1 || q[0] != id1 {
		t.Fatalf("expected queue to still hold track %d, got %v", id1, q)
	}
}
