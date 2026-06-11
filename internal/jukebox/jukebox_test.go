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

	if got, _ := jb.Request("sess1", id, ""); got != Requested {
		t.Fatalf("first request: want Requested, got %v", got)
	}
	if got, _ := jb.Request("sess1", id, ""); got != AlreadyQueued {
		t.Fatalf("duplicate request: want AlreadyQueued, got %v", got)
	}

	tr, ok := jb.Next("sess1")
	if !ok || tr.ID != id {
		t.Fatalf("Next: want track %d ok=true; got %d ok=%v", id, tr.ID, ok)
	}
	if got, _ := jb.Request("sess1", id, ""); got != RecentlyPlayed {
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
	n := jb.RequestAlbum("s", albumID, "")
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
	if got, _ := jb.Request("s", id, ""); got != Requested {
		t.Fatalf("want Requested, got %v", got)
	}
	jb.Next("s") // play it -> goes to history
	if got, _ := jb.Request("s", id, ""); got != Requested {
		t.Fatalf("window=0 should allow immediate repeat, got %v", got)
	}
}

func TestQueueReturnsRequester(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	id, _ := store.UpsertTrack(db, model.Track{Path: "/m/a.mp3", Title: "A"}, "Band", "LP")
	jb := New(db, Config{HistoryWindow: 0})
	jb.EnsureStream("s", "private")
	if _, err := jb.Request("s", id, "Mira"); err != nil {
		t.Fatal(err)
	}
	q, err := jb.Queue("s")
	if err != nil {
		t.Fatal(err)
	}
	if len(q) != 1 || q[0].RequestedBy != "Mira" || q[0].Track.ID != id {
		t.Fatalf("got %+v", q)
	}
}

func TestShuffleFlagDrivesNext(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	jb := New(db, Config{HistoryWindow: 0})
	jb.EnsureStream("s", "private")
	for _, ti := range []string{"A", "B", "C"} {
		id, _ := store.UpsertTrack(db, model.Track{Path: "/m/" + ti + ".mp3", Title: ti}, "Band", "LP")
		jb.Request("s", id, "")
	}
	jb.SetShuffle("s", true)
	got := 0
	for {
		if _, ok := jb.Next("s"); !ok {
			break
		}
		got++
	}
	if got != 3 {
		t.Fatalf("want 3 pops, got %d", got)
	}
}
