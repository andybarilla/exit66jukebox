package store

import (
	"database/sql"
	"testing"
)

// seedEnrichTrack inserts an artist/album/track and returns their ids.
func seedEnrichTrack(t *testing.T, db *sql.DB, artist, album, title, path string) (artistID, albumID, trackID int64) {
	t.Helper()
	res, err := db.Exec(`INSERT INTO artist(name) VALUES(?)`, artist)
	if err != nil {
		t.Fatalf("artist: %v", err)
	}
	artistID, _ = res.LastInsertId()
	res, err = db.Exec(`INSERT INTO album(name, artist_id) VALUES(?, ?)`, album, artistID)
	if err != nil {
		t.Fatalf("album: %v", err)
	}
	albumID, _ = res.LastInsertId()
	res, err = db.Exec(
		`INSERT INTO track(path, mod_time, size, title, artist_id, album_id) VALUES(?, 1, 1, ?, ?, ?)`,
		path, title, artistID, albumID)
	if err != nil {
		t.Fatalf("track: %v", err)
	}
	trackID, _ = res.LastInsertId()
	return
}

func TestTracksNeedingEnrichmentSkipsMatched(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()

	_, _, needID := seedEnrichTrack(t, db, "A", "X", "T1", "/m/a.mp3")
	_, _, matchedID := seedEnrichTrack(t, db, "B", "Y", "T2", "/m/b.mp3")
	if _, err := db.Exec(`UPDATE track SET mbid = 'has-mbid' WHERE id = ?`, matchedID); err != nil {
		t.Fatalf("set mbid: %v", err)
	}

	targets, err := TracksNeedingEnrichment(db)
	if err != nil {
		t.Fatalf("TracksNeedingEnrichment: %v", err)
	}
	if len(targets) != 1 || targets[0].TrackID != needID {
		t.Fatalf("expected only track %d, got %+v", needID, targets)
	}
	if targets[0].Artist != "A" || targets[0].Album != "X" {
		t.Errorf("target carries wrong names: %+v", targets[0])
	}
}

func TestApplyEnrichmentReplacesPlaceholders(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()

	arID, alID, trID := seedEnrichTrack(t, db, "Unknown Artist", "Unknown Album", "a.mp3", "/m/a.mp3")
	e := Enrichment{
		TrackID: trID, ArtistID: arID, AlbumID: alID, Path: "/m/a.mp3",
		RecordingMBID: "rec1", ArtistMBID: "art1", ReleaseMBID: "rel1",
		NewTitle: "Karma Police", NewArtist: "Radiohead", NewAlbum: "OK Computer",
	}
	if err := ApplyEnrichment(db, e); err != nil {
		t.Fatalf("ApplyEnrichment: %v", err)
	}

	var name, mbid string
	db.QueryRow(`SELECT name, mbid FROM artist WHERE id = ?`, arID).Scan(&name, &mbid)
	if name != "Radiohead" || mbid != "art1" {
		t.Errorf("artist = %q/%q, want Radiohead/art1", name, mbid)
	}
	db.QueryRow(`SELECT name, mbid FROM album WHERE id = ?`, alID).Scan(&name, &mbid)
	if name != "OK Computer" || mbid != "rel1" {
		t.Errorf("album = %q/%q, want OK Computer/rel1", name, mbid)
	}
	var title string
	db.QueryRow(`SELECT title, mbid FROM track WHERE id = ?`, trID).Scan(&title, &mbid)
	if title != "Karma Police" || mbid != "rec1" {
		t.Errorf("track = %q/%q, want Karma Police/rec1", title, mbid)
	}
}

func TestApplyEnrichmentKeepsRealNames(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()

	arID, alID, trID := seedEnrichTrack(t, db, "Real Artist", "Real Album", "Real Title", "/m/a.mp3")
	e := Enrichment{
		TrackID: trID, ArtistID: arID, AlbumID: alID, Path: "/m/a.mp3",
		RecordingMBID: "rec1", ArtistMBID: "art1", ReleaseMBID: "rel1",
		NewTitle: "Wrong", NewArtist: "Wrong", NewAlbum: "Wrong",
	}
	if err := ApplyEnrichment(db, e); err != nil {
		t.Fatalf("ApplyEnrichment: %v", err)
	}

	var name string
	db.QueryRow(`SELECT name FROM artist WHERE id = ?`, arID).Scan(&name)
	if name != "Real Artist" {
		t.Errorf("artist renamed to %q, should keep Real Artist", name)
	}
	db.QueryRow(`SELECT name FROM album WHERE id = ?`, alID).Scan(&name)
	if name != "Real Album" {
		t.Errorf("album renamed to %q, should keep Real Album", name)
	}
	var title string
	db.QueryRow(`SELECT title FROM track WHERE id = ?`, trID).Scan(&title)
	if title != "Real Title" {
		t.Errorf("title changed to %q, should keep Real Title", title)
	}
	// MBIDs are still recorded even though names were preserved.
	var mbid string
	db.QueryRow(`SELECT mbid FROM track WHERE id = ?`, trID).Scan(&mbid)
	if mbid != "rec1" {
		t.Errorf("track mbid = %q, want rec1", mbid)
	}
}

func TestApplyEnrichmentReplacesFilenameTitle(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()

	// Title equals the filename => placeholder, should be replaced.
	arID, alID, trID := seedEnrichTrack(t, db, "Real Artist", "Real Album", "a.mp3", "/m/a.mp3")
	e := Enrichment{
		TrackID: trID, ArtistID: arID, AlbumID: alID, Path: "/m/a.mp3",
		RecordingMBID: "rec1", NewTitle: "Karma Police",
	}
	if err := ApplyEnrichment(db, e); err != nil {
		t.Fatalf("ApplyEnrichment: %v", err)
	}
	var title string
	db.QueryRow(`SELECT title FROM track WHERE id = ?`, trID).Scan(&title)
	if title != "Karma Police" {
		t.Errorf("filename-placeholder title = %q, want Karma Police", title)
	}
}

func TestApplyEnrichmentUniqueCollisionKeepsMBID(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()

	// An existing artist "Radiohead" already occupies the unique name.
	if _, err := db.Exec(`INSERT INTO artist(name) VALUES('Radiohead')`); err != nil {
		t.Fatalf("seed existing: %v", err)
	}
	arID, alID, trID := seedEnrichTrack(t, db, "Unknown Artist", "Unknown Album", "a.mp3", "/m/a.mp3")
	e := Enrichment{
		TrackID: trID, ArtistID: arID, AlbumID: alID, Path: "/m/a.mp3",
		RecordingMBID: "rec1", ArtistMBID: "art1", NewArtist: "Radiohead",
	}
	if err := ApplyEnrichment(db, e); err != nil {
		t.Fatalf("ApplyEnrichment should not error on collision: %v", err)
	}
	var name, mbid string
	db.QueryRow(`SELECT name, mbid FROM artist WHERE id = ?`, arID).Scan(&name, &mbid)
	if name != "Unknown Artist" {
		t.Errorf("artist renamed to %q despite collision, should stay Unknown Artist", name)
	}
	if mbid != "art1" {
		t.Errorf("artist mbid = %q, want art1 (MBID recorded despite skipped rename)", mbid)
	}
}

func TestApplyEnrichmentSkipsSharedPlaceholderRow(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()

	// Two untagged tracks collapse onto one shared "Unknown Artist"/"Unknown
	// Album" row (scan keys artist by name, album by name+artist). Enriching one
	// must not rename or stamp the shared row, or it mis-attributes the sibling.
	arID, alID, trA := seedEnrichTrack(t, db, "Unknown Artist", "Unknown Album", "a.mp3", "/m/a.mp3")
	resB, err := db.Exec(
		`INSERT INTO track(path, mod_time, size, title, artist_id, album_id) VALUES('/m/b.mp3', 1, 1, 'b.mp3', ?, ?)`,
		arID, alID)
	if err != nil {
		t.Fatalf("track B: %v", err)
	}
	trB, _ := resB.LastInsertId()

	e := Enrichment{
		TrackID: trA, ArtistID: arID, AlbumID: alID, Path: "/m/a.mp3",
		RecordingMBID: "rec1", ArtistMBID: "art1", ReleaseMBID: "rel1",
		NewTitle: "Karma Police", NewArtist: "Radiohead", NewAlbum: "OK Computer",
	}
	if err := ApplyEnrichment(db, e); err != nil {
		t.Fatalf("ApplyEnrichment: %v", err)
	}

	// The shared artist/album row is untouched (name + mbid).
	var name, mbid string
	db.QueryRow(`SELECT name, mbid FROM artist WHERE id = ?`, arID).Scan(&name, &mbid)
	if name != "Unknown Artist" || mbid != "" {
		t.Errorf("shared artist = %q/%q, want Unknown Artist/'' (untouched)", name, mbid)
	}
	db.QueryRow(`SELECT name, mbid FROM album WHERE id = ?`, alID).Scan(&name, &mbid)
	if name != "Unknown Album" || mbid != "" {
		t.Errorf("shared album = %q/%q, want Unknown Album/'' (untouched)", name, mbid)
	}
	// But the per-track recording mbid and title still apply to track A only.
	var title string
	db.QueryRow(`SELECT mbid, title FROM track WHERE id = ?`, trA).Scan(&mbid, &title)
	if mbid != "rec1" || title != "Karma Police" {
		t.Errorf("track A = %q/%q, want rec1/Karma Police", mbid, title)
	}
	// Track B is left entirely for its own future match.
	db.QueryRow(`SELECT mbid, title FROM track WHERE id = ?`, trB).Scan(&mbid, &title)
	if mbid != "" || title != "b.mp3" {
		t.Errorf("sibling track B = %q/%q, want ''/b.mp3 (unaffected)", mbid, title)
	}
}

func TestApplyEnrichmentIdempotent(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()

	arID, alID, trID := seedEnrichTrack(t, db, "Unknown Artist", "Unknown Album", "a.mp3", "/m/a.mp3")
	e := Enrichment{
		TrackID: trID, ArtistID: arID, AlbumID: alID, Path: "/m/a.mp3",
		RecordingMBID: "rec1", ArtistMBID: "art1", ReleaseMBID: "rel1",
		NewTitle: "Karma Police", NewArtist: "Radiohead", NewAlbum: "OK Computer",
	}
	if err := ApplyEnrichment(db, e); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if err := ApplyEnrichment(db, e); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	var name string
	db.QueryRow(`SELECT name FROM artist WHERE id = ?`, arID).Scan(&name)
	if name != "Radiohead" {
		t.Errorf("artist = %q after double apply, want Radiohead", name)
	}
}

func TestAlbumCoverByTrackAndSet(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()

	_, alID, trID := seedEnrichTrack(t, db, "A", "X", "T", "/m/a.mp3")
	if _, ok := AlbumCoverByTrack(db, trID); ok {
		t.Error("expected ok=false for album with no cover")
	}
	if err := SetAlbumCover(db, alID, "/covers/1.jpg"); err != nil {
		t.Fatalf("SetAlbumCover: %v", err)
	}
	cover, ok := AlbumCoverByTrack(db, trID)
	if !ok || cover != "/covers/1.jpg" {
		t.Fatalf("AlbumCoverByTrack = %q/%v, want /covers/1.jpg/true", cover, ok)
	}
}
