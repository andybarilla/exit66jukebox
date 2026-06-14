package store

import (
	"database/sql"
	_ "embed"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

// Open opens (or creates) the SQLite database at path and applies the schema.
// Pass ":memory:" for an ephemeral test database.
func Open(path string) (*sql.DB, error) {
	// Pragmas go in the DSN so modernc applies them to EVERY pooled connection.
	// Setting them via db.Exec only touches one connection; a second connection
	// (e.g. the scan's PruneOrphans racing startup writes) would then have
	// busy_timeout=0 and fail instantly with "database is locked (5)".
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	if path == ":memory:" {
		// WAL is meaningless for an in-memory db; keep the rest.
		dsn = "file::memory:?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// modernc :memory: lives per-connection; pin to one so tests see the schema.
	if path == ":memory:" {
		db.SetMaxOpenConns(1)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
