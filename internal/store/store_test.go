package store

import "testing"

func TestOpenRunsMigrations(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	var n int
	err = db.QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='track'`,
	).Scan(&n)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected track table to exist, got count %d", n)
	}
}
