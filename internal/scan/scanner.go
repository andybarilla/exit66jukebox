package scan

import (
	"database/sql"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"

	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

// Result summarizes one scan run.
type Result struct {
	Added   int
	Updated int
	Skipped int
	Failed  int
}

var audioExt = map[string]bool{".mp3": true, ".ogg": true, ".flac": true}

type job struct {
	libraryID  int64
	path       string
	modTime    int64
	size       int64
	exists     bool // already indexed and unchanged
	wasIndexed bool // existed in the index but stamp differed
}

// Scan walks the given roots, reads tags from new/changed audio files using
// `workers` goroutines, and upserts them. Unchanged files (same mod_time and
// size) are skipped without reading tags. If p is non-nil its counters are
// updated live as files are processed, so a concurrent reader can observe
// progress; pass nil when live progress isn't needed.
func Scan(db *sql.DB, roots []string, workers int, p *Progress) (Result, error) {
	if workers < 1 {
		workers = 1
	}
	if p == nil {
		p = &Progress{}
	}
	var res Result
	jobs := make(chan job)
	var walkErr error
	seenPathsByLibrary := map[int64][]string{}
	scannedLibraryIDs := []int64{}

	go func() {
		defer close(jobs)
		for _, root := range roots {
			libraryID, err := store.EnsureLocalLibrary(db, root, filepath.Base(root))
			if err != nil {
				if walkErr == nil {
					walkErr = err
				}
				continue
			}
			rootHadWalkError := false
			if err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					rootHadWalkError = true
					return nil
				}
				if d.IsDir() {
					return nil
				}
				if !audioExt[strings.ToLower(filepath.Ext(p))] {
					return nil
				}
				seenPathsByLibrary[libraryID] = append(seenPathsByLibrary[libraryID], p)
				info, err := d.Info()
				if err != nil {
					rootHadWalkError = true
					return nil
				}
				mt, sz := info.ModTime().Unix(), info.Size()
				omt, osz, ok := store.TrackStampInLibrary(db, libraryID, p)
				if ok && omt == mt && osz == sz {
					jobs <- job{libraryID: libraryID, path: p, exists: true}
					return nil
				}
				jobs <- job{libraryID: libraryID, path: p, modTime: mt, size: sz, wasIndexed: ok}
				return nil
			}); err != nil {
				rootHadWalkError = true
				if walkErr == nil {
					walkErr = err
				}
			}
			if !rootHadWalkError {
				scannedLibraryIDs = append(scannedLibraryIDs, libraryID)
			}
		}
	}()

	var wg sync.WaitGroup
	var mu sync.Mutex // serialize writes (single SQLite writer)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if j.exists {
					p.skipped.Add(1)
					continue
				}
				meta, err := ReadTags(j.path)
				if err != nil {
					p.failed.Add(1)
					continue
				}
				tr := model.Track{
					Path: j.path, ModTime: j.modTime, Size: j.size,
					Title: meta.Title, TrackNo: meta.TrackNo, Genre: meta.Genre,
					Duration: probeDuration(j.path), Links: meta.Links,
				}
				mu.Lock()
				_, err = store.UpsertTrackInLibrary(db, j.libraryID, tr, meta.Artist, meta.AlbumArtistOrFallback(), meta.Album)
				mu.Unlock()
				if err != nil {
					p.failed.Add(1)
					continue
				}
				if j.wasIndexed {
					p.updated.Add(1)
				} else {
					p.added.Add(1)
				}
			}
		}()
	}
	wg.Wait()

	// Re-pointing tracks to album-artist-keyed albums leaves the old
	// per-track-artist album (and its artist) orphaned; clear them.
	if err := store.PruneOrphans(db); err != nil && walkErr == nil {
		walkErr = err
	}
	for _, libraryID := range scannedLibraryIDs {
		if err := store.DeleteLocalLibraryTracksExcept(db, libraryID, seenPathsByLibrary[libraryID]); err != nil && walkErr == nil {
			walkErr = err
		}
	}

	snap := p.Snapshot()
	res.Added = snap.Added
	res.Updated = snap.Updated
	res.Skipped = snap.Skipped
	res.Failed = snap.Failed
	return res, walkErr
}
