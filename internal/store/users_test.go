package store

import (
	"database/sql"
	"errors"
	"sync"
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

func TestCreateUserCanStoreUnverifiedAndVerifiedAccounts(t *testing.T) {
	db := mustOpenMem(t)
	unverifiedID, err := CreateUser(db, "open@example.com", "Open", "h", false, false)
	if err != nil {
		t.Fatalf("create unverified: %v", err)
	}
	verifiedID, err := CreateUser(db, "invite@example.com", "Invite", "h", false, true)
	if err != nil {
		t.Fatalf("create verified: %v", err)
	}
	unverified, _, _ := GetUserByID(db, unverifiedID)
	verified, _, _ := GetUserByID(db, verifiedID)
	if unverified.EmailVerifiedAt != 0 {
		t.Fatalf("open signup user should start unverified, got %d", unverified.EmailVerifiedAt)
	}
	if verified.EmailVerifiedAt == 0 {
		t.Fatal("invited/bootstrap user should start verified")
	}
}

func TestCreateFirstAdminOnlyWinsOnEmptyUserTable(t *testing.T) {
	db := mustOpenMem(t)
	id, err := CreateFirstAdmin(db, "admin@example.com", "Admin", "h")
	if err != nil {
		t.Fatalf("create first admin: %v", err)
	}
	user, ok, err := GetUserByID(db, id)
	if err != nil || !ok {
		t.Fatalf("load first admin: ok=%v err=%v", ok, err)
	}
	if !user.IsAdmin || user.EmailVerifiedAt == 0 {
		t.Fatalf("first admin not admin/verified: %+v", user)
	}
	if _, err := CreateFirstAdmin(db, "second@example.com", "Second", "h"); !errors.Is(err, ErrBootstrapAlreadyClaimed) {
		t.Fatalf("second first-admin attempt: want claimed, got %v", err)
	}
}

func TestCreateFirstAdminConcurrentRaceCreatesOneAdmin(t *testing.T) {
	db := mustOpenMem(t)
	const attempts = 12
	var wg sync.WaitGroup
	results := make(chan error, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := CreateFirstAdmin(db, string(rune('a'+i))+"@example.com", "Admin", "h")
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)

	winners := 0
	claimed := 0
	for err := range results {
		if err == nil {
			winners++
			continue
		}
		if errors.Is(err, ErrBootstrapAlreadyClaimed) {
			claimed++
			continue
		}
		t.Fatalf("unexpected create error: %v", err)
	}
	if winners != 1 || claimed != attempts-1 {
		t.Fatalf("race results: winners=%d claimed=%d", winners, claimed)
	}
	users, err := ListUsers(db)
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(users) != 1 || !users[0].IsAdmin {
		t.Fatalf("want one admin, got %+v", users)
	}
}

func TestCreateUnverifiedUserWithEmailVerificationRollsBackWhenTokenInsertFails(t *testing.T) {
	db := mustOpenMem(t)
	_, err := db.Exec(`CREATE TRIGGER fail_email_verification_insert BEFORE INSERT ON email_verification BEGIN SELECT RAISE(FAIL, 'token insert failed'); END`)
	if err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	_, _, err = CreateUnverifiedUserWithEmailVerification(db, "open@example.com", "Open", "h", 4_000_000_000)
	if err == nil {
		t.Fatal("expected token insert failure")
	}
	if _, ok, err := GetUserByEmail(db, "open@example.com"); err != nil || ok {
		t.Fatalf("user was not rolled back: ok=%v err=%v", ok, err)
	}
}

func TestEmailVerificationTokenLifecycle(t *testing.T) {
	db := mustOpenMem(t)
	userID, err := CreateUser(db, "verify@example.com", "Verify", "h", false, false)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	raw, err := CreateEmailVerification(db, userID, 4_000_000_000)
	if err != nil {
		t.Fatalf("create verification: %v", err)
	}
	if raw == "" {
		t.Fatal("raw token was empty")
	}
	var storedRaw int
	if err := db.QueryRow(`SELECT count(*) FROM email_verification WHERE token_hash = ?`, raw).Scan(&storedRaw); err != nil {
		t.Fatalf("check raw storage: %v", err)
	}
	if storedRaw != 0 {
		t.Fatal("raw verification token was stored")
	}
	verification, ok, err := PendingEmailVerification(db, raw)
	if err != nil || !ok {
		t.Fatalf("pending verification: ok=%v err=%v", ok, err)
	}
	if verification.UserID != userID {
		t.Fatalf("pending user id: want %d, got %d", userID, verification.UserID)
	}
	if err := ConsumeEmailVerification(db, raw); err != nil {
		t.Fatalf("consume verification: %v", err)
	}
	user, _, _ := GetUserByID(db, userID)
	if user.EmailVerifiedAt == 0 {
		t.Fatal("consume did not mark user verified")
	}
	if _, ok, err := PendingEmailVerification(db, raw); err != nil || ok {
		t.Fatalf("used token remained pending: ok=%v err=%v", ok, err)
	}
}

func TestEmailVerificationRejectsExpiredMalformedAndForgedTokens(t *testing.T) {
	db := mustOpenMem(t)
	userID, err := CreateUser(db, "expired@example.com", "Expired", "h", false, false)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	expired, err := CreateEmailVerification(db, userID, 1)
	if err != nil {
		t.Fatalf("create expired verification: %v", err)
	}
	for _, token := range []string{expired, "not-a-real-token", ""} {
		if _, ok, err := PendingEmailVerification(db, token); err != nil || ok {
			t.Fatalf("token %q should not resolve: ok=%v err=%v", token, ok, err)
		}
	}
}

func TestRegenerateEmailVerificationReplacesPendingToken(t *testing.T) {
	db := mustOpenMem(t)
	userID, err := CreateUser(db, "manual@example.com", "Manual", "h", false, false)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	oldToken, _ := CreateEmailVerification(db, userID, 4_000_000_000)
	newToken, err := RegenerateEmailVerification(db, userID, 4_000_000_000)
	if err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	if oldToken == newToken {
		t.Fatal("regenerated token did not change")
	}
	if _, ok, _ := PendingEmailVerification(db, oldToken); ok {
		t.Fatal("old token still pending after regeneration")
	}
	if _, ok, err := PendingEmailVerification(db, newToken); err != nil || !ok {
		t.Fatalf("new token not pending: ok=%v err=%v", ok, err)
	}
}
