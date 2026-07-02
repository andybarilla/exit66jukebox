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

func TestSecurityModeDefaultsToFullLogin(t *testing.T) {
	db := mustOpenMem(t)

	if got := SecurityModeSetting(db); got != SecurityModeFullLogin {
		t.Fatalf("default security mode = %q, want %q", got, SecurityModeFullLogin)
	}
}

func TestParseSecurityModeAcceptsOnlyApprovedModes(t *testing.T) {
	valid := []SecurityMode{SecurityModeOpen, SecurityModeOpenAdminLocked, SecurityModeHouseholdProfiles, SecurityModeFullLogin}
	for _, mode := range valid {
		got, err := ParseSecurityMode(string(mode))
		if err != nil {
			t.Fatalf("ParseSecurityMode(%q) returned error: %v", mode, err)
		}
		if got != mode {
			t.Fatalf("ParseSecurityMode(%q) = %q", mode, got)
		}
	}

	if _, err := ParseSecurityMode("guest"); err == nil {
		t.Fatalf("ParseSecurityMode accepted an unsupported mode")
	}
}

func TestSecurityModeMigratesFromGuestAccessWhenModeMissing(t *testing.T) {
	db := mustOpenMem(t)

	if err := setMetaFlag(db, keyGuestAccess, true); err != nil {
		t.Fatalf("set legacy guest access: %v", err)
	}
	if got := SecurityModeSetting(db); got != SecurityModeOpenAdminLocked {
		t.Fatalf("guest enabled migrated to %q, want %q", got, SecurityModeOpenAdminLocked)
	}

	db = mustOpenMem(t)
	if err := setMetaFlag(db, keyGuestAccess, false); err != nil {
		t.Fatalf("set legacy guest access: %v", err)
	}
	if got := SecurityModeSetting(db); got != SecurityModeFullLogin {
		t.Fatalf("guest disabled migrated to %q, want %q", got, SecurityModeFullLogin)
	}
}

func TestSecurityModeOverridesLegacyGuestAccess(t *testing.T) {
	db := mustOpenMem(t)
	if err := setMetaFlag(db, keyGuestAccess, true); err != nil {
		t.Fatalf("set legacy guest access: %v", err)
	}
	if err := SetSecurityMode(db, SecurityModeHouseholdProfiles); err != nil {
		t.Fatalf("set security mode: %v", err)
	}

	if got := SecurityModeSetting(db); got != SecurityModeHouseholdProfiles {
		t.Fatalf("security mode = %q, want %q", got, SecurityModeHouseholdProfiles)
	}
	if GuestAccessEnabled(db) {
		t.Fatalf("legacy guest access should map from household_profiles as false")
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
