package store

import (
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/model"
)

func TestQueueWithRequester(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	id, _ := UpsertTrack(db, model.Track{Path: "/m/a.mp3", Title: "A"}, "Band", "LP")
	if err := EnsureStream(db, "s", "", "private"); err != nil {
		t.Fatal(err)
	}
	if err := Enqueue(db, "s", id, "Mira"); err != nil {
		t.Fatal(err)
	}

	rows, err := QueueWithRequester(db, "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if rows[0].TrackID != id || rows[0].RequestedBy != "Mira" {
		t.Fatalf("got %+v, want trackID=%d requestedBy=Mira", rows[0], id)
	}
}
