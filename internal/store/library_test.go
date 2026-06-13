package store

import (
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
