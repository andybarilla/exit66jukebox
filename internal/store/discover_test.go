package store

import (
	"database/sql"
	"slices"
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

// play records a play of track on stream at played_at.
func play(t *testing.T, db *sql.DB, streamID string, trackID int64, at int64) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO history(stream_id, track_id, played_at) VALUES(?,?,?)`,
		streamID, trackID, at); err != nil {
		t.Fatalf("history %s/%d: %v", streamID, trackID, err)
	}
}

// titles is the discovered order, which is what the ranking tests assert on.
func titles(got []model.Track) []string {
	out := make([]string, len(got))
	for i, tr := range got {
		out[i] = tr.Title
	}
	return out
}

func seedRankingFixture(t *testing.T, db *sql.DB) (alice, bob string) {
	t.Helper()
	if err := EnsureSharedStream(db, "house", "House"); err != nil {
		t.Fatalf("house: %v", err)
	}
	alice, bob = PersonalStreamID(1), PersonalStreamID(2)
	for _, id := range []string{alice, bob} {
		if err := EnsurePrivateStream(db, id); err != nil {
			t.Fatalf("private %s: %v", id, err)
		}
	}
	// Three never-played tracks at play_count 0: rediscover falls through to
	// last_played, so any play at all decides the order.
	seedTrack(t, db, "/m/a.mp3", "Alpha", "Rock", 0)
	seedTrack(t, db, "/m/b.mp3", "Bravo", "Rock", 0)
	seedTrack(t, db, "/m/c.mp3", "Charlie", "Rock", 0)
	return alice, bob
}

func rediscoverTitles(t *testing.T, db *sql.DB, personal string) []string {
	t.Helper()
	got, err := DiscoverTracks(db, DiscoverOpts{
		OrderBy: "rediscover", PersonalStream: personal, Limit: 10,
	})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	return titles(got)
}

func TestRediscoverIgnoresAnotherUsersPersonalStream(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	alice, bob := seedRankingFixture(t, db)
	var bravo int64
	db.QueryRow(`SELECT id FROM track WHERE title='Bravo'`).Scan(&bravo)
	play(t, db, bob, bravo, 9999)

	// Alice never heard it, so her ranking is the natural order.
	if got := rediscoverTitles(t, db, alice); !slices.Equal(got, []string{"Alpha", "Bravo", "Charlie"}) {
		t.Fatalf("alice order = %v, want natural order unaffected by bob's private play", got)
	}
	// Bob did, so it sinks for him.
	if got := rediscoverTitles(t, db, bob); !slices.Equal(got, []string{"Alpha", "Charlie", "Bravo"}) {
		t.Fatalf("bob order = %v, want his own play to demote Bravo", got)
	}
}

func TestRediscoverCountsSharedStreamPlaysForEveryone(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	alice, bob := seedRankingFixture(t, db)
	var bravo int64
	db.QueryRow(`SELECT id FROM track WHERE title='Bravo'`).Scan(&bravo)
	play(t, db, "house", bravo, 9999)

	for name, personal := range map[string]string{"alice": alice, "bob": bob, "no personal stream": ""} {
		if got := rediscoverTitles(t, db, personal); !slices.Equal(got, []string{"Alpha", "Charlie", "Bravo"}) {
			t.Fatalf("%s order = %v, want the house play to demote Bravo", name, got)
		}
	}
}

func TestRediscoverWithNoPersonalStreamSeesSharedStreamsOnly(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	_, bob := seedRankingFixture(t, db)
	var bravo, charlie int64
	db.QueryRow(`SELECT id FROM track WHERE title='Bravo'`).Scan(&bravo)
	db.QueryRow(`SELECT id FROM track WHERE title='Charlie'`).Scan(&charlie)
	play(t, db, bob, bravo, 9999)    // private: never counts
	play(t, db, "house", charlie, 5) // shared: always counts

	if got := rediscoverTitles(t, db, ""); !slices.Equal(got, []string{"Alpha", "Bravo", "Charlie"}) {
		t.Fatalf("order = %v, want only the house play to rank", got)
	}
}
