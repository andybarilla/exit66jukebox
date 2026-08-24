package store

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/model"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func insertTestTrack(t *testing.T, db *sql.DB, title string) int64 {
	t.Helper()
	id, err := UpsertTrack(db, model.Track{Path: "/m/" + title + ".mp3", Title: title}, "Band", "", "LP")
	if err != nil {
		t.Fatalf("upsert track: %v", err)
	}
	return id
}

func TestCreateSharedStreamEnforcesCapInStore(t *testing.T) {
	db := openTestDB(t)
	// The cap counts every shared stream, house included, so the first
	// MaxSharedStreams creates succeed and the next one fails.
	for i := 0; i < MaxSharedStreams; i++ {
		if err := CreateSharedStream(db, streamIDFor(i), "s"); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	err := CreateSharedStream(db, "one-too-many", "s")
	if !errors.Is(err, ErrStreamCapReached) {
		t.Fatalf("want ErrStreamCapReached, got %v", err)
	}
	got, err := ListStreams(db, KindShared)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != MaxSharedStreams {
		t.Fatalf("want %d shared streams, got %d", MaxSharedStreams, len(got))
	}
}

func streamIDFor(i int) string { return string(rune('a' + i)) }

// EnsureSharedStream is the boot-time house path: it must survive a DB that is
// already at (or over) the cap, or an over-capped instance could not start.
func TestEnsureSharedStreamBypassesCap(t *testing.T) {
	db := openTestDB(t)
	for i := 0; i < MaxSharedStreams; i++ {
		if err := CreateSharedStream(db, streamIDFor(i), "s"); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	if err := EnsureSharedStream(db, "house", "House"); err != nil {
		t.Fatalf("ensure house at cap: %v", err)
	}
	st, ok, err := GetStream(db, "house")
	if err != nil || !ok {
		t.Fatalf("get house: ok=%v err=%v", ok, err)
	}
	if st.Kind != KindShared {
		t.Fatalf("house kind: want shared, got %q", st.Kind)
	}
}

// EnsurePrivateStream is the only implicit-create path left, and it can never
// produce a shared stream — that is what makes the kind-based auth gate
// unbypassable by inventing a URL.
func TestEnsurePrivateStreamNeverCreatesShared(t *testing.T) {
	db := openTestDB(t)
	if err := EnsurePrivateStream(db, "bobs-invented-stream"); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	st, ok, err := GetStream(db, "bobs-invented-stream")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if st.Kind != KindPrivate {
		t.Fatalf("kind: want private, got %q", st.Kind)
	}
	shared, err := ListStreams(db, KindShared)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(shared) != 0 {
		t.Fatalf("want no shared streams, got %v", shared)
	}
}

// A private stream does not count against the shared cap.
func TestPrivateStreamsDoNotCountAgainstCap(t *testing.T) {
	db := openTestDB(t)
	for i := 0; i < 10; i++ {
		if err := EnsurePrivateStream(db, "p"+streamIDFor(i)); err != nil {
			t.Fatalf("ensure private: %v", err)
		}
	}
	if err := CreateSharedStream(db, "kitchen", "Kitchen"); err != nil {
		t.Fatalf("create shared: %v", err)
	}
}

func TestRenameStreamKeepsIDAndAllowsDuplicateNames(t *testing.T) {
	db := openTestDB(t)
	if err := CreateSharedStream(db, "a", "Kitchen"); err != nil {
		t.Fatalf("create a: %v", err)
	}
	if err := CreateSharedStream(db, "b", "Patio"); err != nil {
		t.Fatalf("create b: %v", err)
	}
	if err := RenameStream(db, "b", "Kitchen"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	for _, id := range []string{"a", "b"} {
		st, ok, err := GetStream(db, id)
		if err != nil || !ok {
			t.Fatalf("get %s: ok=%v err=%v", id, ok, err)
		}
		if st.ID != id {
			t.Fatalf("id changed: want %q, got %q", id, st.ID)
		}
		if st.Name != "Kitchen" {
			t.Fatalf("%s name: want Kitchen, got %q", id, st.Name)
		}
	}
}

func TestDeleteStreamRemovesQueueAndStation(t *testing.T) {
	db := openTestDB(t)
	if err := CreateSharedStream(db, "party", "Party"); err != nil {
		t.Fatalf("create: %v", err)
	}
	tid := insertTestTrack(t, db, "Song")
	if err := Enqueue(db, "party", tid, "me"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := UpsertStation(db, Station{StreamID: "party", Genre: "rock", Threshold: 3, Batch: 10}); err != nil {
		t.Fatalf("station: %v", err)
	}

	if err := DeleteStream(db, "party"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok, _ := GetStream(db, "party"); ok {
		t.Fatal("stream row still present after delete")
	}
	ids, err := QueueTrackIDs(db, "party")
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("queue rows survived delete: %v", ids)
	}
	if _, ok := GetStation(db, "party"); ok {
		t.Fatal("station row survived delete")
	}
	// Deleting frees a cap slot.
	if err := CreateSharedStream(db, "party2", "Party"); err != nil {
		t.Fatalf("create after delete: %v", err)
	}
}

func TestDeleteStreamMissingIsNotAnError(t *testing.T) {
	db := openTestDB(t)
	if err := DeleteStream(db, "nope"); err != nil {
		t.Fatalf("delete missing: %v", err)
	}
}

func TestCreateSharedStreamRejectsDuplicateID(t *testing.T) {
	db := openTestDB(t)
	if err := CreateSharedStream(db, "kitchen", "Kitchen"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := CreateSharedStream(db, "kitchen", "Kitchen"); !errors.Is(err, ErrStreamExists) {
		t.Fatalf("want ErrStreamExists, got %v", err)
	}
}
