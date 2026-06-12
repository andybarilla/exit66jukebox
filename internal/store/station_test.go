package store

import (
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/model"
)

func TestStationRoundTrip(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	if err := EnsureStream(db, "s", "", "private"); err != nil {
		t.Fatalf("ensure stream: %v", err)
	}

	if _, ok := GetStation(db, "s"); ok {
		t.Fatalf("expected no station initially")
	}

	if err := UpsertStation(db, Station{StreamID: "s", Genre: "Rock", Threshold: 3, Batch: 10}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	st, ok := GetStation(db, "s")
	if !ok || st.Genre != "Rock" || st.Threshold != 3 || st.Batch != 10 {
		t.Fatalf("unexpected station: %+v ok=%v", st, ok)
	}

	// Upsert again changes genre in place.
	if err := UpsertStation(db, Station{StreamID: "s", Genre: "Jazz", Threshold: 3, Batch: 10}); err != nil {
		t.Fatalf("upsert 2: %v", err)
	}
	st, _ = GetStation(db, "s")
	if st.Genre != "Jazz" {
		t.Fatalf("expected genre updated to Jazz, got %q", st.Genre)
	}

	if err := DeleteStation(db, "s"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := GetStation(db, "s"); ok {
		t.Fatalf("expected station gone after delete")
	}
}

func TestQueueLen(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	EnsureStream(db, "s", "", "private")
	if n, _ := QueueLen(db, "s"); n != 0 {
		t.Fatalf("expected empty queue len 0, got %d", n)
	}
	id, _ := UpsertTrack(db, model.Track{Path: "/m/a.mp3", Title: "A"}, "B", "X")
	if err := Enqueue(db, "s", id, ""); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if n, _ := QueueLen(db, "s"); n != 1 {
		t.Fatalf("expected queue len 1, got %d", n)
	}
}
