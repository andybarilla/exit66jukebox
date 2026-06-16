package store

import "testing"

func TestSettingsDefaults(t *testing.T) {
	db := mustOpenMem(t)
	if SignupEnabled(db) {
		t.Fatal("signup should default off")
	}
	if GuestAccessEnabled(db) {
		t.Fatal("guest access should default off")
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	db := mustOpenMem(t)
	if err := SetSignupEnabled(db, true); err != nil {
		t.Fatal(err)
	}
	if err := SetGuestAccessEnabled(db, true); err != nil {
		t.Fatal(err)
	}
	if !SignupEnabled(db) || !GuestAccessEnabled(db) {
		t.Fatal("settings not persisted")
	}
}

func TestSigningSecretStable(t *testing.T) {
	db := mustOpenMem(t)
	a, err := MediaSigningSecret(db)
	if err != nil || len(a) < 32 {
		t.Fatalf("secret: len=%d err=%v", len(a), err)
	}
	b, _ := MediaSigningSecret(db)
	if string(a) != string(b) {
		t.Fatal("secret changed across calls — not persisted")
	}
}
