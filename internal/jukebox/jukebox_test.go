package jukebox

import (
	"fmt"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

func TestRequestRejectsDuplicateAndRecent(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	id, _ := store.UpsertTrack(db, model.Track{Path: "/m/a.mp3", Title: "A"}, "Band", "Album")

	jb := New(db, Config{HistoryWindow: 5})
	jb.EnsureStream("sess1", "private")

	if got, _ := jb.Request("sess1", id); got != Requested {
		t.Fatalf("first request: want Requested, got %v", got)
	}
	if got, _ := jb.Request("sess1", id); got != AlreadyQueued {
		t.Fatalf("duplicate request: want AlreadyQueued, got %v", got)
	}

	tr, ok := jb.Next("sess1")
	if !ok || tr.ID != id {
		t.Fatalf("Next: want track %d ok=true; got %d ok=%v", id, tr.ID, ok)
	}
	if got, _ := jb.Request("sess1", id); got != RecentlyPlayed {
		t.Fatalf("recent request: want RecentlyPlayed, got %v", got)
	}
}

func TestNextEmptyQueue(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	jb := New(db, Config{HistoryWindow: 5})
	jb.EnsureStream("s", "private")
	if _, ok := jb.Next("s"); ok {
		t.Fatalf("expected ok=false on empty queue")
	}
}

func TestRequestAlbumQueuesAllTracks(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	store.UpsertTrack(db, model.Track{Path: "/m/1.mp3", Title: "One", TrackNo: 1}, "Band", "LP")
	store.UpsertTrack(db, model.Track{Path: "/m/2.mp3", Title: "Two", TrackNo: 2}, "Band", "LP")

	var albumID int64
	db.QueryRow(`SELECT id FROM album WHERE name='LP'`).Scan(&albumID)

	jb := New(db, Config{HistoryWindow: 5})
	jb.EnsureStream("s", "private")
	n := jb.RequestAlbum("s", albumID)
	if n != 2 {
		t.Fatalf("expected 2 tracks queued, got %d", n)
	}
	q, _ := jb.Queue("s")
	if len(q) != 2 {
		t.Fatalf("expected queue length 2, got %d", len(q))
	}
}

func TestRequestAllowsRepeatWhenWindowZero(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	id, _ := store.UpsertTrack(db, model.Track{Path: "/m/z.mp3", Title: "Z"}, "B", "A")
	jb := New(db, Config{HistoryWindow: 0})
	jb.EnsureStream("s", "private")
	if got, _ := jb.Request("s", id); got != Requested {
		t.Fatalf("want Requested, got %v", got)
	}
	jb.Next("s") // play it -> goes to history
	if got, _ := jb.Request("s", id); got != Requested {
		t.Fatalf("window=0 should allow immediate repeat, got %v", got)
	}
}

func TestStartStationFillsEmptyQueue(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	jb := New(db, Config{HistoryWindow: 5})
	jb.EnsureStream("s", "private")
	for i := 0; i < 20; i++ {
		store.UpsertTrack(db, model.Track{
			Path: fmt.Sprintf("/m/%d.mp3", i), Title: fmt.Sprintf("T%d", i), Genre: "Rock",
		}, "Band", "Album")
	}

	if err := jb.StartStation("s", "Rock", 3, 10); err != nil {
		t.Fatalf("start: %v", err)
	}
	q, _ := jb.Queue("s")
	if len(q) != 10 {
		t.Fatalf("expected immediate fill of 10, got %d", len(q))
	}
}

func TestNextRefillsBelowThreshold(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	jb := New(db, Config{HistoryWindow: 0}) // disable fairness to keep counting simple
	jb.EnsureStream("s", "private")
	for i := 0; i < 30; i++ {
		store.UpsertTrack(db, model.Track{
			Path: fmt.Sprintf("/m/%d.mp3", i), Title: fmt.Sprintf("T%d", i), Genre: "Rock",
		}, "Band", "Album")
	}
	jb.StartStation("s", "Rock", 3, 10) // fills to 10

	// Pop down to the threshold boundary. After popping to <3, Next refills.
	for i := 0; i < 8; i++ {
		if _, ok := jb.Next("s"); !ok {
			t.Fatalf("unexpected empty queue at pop %d", i)
		}
	}
	n, _ := store.QueueLen(db, "s")
	if n < 3 {
		t.Fatalf("expected queue refilled to >=3 after draining, got %d", n)
	}
}

func TestStopStationStopsRefill(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	jb := New(db, Config{HistoryWindow: 0})
	jb.EnsureStream("s", "private")
	for i := 0; i < 30; i++ {
		store.UpsertTrack(db, model.Track{
			Path: fmt.Sprintf("/m/%d.mp3", i), Title: fmt.Sprintf("T%d", i), Genre: "Rock",
		}, "Band", "Album")
	}
	jb.StartStation("s", "Rock", 3, 10)
	if err := jb.StopStation("s"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	// Drain fully; with no station, queue must reach 0 and stay there.
	for {
		if _, ok := jb.Next("s"); !ok {
			break
		}
	}
	n, _ := store.QueueLen(db, "s")
	if n != 0 {
		t.Fatalf("expected drained queue to stay empty after stop, got %d", n)
	}
}
