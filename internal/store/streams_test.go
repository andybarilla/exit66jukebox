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

// EnsurePrivateStream is now reached from a request (first-use provisioning of
// the caller's personal stream), but only ever with a server-derived id. It can
// never produce a shared stream, so even handed an id a client chose, the
// kind-based auth gate stays unbypassable.
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

// Per-user ids must be distinct, and must never collide with the alias or with
// a shared id — the alias is what a client sends, not what a row is keyed on.
func TestPersonalStreamIDsAreDistinctAndNamespaced(t *testing.T) {
	a, b := PersonalStreamID(1), PersonalStreamID(2)
	if a == b {
		t.Fatalf("two users derived the same id: %q", a)
	}
	for _, id := range []string{a, b} {
		if !IsPersonalStreamID(id) {
			t.Errorf("%q is not recognised as a personal stream id", id)
		}
		if id == PersonalStreamAlias {
			t.Errorf("derived id collides with the alias: %q", id)
		}
	}
	for _, id := range []string{PersonalStreamAlias, "house", "a1b2c3", ""} {
		if IsPersonalStreamID(id) {
			t.Errorf("%q wrongly claimed as a personal stream id", id)
		}
	}
}

// Deleting a user takes their personal stream with it. Nothing else can: the
// rename/delete routes refuse private streams, so the row would otherwise be
// stranded with queue rows behind a foreign key.
func TestDeleteUserRemovesTheirPersonalStream(t *testing.T) {
	db := openTestDB(t)
	uid, err := CreateUser(db, "bob@example.com", "Bob", "h", false, true)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	other, err := CreateUser(db, "alice@example.com", "Alice", "h", false, true)
	if err != nil {
		t.Fatalf("create other user: %v", err)
	}
	tid := insertTestTrack(t, db, "/m/a.mp3")
	for _, id := range []int64{uid, other} {
		if err := EnsurePrivateStream(db, PersonalStreamID(id)); err != nil {
			t.Fatalf("ensure: %v", err)
		}
		if err := Enqueue(db, PersonalStreamID(id), tid, "x"); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
	}
	if err := UpsertStation(db, Station{StreamID: PersonalStreamID(uid), Genre: "rock", Threshold: 3, Batch: 10}); err != nil {
		t.Fatalf("station: %v", err)
	}

	if err := DeleteUser(db, uid); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if _, ok, _ := GetStream(db, PersonalStreamID(uid)); ok {
		t.Error("the deleted user's personal stream row survived")
	}
	if ids, _ := QueueTrackIDs(db, PersonalStreamID(uid)); len(ids) != 0 {
		t.Errorf("their queue rows survived: %v", ids)
	}
	if _, ok := GetStation(db, PersonalStreamID(uid)); ok {
		t.Error("their station survived")
	}
	// And nobody else's went with them.
	if _, ok, _ := GetStream(db, PersonalStreamID(other)); !ok {
		t.Error("another user's personal stream was deleted")
	}
	if ids, _ := QueueTrackIDs(db, PersonalStreamID(other)); len(ids) != 1 {
		t.Errorf("another user's queue was emptied: %v", ids)
	}
}

// A shared stream in the per-user namespace would be permanently unreachable
// and undeletable: every route resolves personal ids first and refuses them,
// delete included. Nothing can reach this today (createStream mints its own
// id), so the rejection is what keeps that true if a caller-supplied id ever
// arrives.
func TestCreateSharedStreamRejectsReservedIDs(t *testing.T) {
	db := openTestDB(t)
	for _, id := range []string{PersonalStreamID(5), PersonalStreamAlias, personalStreamPrefix} {
		if err := CreateSharedStream(db, id, "Sneaky"); !errors.Is(err, ErrReservedStreamID) {
			t.Errorf("CreateSharedStream(%q) = %v, want ErrReservedStreamID", id, err)
		}
		if _, ok, _ := GetStream(db, id); ok {
			t.Errorf("CreateSharedStream(%q) created a row anyway", id)
		}
	}
	// An ordinary id is unaffected.
	if err := CreateSharedStream(db, "a1b2c3", "Kitchen"); err != nil {
		t.Fatalf("ordinary id rejected: %v", err)
	}
}
