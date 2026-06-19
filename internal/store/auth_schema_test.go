package store

import "testing"

func TestAuthTablesExist(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	for _, tbl := range []string{"user", "session", "invite", "email_verification"} {
		var n int
		err := db.QueryRow(
			`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, tbl,
		).Scan(&n)
		if err != nil || n != 1 {
			t.Fatalf("table %q missing (n=%d err=%v)", tbl, n, err)
		}
	}
}

func TestExistingUsersAreBackfilledVerified(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`UPDATE user SET email_verified_at = 0`); err != nil {
		t.Fatalf("seed unverified state: %v", err)
	}
	if _, err := CreateUser(db, "old@example.com", "Old", "h", false, false); err != nil {
		t.Fatalf("seed unverified user: %v", err)
	}
	if _, err := db.Exec(`DROP TABLE email_verification`); err != nil {
		t.Fatalf("simulate old schema: %v", err)
	}
	if err := migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	u, ok, err := GetUserByEmail(db, "old@example.com")
	if err != nil || !ok {
		t.Fatalf("load user: ok=%v err=%v", ok, err)
	}
	if u.EmailVerifiedAt == 0 {
		t.Fatal("existing user was not backfilled as verified")
	}
}
