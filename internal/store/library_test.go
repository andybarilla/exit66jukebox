package store

import (
	"fmt"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/model"
)

func TestUpsertTrackIsIdempotent(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	tr := model.Track{
		Path: "/music/a.mp3", ModTime: 100, Size: 2048,
		Title: "Song A", TrackNo: 1, Genre: "Rock", Duration: 180,
	}
	id1, err := UpsertTrack(db, tr, "The Band", "", "First Album")
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	id2, err := UpsertTrack(db, tr, "The Band", "", "First Album")
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("expected same track id on re-upsert, got %d then %d", id1, id2)
	}

	var artists int
	db.QueryRow(`SELECT count(*) FROM artist`).Scan(&artists)
	if artists != 1 {
		t.Fatalf("expected 1 artist, got %d", artists)
	}
}

func TestUpsertTrackKeysByLibraryAndPath(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	firstLibraryID, err := EnsureLocalLibrary(db, "/first", "First")
	if err != nil {
		t.Fatalf("first library: %v", err)
	}
	secondLibraryID, err := EnsureLocalLibrary(db, "/second", "Second")
	if err != nil {
		t.Fatalf("second library: %v", err)
	}
	track := model.Track{Path: "song.mp3", ModTime: 100, Size: 2048, Title: "Song"}
	firstTrackID, err := UpsertTrackInLibrary(db, firstLibraryID, track, "Artist", "", "Album")
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	secondTrackID, err := UpsertTrackInLibrary(db, secondLibraryID, track, "Artist", "", "Album")
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if firstTrackID == secondTrackID {
		t.Fatalf("expected matching paths in different libraries to use distinct track rows")
	}
	if _, _, ok := TrackStampInLibrary(db, secondLibraryID, "song.mp3"); !ok {
		t.Fatalf("expected unchanged-file stamp scoped to second library")
	}
}

func TestListTracksSearchAndPage(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	UpsertTrack(db, model.Track{Path: "/m/1.mp3", Title: "Blue Sky"}, "A", "", "X")
	UpsertTrack(db, model.Track{Path: "/m/2.mp3", Title: "Red Moon"}, "B", "", "Y")
	UpsertTrack(db, model.Track{Path: "/m/3.mp3", Title: "Blue Moon"}, "C", "", "Z")

	all, err := ListTracks(db, "", 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 tracks, got %d", len(all))
	}

	blue, _ := ListTracks(db, "Blue", 10, 0)
	if len(blue) != 2 {
		t.Fatalf("expected 2 'Blue' tracks, got %d", len(blue))
	}

	page, _ := ListTracks(db, "", 1, 1)
	if len(page) != 1 {
		t.Fatalf("expected 1 track on page, got %d", len(page))
	}
}

func TestDeleteLocalLibraryTracksExceptAllowsLargeKeepSet(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	firstLibraryID, err := EnsureLocalLibrary(db, "/first", "First")
	if err != nil {
		t.Fatalf("first library: %v", err)
	}
	secondLibraryID, err := EnsureLocalLibrary(db, "/second", "Second")
	if err != nil {
		t.Fatalf("second library: %v", err)
	}

	keepPath := "/first/keep.mp3"
	stalePath := "/first/stale.mp3"
	if _, err := UpsertTrackInLibrary(db, firstLibraryID, model.Track{Path: keepPath, Title: "Keep"}, "Kept Artist", "", "Kept Album"); err != nil {
		t.Fatalf("upsert kept track: %v", err)
	}
	if _, err := UpsertTrackInLibrary(db, firstLibraryID, model.Track{Path: stalePath, Title: "Stale"}, "Stale Artist", "", "Stale Album"); err != nil {
		t.Fatalf("upsert stale track: %v", err)
	}
	if _, err := UpsertTrackInLibrary(db, secondLibraryID, model.Track{Path: stalePath, Title: "Other"}, "Other Artist", "", "Other Album"); err != nil {
		t.Fatalf("upsert other library track: %v", err)
	}

	keepPaths := make([]string, 40_000)
	keepPaths[0] = keepPath
	for i := 1; i < len(keepPaths); i++ {
		keepPaths[i] = fmt.Sprintf("/unseen/%05d.mp3", i)
	}

	if err := DeleteLocalLibraryTracksExcept(db, firstLibraryID, keepPaths); err != nil {
		t.Fatalf("delete local tracks with large keep set: %v", err)
	}
	if _, _, ok := TrackStampInLibrary(db, firstLibraryID, keepPath); !ok {
		t.Fatalf("expected kept track to remain")
	}
	if _, _, ok := TrackStampInLibrary(db, firstLibraryID, stalePath); ok {
		t.Fatalf("expected stale track to be pruned from scanned library")
	}
	if _, _, ok := TrackStampInLibrary(db, secondLibraryID, stalePath); !ok {
		t.Fatalf("expected matching path in another library to remain")
	}
}
