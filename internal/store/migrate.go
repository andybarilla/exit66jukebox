package store

import "database/sql"

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
	return nil
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
