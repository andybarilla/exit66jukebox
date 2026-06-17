package store

import (
	"path/filepath"
	"testing"
)

func TestLibraryConfigSaveSeedsAndReturnsActiveRoots(t *testing.T) {
	db := mustOpenMem(t)
	root := filepath.Clean("/music/roots/../library")

	if err := SeedLocalLibraries(db, []string{root}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if !LibrarySettingsInitialized(db) {
		t.Fatal("seed should mark library settings initialized")
	}

	libs, err := ListLocalLibraries(db)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(libs) != 1 || libs[0].Path != root || !libs[0].Enabled {
		t.Fatalf("seeded libraries = %#v", libs)
	}

	if err := SaveLocalLibraries(db, []LocalLibrary{{Name: "Main", Path: root, Enabled: false}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	roots, err := EnabledLocalLibraryRoots(db)
	if err != nil {
		t.Fatalf("roots: %v", err)
	}
	if len(roots) != 0 {
		t.Fatalf("disabled library should be excluded, got %v", roots)
	}
}

func TestLibraryConfigDoesNotReseedAfterSavedEmptyList(t *testing.T) {
	db := mustOpenMem(t)
	if err := SaveLocalLibraries(db, nil); err != nil {
		t.Fatalf("save empty: %v", err)
	}
	if err := SeedLocalLibraries(db, []string{"/music"}); err != nil {
		t.Fatalf("seed after save: %v", err)
	}
	libs, err := ListLocalLibraries(db)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(libs) != 0 {
		t.Fatalf("saved empty list should remain authoritative, got %#v", libs)
	}
}

func TestLibraryConfigRejectsBlankAndDuplicatePaths(t *testing.T) {
	db := mustOpenMem(t)
	if err := SaveLocalLibraries(db, []LocalLibrary{{Path: "  "}}); err == nil {
		t.Fatal("blank path should fail")
	}
	if err := SaveLocalLibraries(db, []LocalLibrary{{Path: "/music"}, {Path: "/music/./"}}); err == nil {
		t.Fatal("duplicate normalized path should fail")
	}
}

func TestFederationSettingsValidationAndRoundTrip(t *testing.T) {
	db := mustOpenMem(t)
	bad := FederationSettings{Enabled: true, Role: "member", Token: "secret", PeerID: "peer"}
	if err := SaveFederationSettings(db, bad); err == nil {
		t.Fatal("enabled member without hub address should fail")
	}

	want := FederationSettings{Enabled: true, Role: "hub", Listen: ":9443", Token: "secret", PeerID: "peer"}
	if err := SaveFederationSettings(db, want); err != nil {
		t.Fatalf("save federation: %v", err)
	}
	got, ok, err := LoadFederationSettings(db)
	if err != nil || !ok {
		t.Fatalf("load federation: ok=%v err=%v", ok, err)
	}
	if got != want {
		t.Fatalf("federation settings = %#v, want %#v", got, want)
	}
}
