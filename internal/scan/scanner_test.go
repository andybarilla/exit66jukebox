package scan

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

func TestScanIndexesAndIsIncremental(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	dir := t.TempDir()
	src, _ := os.ReadFile("testdata/sample.mp3")
	for _, name := range []string{"a.mp3", "b.mp3"} {
		os.WriteFile(filepath.Join(dir, name), src, 0o644)
	}

	res, err := Scan(db, []string{dir}, 4, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if res.Added != 2 {
		t.Fatalf("expected 2 added, got %d", res.Added)
	}

	res2, _ := Scan(db, []string{dir}, 4, nil)
	if res2.Added != 0 || res2.Updated != 0 {
		t.Fatalf("expected no changes on re-scan, got added=%d updated=%d",
			res2.Added, res2.Updated)
	}
	if res2.Skipped != 2 {
		t.Fatalf("expected 2 skipped on re-scan, got %d", res2.Skipped)
	}
}

// TestScanKeysAlbumByAlbumArtist verifies the scan pipeline keys the album by
// its album-artist. The fixture carries no AlbumArtist tag, so the album-artist
// falls back to the track artist and the album is keyed by it.
func TestScanKeysAlbumByAlbumArtist(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	dir := t.TempDir()
	src, _ := os.ReadFile("testdata/sample.mp3")
	os.WriteFile(filepath.Join(dir, "a.mp3"), src, 0o644)

	if _, err := Scan(db, []string{dir}, 1, nil); err != nil {
		t.Fatalf("scan: %v", err)
	}
	var albumArtist string
	if err := db.QueryRow(
		`SELECT ar.name FROM album a JOIN artist ar ON ar.id = a.artist_id LIMIT 1`,
	).Scan(&albumArtist); err != nil {
		t.Fatalf("query: %v", err)
	}
	if albumArtist != "Test Artist" {
		t.Fatalf("expected album keyed by fallback track artist %q, got %q",
			"Test Artist", albumArtist)
	}
}

func TestScanUsesSameFolderCompilationSetting(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := store.SaveLibraryScanSettings(db, store.LibraryScanSettings{AssumeSameTitleFolderCompilations: true}); err != nil {
		t.Fatalf("save scan settings: %v", err)
	}
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "artist-a.mp3")
	secondPath := filepath.Join(dir, "artist-b.mp3")
	if err := os.WriteFile(firstPath, []byte("a"), 0o644); err != nil {
		t.Fatalf("write first track: %v", err)
	}
	if err := os.WriteFile(secondPath, []byte("b"), 0o644); err != nil {
		t.Fatalf("write second track: %v", err)
	}
	oldReadTags := readTags
	readTags = func(path string) (Meta, error) {
		artist := "Artist A"
		if path == secondPath {
			artist = "Artist B"
		}
		return Meta{Title: filepath.Base(path), Artist: artist, Album: "Shared Album"}, nil
	}
	t.Cleanup(func() { readTags = oldReadTags })

	if _, err := Scan(db, []string{dir}, 2, nil); err != nil {
		t.Fatalf("scan: %v", err)
	}

	var albumArtist string
	if err := db.QueryRow(
		`SELECT ar.name FROM album a JOIN artist ar ON ar.id = a.artist_id WHERE a.name = 'Shared Album'`,
	).Scan(&albumArtist); err != nil {
		t.Fatalf("query album artist: %v", err)
	}
	if albumArtist != store.VariousArtists {
		t.Fatalf("expected album keyed by %q, got %q", store.VariousArtists, albumArtist)
	}
}

func TestScanStoresDuration(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	dir := t.TempDir()
	src, _ := os.ReadFile("testdata/sample.mp3")
	os.WriteFile(filepath.Join(dir, "a.mp3"), src, 0o644)

	if _, err := Scan(db, []string{dir}, 2, nil); err != nil {
		t.Fatalf("scan: %v", err)
	}
	var dur int
	if err := db.QueryRow(`SELECT duration FROM track LIMIT 1`).Scan(&dur); err != nil {
		t.Fatalf("query: %v", err)
	}
	if dur <= 0 {
		t.Fatalf("expected stored duration > 0, got %d", dur)
	}
}

func TestScanReindexesChangedFile(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	dir := t.TempDir()
	src, _ := os.ReadFile("testdata/sample.mp3")
	p := filepath.Join(dir, "a.mp3")
	os.WriteFile(p, src, 0o644)

	if res, _ := Scan(db, []string{dir}, 2, nil); res.Added != 1 {
		t.Fatalf("expected 1 added, got %d", res.Added)
	}
	// Append bytes so size changes and the scanner re-reads it.
	os.WriteFile(p, append(src, src...), 0o644)
	res, _ := Scan(db, []string{dir}, 2, nil)
	if res.Updated != 1 {
		t.Fatalf("expected 1 updated, got %d (added=%d skipped=%d)",
			res.Updated, res.Added, res.Skipped)
	}
}

func TestScanPrunesMissingFilesOnlyFromScannedLibrary(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	firstDir := t.TempDir()
	secondDir := t.TempDir()
	src, _ := os.ReadFile("testdata/sample.mp3")
	firstPath := filepath.Join(firstDir, "same.mp3")
	secondPath := filepath.Join(secondDir, "same.mp3")
	os.WriteFile(firstPath, src, 0o644)
	os.WriteFile(secondPath, src, 0o644)

	if _, err := Scan(db, []string{firstDir, secondDir}, 2, nil); err != nil {
		t.Fatalf("initial scan: %v", err)
	}
	if err := os.Remove(firstPath); err != nil {
		t.Fatalf("remove first library file: %v", err)
	}
	if _, err := Scan(db, []string{firstDir}, 2, nil); err != nil {
		t.Fatalf("rescan first library: %v", err)
	}

	assertTracksInLibrary(t, db, firstDir, 0)
	assertTracksInLibrary(t, db, secondDir, 1)
	if _, _, ok := store.TrackStampInLibrary(db, mustLocalLibraryID(t, db, secondDir), secondPath); !ok {
		t.Fatalf("second library track should remain indexed")
	}
}

func assertTracksInLibrary(t *testing.T, db *sql.DB, libraryPath string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(
		`SELECT count(*)
		 FROM track t
		 JOIN local_library ll ON ll.id = t.library_id
		 WHERE ll.path = ? AND t.source_peer = ''`,
		libraryPath,
	).Scan(&got); err != nil {
		t.Fatalf("count tracks in %s: %v", libraryPath, err)
	}
	if got != want {
		t.Fatalf("expected %d tracks in %s, got %d", want, libraryPath, got)
	}
}

func mustLocalLibraryID(t *testing.T, db *sql.DB, libraryPath string) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`SELECT id FROM local_library WHERE path = ?`, libraryPath).Scan(&id); err != nil {
		t.Fatalf("local library id for %s: %v", libraryPath, err)
	}
	return id
}
