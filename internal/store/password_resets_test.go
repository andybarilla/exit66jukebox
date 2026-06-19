package store

import (
	"testing"
	"time"
)

func TestPasswordResetTokenIsSingleUseAndExpires(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	userID, err := CreateUser(db, "reset@example.com", "Reset", "old-hash", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	expiresAt := time.Now().Add(time.Hour).Unix()

	id, err := CreatePasswordReset(db, "token-hash", userID, expiresAt)
	if err != nil {
		t.Fatalf("CreatePasswordReset: %v", err)
	}

	reset, ok, err := PendingPasswordReset(db, "token-hash")
	if err != nil {
		t.Fatalf("PendingPasswordReset: %v", err)
	}
	if !ok || reset.ID != id || reset.UserID != userID {
		t.Fatalf("pending reset mismatch: ok=%v reset=%+v", ok, reset)
	}

	if err := MarkPasswordResetUsed(db, id); err != nil {
		t.Fatalf("MarkPasswordResetUsed: %v", err)
	}
	if _, ok, err := PendingPasswordReset(db, "token-hash"); err != nil || ok {
		t.Fatalf("used reset should not be pending: ok=%v err=%v", ok, err)
	}

	_, err = CreatePasswordReset(db, "expired-token-hash", userID, time.Now().Add(-time.Minute).Unix())
	if err != nil {
		t.Fatalf("CreatePasswordReset expired: %v", err)
	}
	if _, ok, err := PendingPasswordReset(db, "expired-token-hash"); err != nil || ok {
		t.Fatalf("expired reset should not be pending: ok=%v err=%v", ok, err)
	}
}

func TestConsumePasswordResetTokenIsConditionalSingleUse(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	userID, err := CreateUser(db, "reset@example.com", "Reset", "old-hash", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	_, err = CreatePasswordReset(db, "token-hash", userID, time.Now().Add(time.Hour).Unix())
	if err != nil {
		t.Fatalf("CreatePasswordReset: %v", err)
	}

	reset, ok, err := ConsumePasswordReset(db, "token-hash")
	if err != nil {
		t.Fatalf("ConsumePasswordReset first: %v", err)
	}
	if !ok || reset.UserID != userID {
		t.Fatalf("first consume mismatch: ok=%v reset=%+v", ok, reset)
	}
	if _, ok, err := ConsumePasswordReset(db, "token-hash"); err != nil || ok {
		t.Fatalf("second consume should fail: ok=%v err=%v", ok, err)
	}
}
