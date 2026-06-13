package store

import (
	"path/filepath"
	"testing"
)

// TestScrobbleQueueCreatedAndIdempotent opens a fresh DB, confirms the
// scrobble_queue table exists, then reopens the same file to prove the schema
// is safe to re-apply on an existing database and that rows survive a restart.
func TestScrobbleQueueCreatedAndIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scrobble.db")

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	var n int
	if err := db.QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='scrobble_queue'`).Scan(&n); err != nil {
		t.Fatalf("query master: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected scrobble_queue table to exist, found %d", n)
	}
	if _, err := db.Exec(
		`INSERT INTO scrobble_queue(service, track_id, played_at, created_at)
		 VALUES('listenbrainz', 5, 1000, 1000)`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	db.Close()

	// Reopen the same file: Open re-applies the schema (idempotent) and the row
	// from before the "restart" must still be there.
	db2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()
	var count int
	if err := db2.QueryRow(`SELECT count(*) FROM scrobble_queue`).Scan(&count); err != nil {
		t.Fatalf("count after reopen: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row to survive restart, got %d", count)
	}
}
