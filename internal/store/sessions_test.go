package store

import "testing"

func TestSessionLifecycle(t *testing.T) {
	db := mustOpenMem(t)
	uid, _ := CreateUser(db, "a@b.com", "A", "h", true)
	if err := CreateSession(db, "hash-token", uid, 4_000_000_000); err != nil {
		t.Fatal(err)
	}
	got, ok, err := UserBySession(db, "hash-token")
	if err != nil || !ok || got.ID != uid {
		t.Fatalf("lookup: ok=%v err=%v id=%d", ok, err, got.ID)
	}
	if err := DeleteSession(db, "hash-token"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := UserBySession(db, "hash-token"); ok {
		t.Fatal("session survived delete")
	}
}

func TestExpiredSessionRejected(t *testing.T) {
	db := mustOpenMem(t)
	uid, _ := CreateUser(db, "a@b.com", "A", "h", false)
	if err := CreateSession(db, "old", uid, 100); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := UserBySession(db, "old"); ok {
		t.Fatal("expired session accepted")
	}
}
