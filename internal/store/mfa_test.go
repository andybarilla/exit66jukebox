package store

import (
	"database/sql"
	"reflect"
	"testing"
	"time"
)

func mustCreateMFAUser(t *testing.T, db *sql.DB, email string) int64 {
	t.Helper()
	userID, err := CreateUser(db, email, "MFA User", "hash", false)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return userID
}

func TestMFAFactorLifecycle(t *testing.T) {
	db := mustOpenMem(t)
	userID := mustCreateMFAUser(t, db, "factor@example.com")

	if factor, ok, err := GetMFAFactor(db, userID); err != nil || ok || !reflect.DeepEqual(factor, MFAFactor{}) {
		t.Fatalf("empty factor = %+v, %v, %v; want zero, false, nil", factor, ok, err)
	}

	factor := MFAFactor{
		UserID:           userID,
		SecretCiphertext: []byte("ciphertext-v1"),
		SecretNonce:      []byte("nonce-v1"),
		KeyVersion:       7,
		EnabledAt:        123,
		LastAcceptedStep: -1,
	}
	if err := UpsertMFAFactor(db, factor); err != nil {
		t.Fatalf("upsert factor: %v", err)
	}

	got, ok, err := GetMFAFactor(db, userID)
	if err != nil || !ok {
		t.Fatalf("get factor: ok=%v err=%v", ok, err)
	}
	if !reflect.DeepEqual(got, factor) {
		t.Fatalf("factor = %+v, want %+v", got, factor)
	}

	updated := MFAFactor{
		UserID:           userID,
		SecretCiphertext: []byte("ciphertext-v2"),
		SecretNonce:      []byte("nonce-v2"),
		KeyVersion:       8,
		EnabledAt:        0,
		LastAcceptedStep: 42,
	}
	if err := UpsertMFAFactor(db, updated); err != nil {
		t.Fatalf("upsert updated factor: %v", err)
	}
	got, ok, err = GetMFAFactor(db, userID)
	if err != nil || !ok || !reflect.DeepEqual(got, updated) {
		t.Fatalf("updated factor = %+v, %v, %v; want %+v, true, nil", got, ok, err, updated)
	}

	if err := DisableMFAFactor(db, userID); err != nil {
		t.Fatalf("disable factor: %v", err)
	}
	if factor, ok, err := GetMFAFactor(db, userID); err != nil || ok || !reflect.DeepEqual(factor, MFAFactor{}) {
		t.Fatalf("disabled factor = %+v, %v, %v; want zero, false, nil", factor, ok, err)
	}
}

func TestMFALastAcceptedStepOnlyAdvances(t *testing.T) {
	db := mustOpenMem(t)
	userID := mustCreateMFAUser(t, db, "step@example.com")

	advanced, err := UpdateMFALastAcceptedStep(db, userID, 10)
	if err != nil || advanced {
		t.Fatalf("missing factor update = %v, %v; want false, nil", advanced, err)
	}

	factor := MFAFactor{
		UserID:           userID,
		SecretCiphertext: []byte("ciphertext"),
		SecretNonce:      []byte("nonce"),
		KeyVersion:       1,
		EnabledAt:        1,
		LastAcceptedStep: 20,
	}
	if err := UpsertMFAFactor(db, factor); err != nil {
		t.Fatalf("upsert factor: %v", err)
	}

	cases := []struct {
		step     int64
		advanced bool
	}{
		{step: 20, advanced: false},
		{step: 19, advanced: false},
		{step: 21, advanced: true},
	}
	for _, tc := range cases {
		advanced, err := UpdateMFALastAcceptedStep(db, userID, tc.step)
		if err != nil || advanced != tc.advanced {
			t.Fatalf("update step %d = %v, %v; want %v, nil", tc.step, advanced, err, tc.advanced)
		}
	}

	got, ok, err := GetMFAFactor(db, userID)
	if err != nil || !ok {
		t.Fatalf("get factor: ok=%v err=%v", ok, err)
	}
	if got.LastAcceptedStep != 21 {
		t.Fatalf("last accepted step = %d, want 21", got.LastAcceptedStep)
	}
}

func TestMFATicketConsumeSingleUseAndExpiry(t *testing.T) {
	db := mustOpenMem(t)
	userID := mustCreateMFAUser(t, db, "ticket@example.com")
	now := time.Now().Unix()

	if err := CreateMFATicket(db, "ticket-live", userID, now+60); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	gotUserID, ok, err := ConsumeMFATicket(db, "ticket-live")
	if err != nil || !ok || gotUserID != userID {
		t.Fatalf("consume = %d, %v, %v; want %d, true, nil", gotUserID, ok, err, userID)
	}
	gotUserID, ok, err = ConsumeMFATicket(db, "ticket-live")
	if err != nil || ok || gotUserID != 0 {
		t.Fatalf("reuse = %d, %v, %v; want 0, false, nil", gotUserID, ok, err)
	}

	if err := CreateMFATicket(db, "ticket-expired", userID, now-1); err != nil {
		t.Fatalf("create expired ticket: %v", err)
	}
	gotUserID, ok, err = ConsumeMFATicket(db, "ticket-expired")
	if err != nil || ok || gotUserID != 0 {
		t.Fatalf("expired = %d, %v, %v; want 0, false, nil", gotUserID, ok, err)
	}
}

func TestMFARecoveryCodeReplaceListAndUse(t *testing.T) {
	db := mustOpenMem(t)
	userID := mustCreateMFAUser(t, db, "recovery@example.com")

	if err := ReplaceRecoveryCodes(db, userID, []string{"old-a", "old-b"}); err != nil {
		t.Fatalf("replace old codes: %v", err)
	}
	if err := ReplaceRecoveryCodes(db, userID, []string{"new-a", "new-b", "new-c"}); err != nil {
		t.Fatalf("replace new codes: %v", err)
	}

	hashes, err := ListRecoveryCodeHashes(db, userID)
	if err != nil {
		t.Fatalf("list codes: %v", err)
	}
	if want := []string{"new-a", "new-b", "new-c"}; !reflect.DeepEqual(hashes, want) {
		t.Fatalf("hashes = %v, want %v", hashes, want)
	}

	used, err := MarkRecoveryCodeUsed(db, userID, "new-b")
	if err != nil || !used {
		t.Fatalf("mark used = %v, %v; want true, nil", used, err)
	}
	used, err = MarkRecoveryCodeUsed(db, userID, "new-b")
	if err != nil || used {
		t.Fatalf("reuse mark = %v, %v; want false, nil", used, err)
	}

	hashes, err = ListRecoveryCodeHashes(db, userID)
	if err != nil {
		t.Fatalf("list after use: %v", err)
	}
	if want := []string{"new-a", "new-c"}; !reflect.DeepEqual(hashes, want) {
		t.Fatalf("unused hashes = %v, want %v", hashes, want)
	}
}

func TestMFARecoveryCodeUseRequiresMatchingUser(t *testing.T) {
	db := mustOpenMem(t)
	userA := mustCreateMFAUser(t, db, "recovery-a@example.com")
	userB := mustCreateMFAUser(t, db, "recovery-b@example.com")

	if err := ReplaceRecoveryCodes(db, userA, []string{"shared"}); err != nil {
		t.Fatalf("replace user A codes: %v", err)
	}
	if err := ReplaceRecoveryCodes(db, userB, []string{"shared"}); err != nil {
		t.Fatalf("replace user B codes: %v", err)
	}

	used, err := MarkRecoveryCodeUsed(db, userA, "shared")
	if err != nil || !used {
		t.Fatalf("mark user A code = %v, %v; want true, nil", used, err)
	}

	hashes, err := ListRecoveryCodeHashes(db, userB)
	if err != nil {
		t.Fatalf("list user B codes: %v", err)
	}
	if want := []string{"shared"}; !reflect.DeepEqual(hashes, want) {
		t.Fatalf("user B hashes = %v, want %v", hashes, want)
	}
}

func TestMFAUserDeletionCascades(t *testing.T) {
	db := mustOpenMem(t)
	userID := mustCreateMFAUser(t, db, "cascade@example.com")

	if err := UpsertMFAFactor(db, MFAFactor{UserID: userID, SecretCiphertext: []byte("ciphertext"), SecretNonce: []byte("nonce"), KeyVersion: 1, EnabledAt: 1, LastAcceptedStep: -1}); err != nil {
		t.Fatalf("upsert factor: %v", err)
	}
	if err := CreateMFATicket(db, "cascade-ticket", userID, time.Now().Unix()+60); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if err := ReplaceRecoveryCodes(db, userID, []string{"cascade-code"}); err != nil {
		t.Fatalf("replace codes: %v", err)
	}

	if err := DeleteUser(db, userID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if _, ok, err := GetMFAFactor(db, userID); err != nil || ok {
		t.Fatalf("factor after user delete ok=%v err=%v; want false, nil", ok, err)
	}
	if gotUserID, ok, err := ConsumeMFATicket(db, "cascade-ticket"); err != nil || ok || gotUserID != 0 {
		t.Fatalf("ticket after user delete = %d, %v, %v; want 0, false, nil", gotUserID, ok, err)
	}
	hashes, err := ListRecoveryCodeHashes(db, userID)
	if err != nil {
		t.Fatalf("list codes after user delete: %v", err)
	}
	if len(hashes) != 0 {
		t.Fatalf("recovery hashes after user delete = %v, want empty", hashes)
	}
}
