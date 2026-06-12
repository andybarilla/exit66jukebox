package scan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

func TestProgressRunningFlag(t *testing.T) {
	var p Progress
	if p.Snapshot().Running {
		t.Fatal("a fresh Progress should not report running")
	}
	p.SetRunning(true)
	if !p.Snapshot().Running {
		t.Fatal("SetRunning(true) should be reflected in the snapshot")
	}
	p.SetRunning(false)
	if p.Snapshot().Running {
		t.Fatal("SetRunning(false) should be reflected in the snapshot")
	}
}

func TestScanUpdatesProgressCounts(t *testing.T) {
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

	var p Progress
	res, err := Scan(db, []string{dir}, 4, &p)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	snap := p.Snapshot()
	if snap.Added != 2 {
		t.Fatalf("progress added: want 2, got %d", snap.Added)
	}
	if snap.Added != res.Added {
		t.Fatalf("progress (%d) and result (%d) disagree on added", snap.Added, res.Added)
	}

	// A re-scan reuses a fresh Progress and should report all skipped.
	var p2 Progress
	if _, err := Scan(db, []string{dir}, 4, &p2); err != nil {
		t.Fatalf("rescan: %v", err)
	}
	if got := p2.Snapshot().Skipped; got != 2 {
		t.Fatalf("progress skipped on rescan: want 2, got %d", got)
	}
}

func TestScanAcceptsNilProgress(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	dir := t.TempDir()
	src, _ := os.ReadFile("testdata/sample.mp3")
	os.WriteFile(filepath.Join(dir, "a.mp3"), src, 0o644)

	if _, err := Scan(db, []string{dir}, 2, nil); err != nil {
		t.Fatalf("scan with nil progress: %v", err)
	}
}
