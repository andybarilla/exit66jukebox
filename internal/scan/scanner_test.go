package scan

import (
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
