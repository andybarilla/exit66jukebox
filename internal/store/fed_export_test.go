package store

import (
	"strconv"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/model"
)

func TestExportCatalogReturnsLocalTracks(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	_, err := UpsertTrack(db, model.Track{Path: "/m/a.mp3", Title: "A", Duration: 100, TrackNo: 1},
		"Artist", "Artist", "Album")
	if err != nil {
		t.Fatal(err)
	}
	// A remote row must NOT be re-exported (we only share our own files).
	_, _ = UpsertRemoteTrack(db, RemoteTrack{SourcePeer: "x", RemoteID: 1, Title: "R", ArtistName: "Q", AlbumName: "Z"})

	rows, err := ExportCatalog(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 local track exported, got %d", len(rows))
	}
	if rows[0].Title != "A" || rows[0].RemoteID != 1 || rows[0].ArtistName != "Artist" {
		t.Fatalf("unexpected export row: %+v", rows[0])
	}
}

func TestExportCatalogIncludesSourceLibraryID(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	if err := SaveLocalLibraries(db, []LocalLibrary{{Path: "/music/one", Enabled: true}, {Path: "/music/two", Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	_, err := UpsertTrack(db, model.Track{Path: "/music/two/a.mp3", Title: "A"}, "Artist", "Artist", "Album")
	if err != nil {
		t.Fatal(err)
	}

	rows, err := ExportCatalog(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 local track exported, got %d", len(rows))
	}
	if rows[0].SourceLibraryID == "" {
		t.Fatalf("source_library_id should be populated: %+v", rows[0])
	}
}

func TestExportCatalogSkipsDisabledLocalLibraries(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	if err := SaveLocalLibraries(db, []LocalLibrary{
		{Path: "/enabled", Enabled: true, Name: "Enabled"},
		{Path: "/disabled", Enabled: false, Name: "Disabled"},
	}); err != nil {
		t.Fatal(err)
	}
	var enabledLibraryID, disabledLibraryID int64
	if err := db.QueryRow(`SELECT id FROM local_library WHERE path = '/enabled'`).Scan(&enabledLibraryID); err != nil {
		t.Fatalf("enabled library id: %v", err)
	}
	if err := db.QueryRow(`SELECT id FROM local_library WHERE path = '/disabled'`).Scan(&disabledLibraryID); err != nil {
		t.Fatalf("disabled library id: %v", err)
	}
	if _, err := UpsertTrackInLibrary(db, enabledLibraryID, model.Track{Path: "/enabled/a.mp3", Title: "Enabled"}, "Artist", "Artist", "Album"); err != nil {
		t.Fatal(err)
	}
	if _, err := UpsertTrackInLibrary(db, disabledLibraryID, model.Track{Path: "/disabled/a.mp3", Title: "Disabled"}, "Artist", "Artist", "Album"); err != nil {
		t.Fatal(err)
	}

	rows, err := ExportCatalog(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Title != "Enabled" {
		t.Fatalf("expected only enabled library track exported, got %+v", rows)
	}
}

func TestExportCatalogUsesOpaqueLocalLibraryIdentity(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	if err := SaveLocalLibraries(db, []LocalLibrary{{Path: "/music", Enabled: true, Name: "Music"}}); err != nil {
		t.Fatal(err)
	}
	var libraryID int64
	if err := db.QueryRow(`SELECT id FROM local_library WHERE path = '/music'`).Scan(&libraryID); err != nil {
		t.Fatalf("library id: %v", err)
	}
	_, err := UpsertTrackInLibrary(db, libraryID, model.Track{Path: "/music/a.mp3", Title: "A"}, "Artist", "Artist", "Album")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ExportCatalog(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 local track exported, got %d", len(rows))
	}
	if rows[0].SourceLibraryID == "" || rows[0].SourceLibraryID == strconv.FormatInt(libraryID, 10) {
		t.Fatalf("source_library_id should be an opaque stable library identity, got row=%+v library_id=%d", rows[0], libraryID)
	}
}
