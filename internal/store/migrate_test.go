package store

import "testing"

func TestMigrateAddsAddedAtToOldDB(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	// Simulate an old DB: drop added_at by recreating track without it is hard
	// in-place, so instead verify the column exists and migrate is idempotent.
	has, err := columnExists(db, "track", "added_at")
	if err != nil {
		t.Fatalf("columnExists: %v", err)
	}
	if !has {
		t.Fatalf("expected added_at column to exist after Open")
	}

	// Running migrate again must be a no-op (idempotent).
	if err := migrate(db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestMigrateAddsColumnToTableMissingIt(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	// Simulate a pre-migration database: drop added_at so the ALTER branch runs.
	if _, err := db.Exec(`ALTER TABLE track DROP COLUMN added_at`); err != nil {
		t.Fatalf("drop column: %v", err)
	}
	if has, _ := columnExists(db, "track", "added_at"); has {
		t.Fatalf("precondition failed: column still present after drop")
	}
	// Seed a row whose mod_time should become the backfilled added_at.
	if _, err := db.Exec(`INSERT INTO artist(name) VALUES('A')`); err != nil {
		t.Fatalf("artist: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO album(name, artist_id) VALUES('X', 1)`); err != nil {
		t.Fatalf("album: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO track(path, mod_time, size, title, artist_id, album_id)
		 VALUES('/m/old.mp3', 777, 10, 'Old', 1, 1)`); err != nil {
		t.Fatalf("track: %v", err)
	}

	if err := migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if has, _ := columnExists(db, "track", "added_at"); !has {
		t.Fatalf("expected added_at column to be added by migrate")
	}
	var addedAt int64
	if err := db.QueryRow(`SELECT added_at FROM track WHERE path='/m/old.mp3'`).Scan(&addedAt); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if addedAt != 777 {
		t.Fatalf("expected added_at backfilled to mod_time 777, got %d", addedAt)
	}
}

func TestMigrateAddsMbidColumns(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	// Simulate a pre-#38 database: drop mbid from each table so the ALTER runs.
	for _, tbl := range []string{"artist", "album", "track"} {
		if _, err := db.Exec("ALTER TABLE " + tbl + " DROP COLUMN mbid"); err != nil {
			t.Fatalf("drop mbid from %s: %v", tbl, err)
		}
		if has, _ := columnExists(db, tbl, "mbid"); has {
			t.Fatalf("precondition failed: %s.mbid still present after drop", tbl)
		}
	}

	if err := migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, tbl := range []string{"artist", "album", "track"} {
		if has, _ := columnExists(db, tbl, "mbid"); !has {
			t.Fatalf("expected %s.mbid added by migrate", tbl)
		}
	}

	// Re-running migrate must be a no-op.
	if err := migrate(db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestMigrateBackfillsFromModTime(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	// Insert a track, then zero its added_at to mimic a pre-migration row.
	if _, err := db.Exec(`INSERT INTO artist(name) VALUES('A')`); err != nil {
		t.Fatalf("artist: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO album(name, artist_id) VALUES('X', 1)`); err != nil {
		t.Fatalf("album: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO track(path, mod_time, size, title, artist_id, album_id, added_at)
		 VALUES('/m/a.mp3', 555, 10, 'A', 1, 1, 0)`); err != nil {
		t.Fatalf("track: %v", err)
	}

	if err := migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var addedAt int64
	if err := db.QueryRow(`SELECT added_at FROM track WHERE path='/m/a.mp3'`).Scan(&addedAt); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if addedAt != 555 {
		t.Fatalf("expected added_at backfilled to mod_time 555, got %d", addedAt)
	}
}
