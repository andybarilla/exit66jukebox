package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// busy_timeout must be set on every pooled connection, not just the one the
// schema PRAGMAs happen to run on. Otherwise a second connection (e.g. the
// scan's PruneOrphans racing startup writes) gets SQLITE_BUSY immediately
// instead of waiting, surfacing as "database is locked (5)".
func TestOpenSetsBusyTimeoutOnAllConns(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	// Hold several connections open at once so the pool is forced to create
	// distinct connections rather than reusing one.
	const n = 4
	conns := make([]*sql.Conn, 0, n)
	for i := 0; i < n; i++ {
		c, err := db.Conn(ctx)
		if err != nil {
			t.Fatalf("Conn %d: %v", i, err)
		}
		defer c.Close()
		conns = append(conns, c)
	}
	for i, c := range conns {
		var to int
		if err := c.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&to); err != nil {
			t.Fatalf("conn %d PRAGMA busy_timeout: %v", i, err)
		}
		if to != 5000 {
			t.Fatalf("conn %d busy_timeout = %d, want 5000", i, to)
		}
	}
}
