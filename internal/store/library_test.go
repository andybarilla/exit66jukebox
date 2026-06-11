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
	id1, err := UpsertTrack(db, tr, "The Band", "First Album")
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	id2, err := UpsertTrack(db, tr, "The Band", "First Album")
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
