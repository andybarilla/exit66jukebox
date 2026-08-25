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

func TestDiscoverRediscoverOrdersByCallerPlayCount(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	if err := EnsureSharedStream(db, "house", "House"); err != nil {
		t.Fatalf("house: %v", err)
	}
	// high is seeded first so id order fights the assertion: only the derived
	// count can put low in front.
	high := seedTrack(t, db, "/m/high.mp3", "High", "Rock", 0)
	low := seedTrack(t, db, "/m/low.mp3", "Low", "Rock", 0)
	for i := range 3 {
		seedPlay(t, db, "house", high, int64(100+i))
	}
	seedPlay(t, db, "house", low, 200)

	got, err := DiscoverTracks(db, DiscoverOpts{OrderBy: "rediscover", Limit: 10})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(got) != 2 || got[0].ID != low {
		t.Fatalf("expected least-played track first, got %+v", got)
	}
	// The stored household counter is still served untouched by the ranking.
	if got[0].PlayCount != 0 || got[1].PlayCount != 0 {
		t.Fatalf("stored play_count should be unchanged, got %d and %d", got[0].PlayCount, got[1].PlayCount)
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

// seedPlay records a play of track on stream at played_at.
func seedPlay(t *testing.T, db *sql.DB, streamID string, trackID int64, at int64) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO history(stream_id, track_id, played_at) VALUES(?,?,?)`,
		streamID, trackID, at); err != nil {
		t.Fatalf("history %s/%d: %v", streamID, trackID, err)
	}
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
	// Three never-played tracks: every rediscover key is derived from history,
	// so any play at all decides the order.
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
	// The titles are the ranking, which is what these tests assert on.
	out := make([]string, len(got))
	for i, tr := range got {
		out[i] = tr.Title
	}
	return out
}

func TestRediscoverIgnoresAnotherUsersPersonalStream(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	alice, bob := seedRankingFixture(t, db)
	var bravo int64
	db.QueryRow(`SELECT id FROM track WHERE title='Bravo'`).Scan(&bravo)
	seedPlay(t, db, bob, bravo, 9999)

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
	seedPlay(t, db, "house", bravo, 9999)

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
	seedPlay(t, db, bob, bravo, 9999)    // private: never counts
	seedPlay(t, db, "house", charlie, 5) // shared: always counts

	if got := rediscoverTitles(t, db, ""); !slices.Equal(got, []string{"Alpha", "Bravo", "Charlie"}) {
		t.Fatalf("order = %v, want only the house play to rank", got)
	}
}

// History rows outlive the stream they name: deleteStreamTx drops the stream,
// queue and station rows and history has no foreign key. The ranking is
// defined over stream rows that exist, so an orphaned play counts for nobody —
// including the plays of a shared stream an admin has since deleted, which is
// the price of stating the rule as "shared, or mine".
func TestRediscoverIgnoresHistoryWhoseStreamIsGone(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	alice, _ := seedRankingFixture(t, db)
	var bravo int64
	db.QueryRow(`SELECT id FROM track WHERE title='Bravo'`).Scan(&bravo)
	if err := EnsureSharedStream(db, "kitchen", "Kitchen"); err != nil {
		t.Fatalf("kitchen: %v", err)
	}
	seedPlay(t, db, "kitchen", bravo, 9999)
	if got := rediscoverTitles(t, db, alice); !slices.Equal(got, []string{"Alpha", "Charlie", "Bravo"}) {
		t.Fatalf("order = %v, want the kitchen play to demote Bravo while the stream exists", got)
	}

	if err := DeleteStream(db, "kitchen"); err != nil {
		t.Fatalf("delete kitchen: %v", err)
	}
	if got := rediscoverTitles(t, db, alice); !slices.Equal(got, []string{"Alpha", "Bravo", "Charlie"}) {
		t.Fatalf("order = %v, want the orphaned play to stop counting", got)
	}
}

func TestRediscoverCountIgnoresAnotherUsersPersonalStream(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	alice, bob := seedRankingFixture(t, db)
	var alpha, bravo int64
	db.QueryRow(`SELECT id FROM track WHERE title='Alpha'`).Scan(&alpha)
	db.QueryRow(`SELECT id FROM track WHERE title='Bravo'`).Scan(&bravo)
	// Bob hammers Bravo privately and late; the house heard Alpha once, early.
	seedPlay(t, db, bob, bravo, 9000)
	seedPlay(t, db, bob, bravo, 9999)
	seedPlay(t, db, "house", alpha, 100)

	// For alice Bravo is unheard, so it leads on 0 plays and Alpha sinks on 1.
	// A global count would rank Bravo (2 plays) last instead.
	if got := rediscoverTitles(t, db, alice); !slices.Equal(got, []string{"Bravo", "Charlie", "Alpha"}) {
		t.Fatalf("alice order = %v, want bob's private plays not to count against her", got)
	}
	// Bob heard Bravo twice, so for him it sinks below Alpha's single house play.
	if got := rediscoverTitles(t, db, bob); !slices.Equal(got, []string{"Charlie", "Alpha", "Bravo"}) {
		t.Fatalf("bob order = %v, want his own two plays to sink Bravo", got)
	}
}

// The play count and the recency key are read off one grouped scan of history,
// so they cannot disagree about which streams the caller can be said to have
// heard. Each stream class is asserted twice from that one rule: once where
// only the count can decide the order (the two played tracks share a
// timestamp), and once where only recency can (they share a count). Scoping
// one key differently from the other fails at least one of these.
func TestRediscoverCountAndRecencyShareOneStreamRule(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stream func(alice, bob string) string
		orphan bool // delete the stream after seeding its plays
		counts bool // whether the caller can be said to have heard it
	}{
		{name: "shared stream", stream: func(_, _ string) string { return "house" }, counts: true},
		{name: "caller's own personal stream", stream: func(a, _ string) string { return a }, counts: true},
		{name: "another user's personal stream", stream: func(_, b string) string { return b }, counts: false},
		{name: "a stream since deleted", stream: func(_, _ string) string { return "kitchen" }, orphan: true, counts: false},
	} {
		// Only the count separates Alpha from Bravo: both were last heard at 100.
		t.Run(tc.name+"/count axis", func(t *testing.T) {
			db, alice, under := seedShareRuleFixture(t, tc.stream)
			defer db.Close()
			var alpha, bravo int64
			db.QueryRow(`SELECT id FROM track WHERE title='Alpha'`).Scan(&alpha)
			db.QueryRow(`SELECT id FROM track WHERE title='Bravo'`).Scan(&bravo)
			seedPlay(t, db, "house", alpha, 100)
			seedPlay(t, db, under, bravo, 100)
			seedPlay(t, db, under, bravo, 100)
			seedPlay(t, db, under, bravo, 100)
			dropIfOrphan(t, db, under, tc.orphan)

			want := []string{"Bravo", "Charlie", "Alpha"} // Bravo unheard: 0 plays
			if tc.counts {
				want = []string{"Charlie", "Alpha", "Bravo"} // 0, 1, 3 plays
			}
			if got := rediscoverTitles(t, db, alice); !slices.Equal(got, want) {
				t.Fatalf("count axis = %v, want %v", got, want)
			}
		})
		// Only recency separates them: both were heard exactly once.
		t.Run(tc.name+"/recency axis", func(t *testing.T) {
			db, alice, under := seedShareRuleFixture(t, tc.stream)
			defer db.Close()
			var alpha, bravo int64
			db.QueryRow(`SELECT id FROM track WHERE title='Alpha'`).Scan(&alpha)
			db.QueryRow(`SELECT id FROM track WHERE title='Bravo'`).Scan(&bravo)
			seedPlay(t, db, "house", alpha, 200)
			seedPlay(t, db, under, bravo, 100)
			dropIfOrphan(t, db, under, tc.orphan)

			want := []string{"Bravo", "Charlie", "Alpha"} // Bravo unheard: 0 plays
			if tc.counts {
				want = []string{"Charlie", "Bravo", "Alpha"} // 1 play each, Bravo longer ago
			}
			if got := rediscoverTitles(t, db, alice); !slices.Equal(got, want) {
				t.Fatalf("recency axis = %v, want %v", got, want)
			}
		})
	}
}

// seedShareRuleFixture builds the three-track ranking fixture plus a "kitchen"
// shared stream, and returns alice's view and the stream under test.
func seedShareRuleFixture(t *testing.T, pick func(alice, bob string) string) (*sql.DB, string, string) {
	t.Helper()
	db, _ := Open(":memory:")
	alice, bob := seedRankingFixture(t, db)
	if err := EnsureSharedStream(db, "kitchen", "Kitchen"); err != nil {
		t.Fatalf("kitchen: %v", err)
	}
	return db, alice, pick(alice, bob)
}

func dropIfOrphan(t *testing.T, db *sql.DB, streamID string, orphan bool) {
	t.Helper()
	if !orphan {
		return
	}
	if err := DeleteStream(db, streamID); err != nil {
		t.Fatalf("delete %s: %v", streamID, err)
	}
}

// The rediscover order is the only one that adds a clause before WHERE, so it
// is the only combination where a misordered placeholder is possible: join,
// then genre, then exclusion.
func TestRediscoverBindsJoinGenreAndExclusionInOrder(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	alice, _ := seedRankingFixture(t, db)
	seedTrack(t, db, "/m/j.mp3", "Jazzy", "Jazz", 0)
	var alpha, bravo int64
	db.QueryRow(`SELECT id FROM track WHERE title='Alpha'`).Scan(&alpha)
	db.QueryRow(`SELECT id FROM track WHERE title='Bravo'`).Scan(&bravo)
	seedPlay(t, db, "house", bravo, 60)
	seedPlay(t, db, alice, bravo, 50) // Bravo: two plays alice can be said to have heard
	seedPlay(t, db, "house", alpha, 100)

	got, err := DiscoverTracks(db, DiscoverOpts{
		OrderBy: "rediscover", PersonalStream: alice,
		Genre: "Rock", ExcludeStream: "house", Window: 1, Limit: 10,
	})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	titles := make([]string, len(got))
	for i, tr := range got {
		titles[i] = tr.Title
	}
	// Jazzy is filtered by genre and Alpha by the exclusion window (it is the
	// house's one most recent play). Charlie leads the survivors on 0 plays to
	// Bravo's 2. A misbound placeholder cannot produce all three at once.
	if !slices.Equal(titles, []string{"Charlie", "Bravo"}) {
		t.Fatalf("order = %v, want [Charlie Bravo]", titles)
	}
}
