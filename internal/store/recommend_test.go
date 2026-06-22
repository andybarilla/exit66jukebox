package store

import (
	"database/sql"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/model"
)

// setTrackMBID and bumpPlays are small helpers for the recommendation-mapping tests.
func setTrackMBID(t *testing.T, db *sql.DB, trackID int64, mbid string) {
	t.Helper()
	if _, err := db.Exec(`UPDATE track SET mbid = ? WHERE id = ?`, mbid, trackID); err != nil {
		t.Fatalf("set track mbid: %v", err)
	}
}

func setArtistMBID(t *testing.T, db *sql.DB, artistID int64, mbid string) {
	t.Helper()
	if _, err := db.Exec(`UPDATE artist SET mbid = ? WHERE id = ?`, mbid, artistID); err != nil {
		t.Fatalf("set artist mbid: %v", err)
	}
}

func setPlays(t *testing.T, db *sql.DB, trackID int64, n int) {
	t.Helper()
	if _, err := db.Exec(`UPDATE track SET play_count = ? WHERE id = ?`, n, trackID); err != nil {
		t.Fatalf("set play_count: %v", err)
	}
}

func TestTracksByRecordingMBIDsMatchesAndOrders(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()

	_, _, t1 := seedEnrichTrack(t, db, "A", "X", "T1", "/m/a.mp3")
	_, _, t2 := seedEnrichTrack(t, db, "B", "Y", "T2", "/m/b.mp3")
	seedEnrichTrack(t, db, "C", "Z", "T3", "/m/c.mp3") // no mbid → never matched
	setTrackMBID(t, db, t1, "rec-1")
	setTrackMBID(t, db, t2, "rec-2")

	// Input order is rec-2 then rec-1 (descending recommendation score); the
	// result must preserve it. The unknown MBID is silently skipped.
	got, err := TracksByRecordingMBIDs(db, []string{"rec-2", "rec-unknown", "rec-1"})
	if err != nil {
		t.Fatalf("TracksByRecordingMBIDs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d tracks, want 2", len(got))
	}
	if got[0].ID != t2 || got[1].ID != t1 {
		t.Errorf("order = %d,%d want %d,%d (input order preserved)", got[0].ID, got[1].ID, t2, t1)
	}
}

func TestRecommendationsHideDisabledLocalLibrary(t *testing.T) {
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
	visibleID, err := UpsertTrackInLibrary(db, enabledLibraryID, model.Track{Path: "/enabled/a.mp3", Title: "Visible"}, "Visible Artist", "", "Visible Album")
	if err != nil {
		t.Fatalf("visible track: %v", err)
	}
	hiddenID, err := UpsertTrackInLibrary(db, disabledLibraryID, model.Track{Path: "/disabled/a.mp3", Title: "Hidden"}, "Hidden Artist", "", "Hidden Album")
	if err != nil {
		t.Fatalf("hidden track: %v", err)
	}
	setTrackMBID(t, db, visibleID, "visible-rec")
	setTrackMBID(t, db, hiddenID, "hidden-rec")
	setPlays(t, db, visibleID, 1)
	setPlays(t, db, hiddenID, 50)
	var hiddenArtistID int64
	if err := db.QueryRow(`SELECT artist_id FROM track WHERE id = ?`, hiddenID).Scan(&hiddenArtistID); err != nil {
		t.Fatalf("hidden artist id: %v", err)
	}
	setArtistMBID(t, db, hiddenArtistID, "hidden-artist")
	if _, err := db.Exec(`UPDATE local_library SET enabled = 0 WHERE id = ?`, disabledLibraryID); err != nil {
		t.Fatalf("disable library: %v", err)
	}

	byRecording, err := TracksByRecordingMBIDs(db, []string{"hidden-rec", "visible-rec"})
	if err != nil {
		t.Fatalf("TracksByRecordingMBIDs: %v", err)
	}
	if len(byRecording) != 1 || byRecording[0].ID != visibleID {
		t.Fatalf("recording recommendations = %+v, want only visible", byRecording)
	}

	topArtists, err := TopArtists(db, 10)
	if err != nil {
		t.Fatalf("TopArtists: %v", err)
	}
	if len(topArtists) != 1 || topArtists[0].Name != "Visible Artist" {
		t.Fatalf("top artists = %+v, want only Visible Artist", topArtists)
	}

	similar, err := TracksBySimilarArtists(db, []string{"hidden artist", "visible artist"}, []string{"hidden-artist"}, 5)
	if err != nil {
		t.Fatalf("TracksBySimilarArtists: %v", err)
	}
	if len(similar) != 1 || similar[0].ID != visibleID {
		t.Fatalf("similar-artist tracks = %+v, want only visible", similar)
	}
}

func TestTracksByRecordingMBIDsEmpty(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	got, err := TracksByRecordingMBIDs(db, nil)
	if err != nil {
		t.Fatalf("TracksByRecordingMBIDs(nil): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d, want 0", len(got))
	}
}

func TestTopArtistsByPlayCount(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()

	a1, _, t1 := seedEnrichTrack(t, db, "Alpha", "X", "T1", "/m/a.mp3")
	a2, _, t2 := seedEnrichTrack(t, db, "Beta", "Y", "T2", "/m/b.mp3")
	seedEnrichTrack(t, db, "Gamma", "Z", "T3", "/m/c.mp3") // 0 plays → excluded
	setPlays(t, db, t1, 3)
	setPlays(t, db, t2, 9)
	setArtistMBID(t, db, a1, "mbid-alpha")

	got, err := TopArtists(db, 10)
	if err != nil {
		t.Fatalf("TopArtists: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d artists, want 2 (zero-play artist excluded)", len(got))
	}
	if got[0].ID != a2 {
		t.Errorf("top artist = %d (%q), want %d Beta (most plays)", got[0].ID, got[0].Name, a2)
	}
	if got[1].ID != a1 || got[1].Mbid != "mbid-alpha" {
		t.Errorf("got[1] = %+v, want Alpha with mbid", got[1])
	}
}

func TestTracksBySimilarArtistsMatchesByMBIDAndName(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()

	// Match by MBID even when the name differs.
	aBOC, _, _ := seedEnrichTrack(t, db, "Boards of Canada", "Geogaddi", "Music Is Math", "/m/boc1.mp3")
	setArtistMBID(t, db, aBOC, "mbid-boc")
	// Second track for same artist to exercise the per-artist cap.
	db.Exec(`INSERT INTO track(path, library_id, mod_time, size, title, artist_id, album_id)
		VALUES('/m/boc2.mp3', (SELECT id FROM local_library WHERE path = ''), 1, 1, 'Sunshine Recorder', ?, (SELECT album_id FROM track WHERE artist_id = ? LIMIT 1))`, aBOC, aBOC)
	// Match by normalized name (no MBID).
	seedEnrichTrack(t, db, "Aphex Twin", "SAW", "Xtal", "/m/aphex.mp3")
	// A local artist nobody recommended.
	seedEnrichTrack(t, db, "Nobody", "N", "Track", "/m/nobody.mp3")

	got, err := TracksBySimilarArtists(db,
		[]string{"aphex twin"},          // by name, lowercased
		[]string{"mbid-boc", "unknown"}, // by mbid
		1)                               // cap one track per artist
	if err != nil {
		t.Fatalf("TracksBySimilarArtists: %v", err)
	}
	// One from BOC (capped at 1 of 2) + one from Aphex Twin = 2.
	if len(got) != 2 {
		t.Fatalf("got %d tracks, want 2", len(got))
	}
	gotArtists := map[int64]bool{}
	for _, tr := range got {
		gotArtists[tr.ArtistID] = true
	}
	if !gotArtists[aBOC] {
		t.Errorf("expected a Boards of Canada track (matched by mbid)")
	}
}
