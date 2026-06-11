package jukebox

import (
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

	if got := jb.Request("sess1", id); got != Requested {
		t.Fatalf("first request: want Requested, got %v", got)
	}
	if got := jb.Request("sess1", id); got != AlreadyQueued {
		t.Fatalf("duplicate request: want AlreadyQueued, got %v", got)
	}

	tr, ok := jb.Next("sess1")
	if !ok || tr.ID != id {
		t.Fatalf("Next: want track %d ok=true; got %d ok=%v", id, tr.ID, ok)
	}
	if got := jb.Request("sess1", id); got != RecentlyPlayed {
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
