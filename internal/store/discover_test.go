package store

import (
	"database/sql"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/model"
)

func seedTrack(t *testing.T, db *sql.DB, path, title, genre string, playCount int) int64 {
	t.Helper()
	id, err := UpsertTrack(db, model.Track{Path: path, Title: title, Genre: genre}, "Band", "", "Album")
	if err != nil {
		t.Fatalf("seed %s: %v", path, err)
	}
	if _, err := db.Exec(`UPDATE track SET play_count=? WHERE id=?`, playCount, id); err != nil {
		t.Fatalf("set play_count: %v", err)
	}
	return id
}

func TestDiscoverRediscoverOrdersByPlayCount(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	low := seedTrack(t, db, "/m/low.mp3", "Low", "Rock", 0)
	seedTrack(t, db, "/m/high.mp3", "High", "Rock", 50)

	got, err := DiscoverTracks(db, DiscoverOpts{OrderBy: "rediscover", Limit: 10})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(got) != 2 || got[0].ID != low {
		t.Fatalf("expected least-played track first, got %+v", got)
	}
}

func TestDiscoverRecentOrdersByAddedAt(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	seedTrack(t, db, "/m/old.mp3", "Old", "Rock", 0)
	newID := seedTrack(t, db, "/m/new.mp3", "New", "Rock", 0)
	// Force ordering: make old older, new newer.
	db.Exec(`UPDATE track SET added_at=100 WHERE path='/m/old.mp3'`)
	db.Exec(`UPDATE track SET added_at=200 WHERE id=?`, newID)

	got, err := DiscoverTracks(db, DiscoverOpts{OrderBy: "recent", Limit: 10})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(got) != 2 || got[0].ID != newID {
		t.Fatalf("expected newest first, got %+v", got)
	}
}

func TestDiscoverGenreFilter(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	rock := seedTrack(t, db, "/m/r.mp3", "R", "Rock", 0)
	seedTrack(t, db, "/m/j.mp3", "J", "Jazz", 0)

	got, err := DiscoverTracks(db, DiscoverOpts{OrderBy: "rediscover", Genre: "Rock", Limit: 10})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(got) != 1 || got[0].ID != rock {
		t.Fatalf("expected only the Rock track, got %+v", got)
	}
}

func TestDiscoverExcludeStreamSkipsRecentHistory(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	played := seedTrack(t, db, "/m/p.mp3", "P", "Rock", 0)
	fresh := seedTrack(t, db, "/m/f.mp3", "F", "Rock", 0)
	// Mark `played` as recently played on stream "s".
	db.Exec(`INSERT INTO history(stream_id, track_id, played_at) VALUES('s', ?, 999)`, played)

	got, err := DiscoverTracks(db, DiscoverOpts{
		OrderBy: "random", Genre: "Rock", ExcludeStream: "s", Window: 5, Limit: 10,
	})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(got) != 1 || got[0].ID != fresh {
		t.Fatalf("expected recently-played track excluded, got %+v", got)
	}
}

func TestGenreCounts(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	seedTrack(t, db, "/m/r1.mp3", "R1", "Rock", 0)
	seedTrack(t, db, "/m/r2.mp3", "R2", "Rock", 0)
	seedTrack(t, db, "/m/j1.mp3", "J1", "Jazz", 0)
	seedTrack(t, db, "/m/blank.mp3", "B", "", 0)

	got, err := GenreCounts(db)
	if err != nil {
		t.Fatalf("genres: %v", err)
	}
	// Empty-genre tracks are excluded; Rock=2, Jazz=1, ordered by name.
	if len(got) != 2 || got[0].Genre != "Jazz" || got[0].Count != 1 ||
		got[1].Genre != "Rock" || got[1].Count != 2 {
		t.Fatalf("unexpected genre counts: %+v", got)
	}
}

func TestDiscoverAndGenreCountsHideDisabledLocalLibrary(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()

	enabledLibraryID, err := EnsureLocalLibrary(db, "/enabled", "Enabled")
	if err != nil {
		t.Fatalf("enabled library: %v", err)
	}
	disabledLibraryID, err := EnsureLocalLibrary(db, "/disabled", "Disabled")
	if err != nil {
		t.Fatalf("disabled library: %v", err)
	}
	visibleID, err := UpsertTrackInLibrary(db, enabledLibraryID, model.Track{Path: "/enabled/a.mp3", Title: "Visible", Genre: "Jazz"}, "Band", "", "Album")
	if err != nil {
		t.Fatalf("visible track: %v", err)
	}
	if _, err := UpsertTrackInLibrary(db, disabledLibraryID, model.Track{Path: "/disabled/a.mp3", Title: "Hidden", Genre: "Rock"}, "Band", "", "Album"); err != nil {
		t.Fatalf("hidden track: %v", err)
	}
	if _, err := db.Exec(`UPDATE local_library SET enabled = 0 WHERE id = ?`, disabledLibraryID); err != nil {
		t.Fatalf("disable library: %v", err)
	}

	got, err := DiscoverTracks(db, DiscoverOpts{OrderBy: "recent", Limit: 10})
	if err != nil {
		t.Fatalf("DiscoverTracks: %v", err)
	}
	if len(got) != 1 || got[0].ID != visibleID {
		t.Fatalf("discover = %+v, want only enabled-library track", got)
	}

	genres, err := GenreCounts(db)
	if err != nil {
		t.Fatalf("GenreCounts: %v", err)
	}
	if len(genres) != 1 || genres[0].Genre != "Jazz" || genres[0].Count != 1 {
		t.Fatalf("genres = %+v, want only Jazz=1", genres)
	}
}

func TestUpsertStampsAddedAt(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()

	id, err := UpsertTrack(db, model.Track{Path: "/m/a.mp3", Title: "A"}, "Band", "", "Album")
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	var addedAt int64
	if err := db.QueryRow(`SELECT added_at FROM track WHERE id=?`, id).Scan(&addedAt); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if addedAt <= 0 {
		t.Fatalf("expected added_at to be stamped on insert, got %d", addedAt)
	}
}
