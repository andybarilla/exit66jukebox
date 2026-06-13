package store

import (
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/model"
)

func TestQueueWithRequester(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	id, _ := UpsertTrack(db, model.Track{Path: "/m/a.mp3", Title: "A"}, "Band", "", "LP")
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

func TestPopNextShuffleEmptiesQueue(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	EnsureStream(db, "s", "", "private")
	var ids []int64
	for _, title := range []string{"A", "B", "C"} {
		id, _ := UpsertTrack(db, model.Track{Path: "/m/" + title + ".mp3", Title: title}, "Band", "", "LP")
		Enqueue(db, "s", id, "")
		ids = append(ids, id)
	}
	seen := map[int64]bool{}
	for range ids {
		tid, ok := PopNextShuffle(db, "s")
		if !ok {
			t.Fatal("expected ok=true while queue non-empty")
		}
		if seen[tid] {
			t.Fatalf("popped duplicate %d", tid)
		}
		seen[tid] = true
	}
	if _, ok := PopNextShuffle(db, "s"); ok {
		t.Fatal("expected ok=false on empty queue")
	}
	if len(seen) != 3 {
		t.Fatalf("want 3 distinct pops, got %d", len(seen))
	}
}
