package store

import (
	"database/sql"
	"testing"
)

func TestSettingsDefaults(t *testing.T) {
	db := mustOpenMem(t)
	if SignupEnabled(db) {
		t.Fatal("signup should default off")
	}
	if GuestAccessEnabled(db) {
		t.Fatal("guest access should default off")
	}
	if AdminMFARequired(db) {
		t.Fatal("admin MFA should default off")
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
	if err := SetAdminMFARequired(db, true); err != nil {
		t.Fatal(err)
	}
	if !SignupEnabled(db) || !GuestAccessEnabled(db) || !AdminMFARequired(db) {
		t.Fatal("settings not persisted")
	}
	if err := SetAdminMFARequired(db, false); err != nil {
		t.Fatal(err)
	}
	if AdminMFARequired(db) {
		t.Fatal("admin MFA setting should persist false")
	}
}

func TestMFASchemaFreshDBHasTablesAndIndexes(t *testing.T) {
	db := mustOpenMem(t)

	assertMFASchema(t, db)
}

func TestMFAMigrationAddsTablesAndIndexes(t *testing.T) {
	db := mustOpenMem(t)

	if _, err := db.Exec(`
		DROP TABLE IF EXISTS mfa_recovery_code;
		DROP TABLE IF EXISTS mfa_ticket;
		DROP TABLE IF EXISTS mfa_factor;
	`); err != nil {
		t.Fatalf("drop MFA schema: %v", err)
	}

	if err := migrate(db); err != nil {
		t.Fatalf("migrate MFA schema: %v", err)
	}
	if err := migrate(db); err != nil {
		t.Fatalf("second migrate MFA schema: %v", err)
	}

	assertMFASchema(t, db)
}

func assertMFASchema(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, table := range []string{"mfa_factor", "mfa_ticket", "mfa_recovery_code"} {
		if !tableExists(t, db, table) {
			t.Fatalf("expected table %s", table)
		}
	}
	for _, index := range []string{"idx_mfa_ticket_user", "idx_mfa_ticket_expires", "idx_mfa_recovery_code_user", "idx_mfa_recovery_code_user_hash"} {
		if !indexExists(t, db, index) {
			t.Fatalf("expected index %s", index)
		}
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
