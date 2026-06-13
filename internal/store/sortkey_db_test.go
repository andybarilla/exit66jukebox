package store

import (
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/model"
)

func TestUpsertPopulatesSortKey(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	UpsertTrack(db, model.Track{Path: "/m/1.mp3", Title: "Come Together", TrackNo: 1}, "The Beatles", "Abbey Road")

	var albumKey, artistKey string
	db.QueryRow(`SELECT sort_key FROM album WHERE name = 'Abbey Road'`).Scan(&albumKey)
	db.QueryRow(`SELECT sort_key FROM artist WHERE name = 'The Beatles'`).Scan(&artistKey)

	if albumKey != "abbey road" {
		t.Errorf("album sort_key = %q, want %q", albumKey, "abbey road")
	}
	if artistKey != "beatles" {
		t.Errorf("artist sort_key = %q, want %q", artistKey, "beatles")
	}
}

func TestMigrateBackfillsSortKey(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	// Simulate pre-migration rows: insert directly, then blank the sort_key as an
	// older schema would have left it.
	db.Exec(`INSERT INTO artist(name, sort_key) VALUES('A Perfect Circle', '')`)
	db.Exec(`INSERT INTO album(name, artist_id, sort_key) VALUES('The Thirteenth Step', 1, '')`)

	if err := migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var albumKey, artistKey string
	db.QueryRow(`SELECT sort_key FROM album WHERE name = 'The Thirteenth Step'`).Scan(&albumKey)
	db.QueryRow(`SELECT sort_key FROM artist WHERE name = 'A Perfect Circle'`).Scan(&artistKey)
	if albumKey != "thirteenth step" {
		t.Errorf("backfilled album sort_key = %q, want %q", albumKey, "thirteenth step")
	}
	if artistKey != "perfect circle" {
		t.Errorf("backfilled artist sort_key = %q, want %q", artistKey, "perfect circle")
	}
}
