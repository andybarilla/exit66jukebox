package store

import (
	"database/sql"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/model"

	_ "modernc.org/sqlite"
)

func TestOpenMigratesOldFederationSchemaBeforeCreatingIndexes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "exit66.db")
	oldDB, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open old db: %v", err)
	}
	if _, err := oldDB.Exec(`
		CREATE TABLE artist (
			id       INTEGER PRIMARY KEY AUTOINCREMENT,
			name     TEXT NOT NULL UNIQUE,
			sort_key TEXT NOT NULL DEFAULT '',
			mbid     TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE album (
			id        INTEGER PRIMARY KEY AUTOINCREMENT,
			name      TEXT NOT NULL,
			artist_id INTEGER NOT NULL REFERENCES artist(id),
			sort_key  TEXT NOT NULL DEFAULT '',
			cover     TEXT NOT NULL DEFAULT '',
			mbid      TEXT NOT NULL DEFAULT '',
			UNIQUE(name, artist_id)
		);
		CREATE TABLE track (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			path       TEXT NOT NULL,
			mod_time   INTEGER NOT NULL,
			size       INTEGER NOT NULL,
			title      TEXT NOT NULL,
			artist_id  INTEGER NOT NULL REFERENCES artist(id),
			album_id   INTEGER NOT NULL REFERENCES album(id),
			track_no   INTEGER NOT NULL DEFAULT 0,
			genre      TEXT NOT NULL DEFAULT '',
			duration   INTEGER NOT NULL DEFAULT 0,
			play_count INTEGER NOT NULL DEFAULT 0,
			added_at   INTEGER NOT NULL DEFAULT 0,
			mbid       TEXT NOT NULL DEFAULT '',
			links      TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE meta (key TEXT PRIMARY KEY, value INTEGER NOT NULL);
		INSERT INTO meta(key, value) VALUES('library_version', 1);
	`); err != nil {
		oldDB.Close()
		t.Fatalf("create old db: %v", err)
	}
	if err := oldDB.Close(); err != nil {
		t.Fatalf("close old db: %v", err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open old db: %v", err)
	}
	defer db.Close()

	for _, column := range []string{"source_peer", "source_library_id", "remote_id", "library_id"} {
		hasColumn, err := columnExists(db, "track", column)
		if err != nil {
			t.Fatalf("check track.%s: %v", column, err)
		}
		if !hasColumn {
			t.Fatalf("expected track.%s column", column)
		}
	}
	for _, index := range []string{"idx_track_remote_library", "idx_track_local_library_path"} {
		if !indexExists(t, db, index) {
			t.Fatalf("expected index %s", index)
		}
	}
	if !tableExists(t, db, "remote_library") {
		t.Fatalf("expected remote_library table")
	}
}

func TestOpenMigratesOldLocalLibraryBeforeCreatingSourceLibraryIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "exit66.db")
	oldDB, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open old db: %v", err)
	}
	if _, err := oldDB.Exec(`
		CREATE TABLE local_library (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			path       TEXT NOT NULL UNIQUE,
			enabled    INTEGER NOT NULL DEFAULT 1,
			name       TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);
		INSERT INTO local_library(path, enabled, name, created_at, updated_at)
		VALUES('/music', 1, 'Music', 1, 1);
	`); err != nil {
		oldDB.Close()
		t.Fatalf("create old local_library: %v", err)
	}
	if err := oldDB.Close(); err != nil {
		t.Fatalf("close old db: %v", err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open old db: %v", err)
	}
	defer db.Close()

	if has, err := columnExists(db, "local_library", "source_library_id"); err != nil || !has {
		t.Fatalf("expected local_library.source_library_id after Open, has=%v err=%v", has, err)
	}
	if !indexExists(t, db, "idx_local_library_source_library_id") {
		t.Fatalf("expected idx_local_library_source_library_id")
	}
	var sourceLibraryID string
	if err := db.QueryRow(`SELECT source_library_id FROM local_library WHERE path = '/music'`).Scan(&sourceLibraryID); err != nil {
		t.Fatalf("source library id: %v", err)
	}
	if sourceLibraryID == "" {
		t.Fatalf("expected source_library_id backfilled")
	}
}

func indexExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	return objectExists(t, db, "index", name)
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	return objectExists(t, db, "table", name)
}

func objectExists(t *testing.T, db *sql.DB, objectType, name string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = ? AND name = ?`, objectType, name).Scan(&count); err != nil {
		t.Fatalf("check %s %s: %v", objectType, name, err)
	}
	return count > 0
}

func TestMigrateAssignsExistingTracksToDefaultLibraries(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO artist(name) VALUES('A')`); err != nil {
		t.Fatalf("artist: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO album(name, artist_id) VALUES('X', 1)`); err != nil {
		t.Fatalf("album: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO track(path, mod_time, size, title, artist_id, album_id, source_peer, remote_id)
		VALUES('/m/a.mp3', 1, 1, 'Local', 1, 1, '', 0), ('fed://home/42', 0, 0, 'Remote', 1, 1, 'home', 42)`); err != nil {
		t.Fatalf("tracks: %v", err)
	}
	if _, err := db.Exec(`UPDATE track SET library_id = 0, source_library_id = ''`); err != nil {
		t.Fatalf("reset library identity: %v", err)
	}

	if err := migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var localLibraryID, remoteLibraryID int64
	if err := db.QueryRow(`SELECT library_id FROM track WHERE source_peer = ''`).Scan(&localLibraryID); err != nil {
		t.Fatalf("local library id: %v", err)
	}
	if localLibraryID == 0 {
		t.Fatalf("expected local track assigned to a default local library")
	}
	if err := db.QueryRow(`SELECT id FROM local_library WHERE id = ?`, localLibraryID).Scan(&localLibraryID); err != nil {
		t.Fatalf("default local library missing: %v", err)
	}
	var sourceLibraryID string
	if err := db.QueryRow(`SELECT library_id, source_library_id FROM track WHERE source_peer = 'home'`).Scan(&remoteLibraryID, &sourceLibraryID); err != nil {
		t.Fatalf("remote library identity: %v", err)
	}
	if remoteLibraryID == 0 || sourceLibraryID != DefaultRemoteSourceLibraryID {
		t.Fatalf("expected remote default library identity, got id=%d source=%q", remoteLibraryID, sourceLibraryID)
	}
}

func TestMigrateBackfillsOpaqueLocalLibraryIdentity(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	libraryID, err := EnsureLocalLibrary(db, "/music", "Music")
	if err != nil {
		t.Fatalf("library: %v", err)
	}
	if _, err := UpsertTrackInLibrary(db, libraryID, model.Track{Path: "/music/a.mp3", Title: "A"}, "Artist", "Artist", "Album"); err != nil {
		t.Fatalf("track: %v", err)
	}
	if _, err := db.Exec(`DROP INDEX IF EXISTS idx_local_library_source_library_id`); err != nil {
		t.Fatalf("drop source library index: %v", err)
	}
	if has, err := columnExists(db, "local_library", "source_library_id"); err != nil {
		t.Fatalf("check source library id: %v", err)
	} else if has {
		if _, err := db.Exec(`ALTER TABLE local_library DROP COLUMN source_library_id`); err != nil {
			t.Fatalf("drop source library id: %v", err)
		}
	}

	if err := migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if has, err := columnExists(db, "local_library", "source_library_id"); err != nil || !has {
		t.Fatalf("expected local_library.source_library_id after migrate, has=%v err=%v", has, err)
	}
	rows, err := ExportCatalog(db)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 exported track, got %d", len(rows))
	}
	if rows[0].SourceLibraryID == "" || rows[0].SourceLibraryID == strconv.FormatInt(libraryID, 10) {
		t.Fatalf("source_library_id should be backfilled as opaque identity, got row=%+v library_id=%d", rows[0], libraryID)
	}
}

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

func TestCurrentLibraryVersionBumpedForFolderCompilationHeuristic(t *testing.T) {
	if currentLibraryVersion != 5 {
		t.Fatalf("currentLibraryVersion = %d, want 5 for same-title folder compilation heuristic", currentLibraryVersion)
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

// An instance carrying the pre-#128 global personal stream must upgrade
// cleanly: the shared row and everything hanging off it go, because its queue
// was common to every user and there is nobody to migrate it to.
func TestMigrateDropsTheLegacyGlobalPersonalStream(t *testing.T) {
	db := openTestDB(t)
	tid := insertTestTrack(t, db, "/m/a.mp3")
	// Stand in for the row boot used to create, with a queue and a station.
	if err := EnsurePrivateStream(db, legacyPersonalStreamID); err != nil {
		t.Fatalf("seed legacy stream: %v", err)
	}
	if err := Enqueue(db, legacyPersonalStreamID, tid, "Alice"); err != nil {
		t.Fatalf("seed queue: %v", err)
	}
	if err := UpsertStation(db, Station{
		StreamID: legacyPersonalStreamID, Genre: "rock", Threshold: 3, Batch: 10,
	}); err != nil {
		t.Fatalf("seed station: %v", err)
	}

	if err := migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, ok, _ := GetStream(db, legacyPersonalStreamID); ok {
		t.Error("legacy personal stream row survived the migration")
	}
	if ids, _ := QueueTrackIDs(db, legacyPersonalStreamID); len(ids) != 0 {
		t.Errorf("legacy queue rows survived: %v", ids)
	}
	if _, ok := GetStation(db, legacyPersonalStreamID); ok {
		t.Error("legacy station survived")
	}
	// Idempotent: a second run on the now-clean DB is a no-op.
	if err := migrate(db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

// A shared stream an operator happened to name "me" is a real stream with
// listeners, not the legacy row, so the migration must leave it alone.
func TestMigrateKeepsASharedStreamNamedMe(t *testing.T) {
	db := openTestDB(t)
	if err := EnsureSharedStream(db, legacyPersonalStreamID, "Me Room"); err != nil {
		t.Fatalf("seed shared: %v", err)
	}
	if err := migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	st, ok, err := GetStream(db, legacyPersonalStreamID)
	if err != nil || !ok {
		t.Fatalf("shared stream named %q was dropped: ok=%v err=%v", legacyPersonalStreamID, ok, err)
	}
	if st.Kind != KindShared {
		t.Fatalf("kind = %q, want shared", st.Kind)
	}
}
