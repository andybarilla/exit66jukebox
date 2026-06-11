package scan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

func TestScanIndexesAndIsIncremental(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()

	dir := t.TempDir()
	src, _ := os.ReadFile("testdata/sample.mp3")
	for _, name := range []string{"a.mp3", "b.mp3"} {
		os.WriteFile(filepath.Join(dir, name), src, 0o644)
	}

	res, err := Scan(db, []string{dir}, 4)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if res.Added != 2 {
		t.Fatalf("expected 2 added, got %d", res.Added)
	}

	res2, _ := Scan(db, []string{dir}, 4)
	if res2.Added != 0 || res2.Updated != 0 {
		t.Fatalf("expected no changes on re-scan, got added=%d updated=%d",
			res2.Added, res2.Updated)
	}
	if res2.Skipped != 2 {
		t.Fatalf("expected 2 skipped on re-scan, got %d", res2.Skipped)
	}
}
