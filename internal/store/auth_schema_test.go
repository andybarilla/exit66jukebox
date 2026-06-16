package store

import "testing"

func TestAuthTablesExist(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	for _, tbl := range []string{"user", "session", "invite"} {
		var n int
		err := db.QueryRow(
			`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, tbl,
		).Scan(&n)
		if err != nil || n != 1 {
			t.Fatalf("table %q missing (n=%d err=%v)", tbl, n, err)
		}
	}
}
