package store

import "database/sql"

// currentLibraryVersion is bumped whenever the indexing rules change in a way
// that stored columns can't re-derive, forcing a one-time full re-scan. v1:
// albums re-keyed by album-artist (#32). v2: compilation flag forces "Various
// Artists" when AlbumArtist is blank (#63). v3: comment-tag links extracted (#46).
const currentLibraryVersion = 4

const DefaultRemoteSourceLibraryID = "default"

// columnExists reports whether a column is present on a table.
func columnExists(db *sql.DB, table, col string) (bool, error) {
	var n int
	err := db.QueryRow(
		`SELECT count(*) FROM pragma_table_info(?) WHERE name = ?`, table, col,
	).Scan(&n)
	return n > 0, err
}

// migrate brings an existing database up to the current schema. It is
// idempotent: safe to run on every Open. CREATE TABLE IF NOT EXISTS in
// schema.sql cannot add columns to a pre-existing table, so additive column
// changes are applied here.
func migrate(db *sql.DB) error {
	has, err := columnExists(db, "track", "added_at")
	if err != nil {
		return err
	}
	if !has {
		if _, err := db.Exec(`ALTER TABLE track ADD COLUMN added_at INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	// Backfill any rows that predate added_at (value 0) from mod_time.
	if _, err := db.Exec(`UPDATE track SET added_at = mod_time WHERE added_at = 0`); err != nil {
		return err
	}
	// mbid columns for MusicBrainz enrichment (#38). Table names are a fixed
	// local list, not user input; ALTER TABLE cannot use ? for identifiers.
	for _, t := range []string{"artist", "album", "track"} {
		has, err := columnExists(db, t, "mbid")
		if err != nil {
			return err
		}
		if !has {
			if _, err := db.Exec("ALTER TABLE " + t + " ADD COLUMN mbid TEXT NOT NULL DEFAULT ''"); err != nil {
				return err
			}
		}
	}
	// links column holds newline-joined comment-tag URLs (#46). The version bump
	// below forces a re-scan that backfills it for already-indexed libraries.
	has, err = columnExists(db, "track", "links")
	if err != nil {
		return err
	}
	if !has {
		if _, err := db.Exec(`ALTER TABLE track ADD COLUMN links TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	// sort_key columns drive backend-owned library ordering (#53). Add and backfill
	// for any artist/album rows that predate the column or were left blank.
	for _, t := range []string{"artist", "album"} {
		has, err := columnExists(db, t, "sort_key")
		if err != nil {
			return err
		}
		if !has {
			if _, err := db.Exec("ALTER TABLE " + t + " ADD COLUMN sort_key TEXT NOT NULL DEFAULT ''"); err != nil {
				return err
			}
		}
		if err := backfillSortKeys(db, t); err != nil {
			return err
		}
	}
	// Federation: remote rows carry their owning peer and the track's id on that
	// peer. Local rows leave both empty/0 (#86). path is "" for remote tracks, so
	// the path-unique index can't key them — a partial unique index on
	// (source_peer, remote_id) covers remote rows instead (added in schema.sql).
	if has, err := columnExists(db, "track", "source_peer"); err != nil {
		return err
	} else if !has {
		if _, err := db.Exec(`ALTER TABLE track ADD COLUMN source_peer TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	if has, err := columnExists(db, "track", "remote_id"); err != nil {
		return err
	} else if !has {
		if _, err := db.Exec(`ALTER TABLE track ADD COLUMN remote_id INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	if has, err := columnExists(db, "track", "source_library_id"); err != nil {
		return err
	} else if !has {
		if _, err := db.Exec(`ALTER TABLE track ADD COLUMN source_library_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	if has, err := columnExists(db, "track", "library_id"); err != nil {
		return err
	} else if !has {
		if _, err := db.Exec(`ALTER TABLE track ADD COLUMN library_id INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	if _, err := db.Exec(
		`CREATE TABLE IF NOT EXISTS remote_library (
			id                INTEGER PRIMARY KEY AUTOINCREMENT,
			source_peer       TEXT NOT NULL,
			source_library_id TEXT NOT NULL,
			name              TEXT NOT NULL DEFAULT '',
			created_at        INTEGER NOT NULL,
			updated_at        INTEGER NOT NULL,
			UNIQUE(source_peer, source_library_id)
		)`,
	); err != nil {
		return err
	}
	if err := backfillTrackLibraries(db); err != nil {
		return err
	}
	if _, err := db.Exec(`DROP INDEX IF EXISTS idx_track_remote`); err != nil {
		return err
	}
	if _, err := db.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_track_remote_library
		 ON track(source_peer, source_library_id, remote_id) WHERE source_peer <> ''`,
	); err != nil {
		return err
	}
	if _, err := db.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_track_local_library_path
		 ON track(library_id, path) WHERE source_peer = ''`,
	); err != nil {
		return err
	}
	return migrateLibraryVersion(db)
}

func backfillTrackLibraries(db *sql.DB) error {
	var localTracksNeedingLibrary int
	if err := db.QueryRow(`SELECT count(*) FROM track WHERE source_peer = '' AND library_id = 0`).Scan(&localTracksNeedingLibrary); err != nil {
		return err
	}
	if localTracksNeedingLibrary > 0 {
		defaultLocalLibraryID, err := EnsureLocalLibrary(db, "", "Default Library")
		if err != nil {
			return err
		}
		if _, err := db.Exec(
			`UPDATE track SET library_id = ? WHERE source_peer = '' AND library_id = 0`, defaultLocalLibraryID,
		); err != nil {
			return err
		}
	}
	rows, err := db.Query(`SELECT DISTINCT source_peer FROM track WHERE source_peer <> ''`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var peers []string
	for rows.Next() {
		var peer string
		if err := rows.Scan(&peer); err != nil {
			return err
		}
		peers = append(peers, peer)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, peer := range peers {
		remoteLibraryID, err := EnsureRemoteLibrary(db, peer, DefaultRemoteSourceLibraryID, "Default Library")
		if err != nil {
			return err
		}
		if _, err := db.Exec(
			`UPDATE track SET library_id = ?, source_library_id = ?
			 WHERE source_peer = ? AND (library_id = 0 OR source_library_id = '')`,
			remoteLibraryID, DefaultRemoteSourceLibraryID, peer,
		); err != nil {
			return err
		}
	}
	return nil
}

// migrateLibraryVersion forces a one-time full re-scan when the stored
// library_version is behind currentLibraryVersion. Stored columns can't
// re-derive album-artist grouping, so it zeroes every track's mod_time/size:
// the next scan re-reads all files and re-points each track to its
// album-artist-keyed album. Orphaned per-track-artist albums are pruned after
// that scan (see PruneOrphans). A fresh DB has no tracks, so the UPDATE is a
// no-op and the version is simply stamped.
func migrateLibraryVersion(db *sql.DB) error {
	var v int
	// Missing row scans nothing and leaves v at 0 (older than any real version).
	db.QueryRow(`SELECT value FROM meta WHERE key = 'library_version'`).Scan(&v)
	if v >= currentLibraryVersion {
		return nil
	}
	if _, err := db.Exec(`UPDATE track SET mod_time = 0, size = 0`); err != nil {
		return err
	}
	_, err := db.Exec(
		`INSERT INTO meta(key, value) VALUES('library_version', ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		currentLibraryVersion)
	return err
}

// backfillSortKeys recomputes sort_key in Go for every row in table whose key is
// still blank. Table is a fixed local name, never user input.
func backfillSortKeys(db *sql.DB, table string) error {
	rows, err := db.Query("SELECT id, name FROM " + table + " WHERE sort_key = ''")
	if err != nil {
		return err
	}
	defer rows.Close()
	type row struct {
		id   int64
		name string
	}
	var todo []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.name); err != nil {
			return err
		}
		todo = append(todo, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, r := range todo {
		if _, err := db.Exec("UPDATE "+table+" SET sort_key = ? WHERE id = ?",
			normalizeSortKey(r.name), r.id); err != nil {
			return err
		}
	}
	return nil
}
