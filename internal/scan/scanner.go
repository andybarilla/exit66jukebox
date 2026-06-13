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

	go func() {
		defer close(jobs)
		for _, root := range roots {
			if err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				if !audioExt[strings.ToLower(filepath.Ext(p))] {
					return nil
				}
				info, err := d.Info()
				if err != nil {
					return nil
				}
				mt, sz := info.ModTime().Unix(), info.Size()
				omt, osz, ok := store.TrackStamp(db, p)
				if ok && omt == mt && osz == sz {
					jobs <- job{path: p, exists: true}
					return nil
				}
				jobs <- job{path: p, modTime: mt, size: sz, wasIndexed: ok}
				return nil
			}); err != nil && walkErr == nil {
				walkErr = err
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
					Duration: probeDuration(j.path),
				}
				mu.Lock()
				_, err = store.UpsertTrack(db, tr, meta.Artist, meta.AlbumArtistOrFallback(), meta.Album)
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

	snap := p.Snapshot()
	res.Added = snap.Added
	res.Updated = snap.Updated
	res.Skipped = snap.Skipped
	res.Failed = snap.Failed
	return res, walkErr
}
