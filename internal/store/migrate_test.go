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

// TestMigrateForcesRescanOnVersionBump verifies a stored library_version behind
// currentLibraryVersion zeroes every track's mod_time/size (so the next scan
// re-reads all files) and stamps the new version.
func TestMigrateForcesRescanOnVersionBump(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	// Seed an indexed track and rewind the library version to mimic a pre-#32 DB.
	if _, err := db.Exec(`INSERT INTO artist(name) VALUES('A')`); err != nil {
		t.Fatalf("artist: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO album(name, artist_id) VALUES('X', 1)`); err != nil {
		t.Fatalf("album: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO track(path, mod_time, size, title, artist_id, album_id)
		 VALUES('/m/a.mp3', 111, 222, 'A', 1, 1)`); err != nil {
		t.Fatalf("track: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM meta WHERE key = 'library_version'`); err != nil {
		t.Fatalf("reset version: %v", err)
	}

	if err := migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var mt, sz int64
	db.QueryRow(`SELECT mod_time, size FROM track WHERE path='/m/a.mp3'`).Scan(&mt, &sz)
	if mt != 0 || sz != 0 {
		t.Fatalf("expected mod_time/size zeroed to force re-scan, got %d/%d", mt, sz)
	}
	var v int
	db.QueryRow(`SELECT value FROM meta WHERE key='library_version'`).Scan(&v)
	if v != currentLibraryVersion {
		t.Fatalf("expected library_version stamped to %d, got %d", currentLibraryVersion, v)
	}

	// Re-running migrate is a no-op: the stamp prevents re-zeroing.
	if _, err := db.Exec(`UPDATE track SET mod_time=999, size=999`); err != nil {
		t.Fatalf("restamp: %v", err)
	}
	if err := migrate(db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	db.QueryRow(`SELECT mod_time FROM track WHERE path='/m/a.mp3'`).Scan(&mt)
	if mt != 999 {
		t.Fatalf("expected second migrate to leave mod_time untouched, got %d", mt)
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
