package store

import (
	"database/sql"
	"testing"
)

func mustOpenMem(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestUserCreateGetCount(t *testing.T) {
	db := mustOpenMem(t)
	if n, _ := CountUsers(db); n != 0 {
		t.Fatalf("want 0 users, got %d", n)
	}
	id, err := CreateUser(db, "a@b.com", "Alice", "hash1", true)
	if err != nil {
		t.Fatal(err)
	}
	u, ok, err := GetUserByEmail(db, "a@b.com")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if u.ID != id || u.DisplayName != "Alice" || !u.IsAdmin || u.PasswordHash != "hash1" {
		t.Fatalf("bad user: %+v", u)
	}
	if n, _ := CountUsers(db); n != 1 {
		t.Fatalf("want 1 user, got %d", n)
	}
}

func TestUserDuplicateEmail(t *testing.T) {
	db := mustOpenMem(t)
	if _, err := CreateUser(db, "a@b.com", "A", "h", false); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateUser(db, "a@b.com", "A2", "h", false); err == nil {
		t.Fatal("duplicate email allowed")
	}
}

func TestListAndDeleteUser(t *testing.T) {
	db := mustOpenMem(t)
	id, _ := CreateUser(db, "a@b.com", "A", "h", true)
	if us, _ := ListUsers(db); len(us) != 1 {
		t.Fatalf("want 1, got %d", len(us))
	}
	if err := DeleteUser(db, id); err != nil {
		t.Fatal(err)
	}
	if n, _ := CountUsers(db); n != 0 {
		t.Fatalf("want 0 after delete, got %d", n)
	}
}
