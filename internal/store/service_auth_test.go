package store

import (
	"path/filepath"
	"testing"
)

func TestServiceAuthPutGetDelete(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if _, _, ok, err := GetServiceAuth(db, "lastfm"); err != nil || ok {
		t.Fatalf("GetServiceAuth on empty = (ok=%v, err=%v), want (false, nil)", ok, err)
	}

	if err := PutServiceAuth(db, "lastfm", "sess-key", "alice"); err != nil {
		t.Fatalf("PutServiceAuth: %v", err)
	}
	key, user, ok, err := GetServiceAuth(db, "lastfm")
	if err != nil || !ok {
		t.Fatalf("GetServiceAuth = (ok=%v, err=%v), want (true, nil)", ok, err)
	}
	if key != "sess-key" || user != "alice" {
		t.Errorf("got (%q, %q), want (sess-key, alice)", key, user)
	}

	// Upsert: a second Put for the same service replaces the row.
	if err := PutServiceAuth(db, "lastfm", "sess-2", "bob"); err != nil {
		t.Fatalf("PutServiceAuth upsert: %v", err)
	}
	key, user, _, _ = GetServiceAuth(db, "lastfm")
	if key != "sess-2" || user != "bob" {
		t.Errorf("after upsert got (%q, %q), want (sess-2, bob)", key, user)
	}

	if err := DeleteServiceAuth(db, "lastfm"); err != nil {
		t.Fatalf("DeleteServiceAuth: %v", err)
	}
	if _, _, ok, _ := GetServiceAuth(db, "lastfm"); ok {
		t.Error("GetServiceAuth after delete = ok, want gone")
	}
}

// The session key must survive a restart so auth is one-time.
func TestServiceAuthSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := PutServiceAuth(db, "lastfm", "persisted", "carol"); err != nil {
		t.Fatalf("PutServiceAuth: %v", err)
	}
	db.Close()

	db2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()
	key, user, ok, err := GetServiceAuth(db2, "lastfm")
	if err != nil || !ok {
		t.Fatalf("after restart = (ok=%v, err=%v), want (true, nil)", ok, err)
	}
	if key != "persisted" || user != "carol" {
		t.Errorf("after restart got (%q, %q), want (persisted, carol)", key, user)
	}
}
