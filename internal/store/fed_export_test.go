package store

import (
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
