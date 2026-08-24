package store

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/model"
)

func withLibraryPathHomeDir(t *testing.T, fn func() (string, error)) {
	t.Helper()
	original := libraryPathHomeDir
	libraryPathHomeDir = fn
	t.Cleanup(func() {
		libraryPathHomeDir = original
	})
}

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

func TestLibraryConfigSavePreservesExistingLibraryIDs(t *testing.T) {
	db := mustOpenMem(t)
	if err := SaveLocalLibraries(db, []LocalLibrary{{Path: "/music", Enabled: true, Name: "Main"}}); err != nil {
		t.Fatalf("initial save: %v", err)
	}
	libs, err := ListLocalLibraries(db)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(libs) != 1 {
		t.Fatalf("expected one library, got %#v", libs)
	}
	wantID := libs[0].ID
	if err := SaveLocalLibraries(db, []LocalLibrary{{ID: wantID, Path: "/music", Enabled: false, Name: "Renamed"}}); err != nil {
		t.Fatalf("second save: %v", err)
	}
	libs, err = ListLocalLibraries(db)
	if err != nil {
		t.Fatalf("list after save: %v", err)
	}
	if len(libs) != 1 || libs[0].ID != wantID || libs[0].Enabled || libs[0].Name != "Renamed" {
		t.Fatalf("library identity was not preserved: want id %d disabled Renamed, got %#v", wantID, libs)
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

func TestLibraryConfigExpandsHomePaths(t *testing.T) {
	db := mustOpenMem(t)
	home := filepath.Join(t.TempDir(), "home")
	withLibraryPathHomeDir(t, func() (string, error) {
		return home, nil
	})

	if err := SaveLocalLibraries(db, []LocalLibrary{
		{Path: "~/Music", Enabled: true, Name: " Main "},
		{Path: "~", Enabled: true},
	}); err != nil {
		t.Fatalf("save home paths: %v", err)
	}

	libs, err := ListLocalLibraries(db)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(libs) != 2 {
		t.Fatalf("expected two libraries, got %#v", libs)
	}
	if libs[0].Path != filepath.Join(home, "Music") || libs[0].Name != "Main" {
		t.Fatalf("first library = %#v", libs[0])
	}
	if libs[1].Path != filepath.Clean(home) {
		t.Fatalf("second library path = %q, want %q", libs[1].Path, filepath.Clean(home))
	}
}

func TestLibraryConfigRejectsUnsupportedHomeForms(t *testing.T) {
	db := mustOpenMem(t)

	err := SaveLocalLibraries(db, []LocalLibrary{{Path: "~other/Music", Enabled: true}})
	if err == nil || !strings.Contains(err.Error(), "unsupported library path") {
		t.Fatalf("unsupported home form error = %v", err)
	}
}

func TestLibraryConfigHomeLookupFailureIsValidationError(t *testing.T) {
	db := mustOpenMem(t)
	withLibraryPathHomeDir(t, func() (string, error) {
		return "", errors.New("home unavailable")
	})

	err := SaveLocalLibraries(db, []LocalLibrary{{Path: "~/Music", Enabled: true}})
	if err == nil || !strings.Contains(err.Error(), "resolve home directory") {
		t.Fatalf("home lookup error = %v", err)
	}
}

func TestLibraryConfigRejectsDuplicatePathsAfterHomeExpansion(t *testing.T) {
	db := mustOpenMem(t)
	home := filepath.Join(t.TempDir(), "home")
	withLibraryPathHomeDir(t, func() (string, error) {
		return home, nil
	})

	err := SaveLocalLibraries(db, []LocalLibrary{
		{Path: "~/Music", Enabled: true},
		{Path: filepath.Join(home, "Music", "."), Enabled: true},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate library path") {
		t.Fatalf("duplicate expanded path error = %v", err)
	}
}

func TestLibraryConfigurationExpandsHomePaths(t *testing.T) {
	db := mustOpenMem(t)
	home := filepath.Join(t.TempDir(), "home")
	withLibraryPathHomeDir(t, func() (string, error) {
		return home, nil
	})

	if err := SaveLibraryConfiguration(db, []LocalLibrary{{Path: "~/Library", Enabled: true}}, FederationSettings{}); err != nil {
		t.Fatalf("save configuration: %v", err)
	}

	libs, err := ListLocalLibraries(db)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(libs) != 1 || libs[0].Path != filepath.Join(home, "Library") {
		t.Fatalf("libraries = %#v", libs)
	}
}

func TestLibraryScanSettingsDefaultDisabledAndRoundTrip(t *testing.T) {
	db := mustOpenMem(t)

	settings, err := LoadLibraryScanSettings(db)
	if err != nil {
		t.Fatalf("load default scan settings: %v", err)
	}
	if settings.AssumeSameTitleFolderCompilations {
		t.Fatal("same-title folder compilation heuristic should default to disabled")
	}

	settings.AssumeSameTitleFolderCompilations = true
	if err := SaveLibraryScanSettings(db, settings); err != nil {
		t.Fatalf("save scan settings: %v", err)
	}
	settings, err = LoadLibraryScanSettings(db)
	if err != nil {
		t.Fatalf("load saved scan settings: %v", err)
	}
	if !settings.AssumeSameTitleFolderCompilations {
		t.Fatal("same-title folder compilation heuristic should round-trip enabled")
	}
}

func TestLibraryScanSettingsChangeForcesLocalRescan(t *testing.T) {
	db := mustOpenMem(t)
	libraryID, err := EnsureLocalLibrary(db, "/music", "Music")
	if err != nil {
		t.Fatalf("ensure library: %v", err)
	}
	track := model.Track{Path: "/music/a.mp3", ModTime: 123, Size: 456, Title: "Song"}
	if _, err := UpsertTrackInLibrary(db, libraryID, track, "Artist", "Artist", "Album"); err != nil {
		t.Fatalf("upsert track: %v", err)
	}

	if err := SaveLibraryScanSettings(db, LibraryScanSettings{AssumeSameTitleFolderCompilations: true}); err != nil {
		t.Fatalf("save scan settings: %v", err)
	}

	modTime, size, ok := TrackStampInLibrary(db, libraryID, track.Path)
	if !ok {
		t.Fatal("track should remain indexed")
	}
	if modTime != 0 || size != 0 {
		t.Fatalf("scan setting change should force re-read, got mod_time=%d size=%d", modTime, size)
	}
}

func TestFederationSettingsValidationAndRoundTrip(t *testing.T) {
	db := mustOpenMem(t)
	bad := FederationSettings{Enabled: true, Role: "member", Token: "secret", PeerID: "peer"}
	if err := SaveFederationSettings(db, bad); err == nil {
		t.Fatal("enabled member without hub address should fail")
	}

	want := FederationSettings{Enabled: true, Role: "hub", Listen: ":9443", Token: "secret", PeerID: "peer", DirectP2P: true}
	if err := SaveFederationSettings(db, want); err != nil {
		t.Fatalf("save federation: %v", err)
	}
	got, ok, err := LoadFederationSettings(db)
	if err != nil || !ok {
		t.Fatalf("load federation: ok=%v err=%v", ok, err)
	}
	if !FederationSettingsEqual(got, want) {
		t.Fatalf("federation settings = %#v, want %#v", got, want)
	}
}

func TestFederationSettingsAcceptsPeerRole(t *testing.T) {
	db := mustOpenMem(t)
	want := FederationSettings{Enabled: true, Role: "peer", Listen: ":9443", Token: "secret", PeerID: "peer-a", DirectP2P: true}
	if err := SaveFederationSettings(db, want); err != nil {
		t.Fatalf("save peer federation: %v", err)
	}
	got, ok, err := LoadFederationSettings(db)
	if err != nil || !ok {
		t.Fatalf("load federation: ok=%v err=%v", ok, err)
	}
	if !FederationSettingsEqual(got, want) {
		t.Fatalf("peer federation settings = %#v, want %#v", got, want)
	}
}

func TestDisabledFederationComparisonIgnoresOperationalFields(t *testing.T) {
	a := FederationSettings{Enabled: false, Token: "secret", PeerID: "peer", HubAddr: "hub", Listen: ":9443"}
	b := FederationSettings{Enabled: false}
	if !FederationSettingsEqual(a, b) {
		t.Fatalf("disabled federation settings should compare equal: %#v %#v", a, b)
	}
}

func TestDisabledFederationSavePreservesOperationalFields(t *testing.T) {
	db := mustOpenMem(t)
	want := FederationSettings{Enabled: false, Token: "secret", PeerID: "peer", HubAddr: "hub", Listen: ":9443", DirectP2P: true}
	if err := SaveFederationSettings(db, want); err != nil {
		t.Fatalf("save federation: %v", err)
	}
	got, ok, err := LoadFederationSettings(db)
	if err != nil || !ok {
		t.Fatalf("load federation: ok=%v err=%v", ok, err)
	}
	if !FederationSettingsEqual(got, want) {
		t.Fatalf("disabled federation settings = %#v, want %#v", got, want)
	}
}
