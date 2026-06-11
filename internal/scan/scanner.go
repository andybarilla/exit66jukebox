package scan

import (
	"database/sql"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

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
	path    string
	modTime int64
	size    int64
	exists  bool // already indexed and unchanged
}

// Scan walks the given roots, reads tags from new/changed audio files using
// `workers` goroutines, and upserts them. Unchanged files (same mod_time and
// size) are skipped without reading tags.
func Scan(db *sql.DB, roots []string, workers int) (Result, error) {
	if workers < 1 {
		workers = 1
	}
	var res Result
	jobs := make(chan job)

	go func() {
		defer close(jobs)
		for _, root := range roots {
			filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
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
				if omt, osz, ok := store.TrackStamp(db, p); ok && omt == mt && osz == sz {
					jobs <- job{path: p, exists: true}
					return nil
				}
				jobs <- job{path: p, modTime: mt, size: sz}
				return nil
			})
		}
	}()

	var added, updated, skipped, failed int64
	var wg sync.WaitGroup
	var mu sync.Mutex // serialize writes (single SQLite writer)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if j.exists {
					atomic.AddInt64(&skipped, 1)
					continue
				}
				meta, err := ReadTags(j.path)
				if err != nil {
					atomic.AddInt64(&failed, 1)
					continue
				}
				tr := model.Track{
					Path: j.path, ModTime: j.modTime, Size: j.size,
					Title: meta.Title, TrackNo: meta.TrackNo, Genre: meta.Genre,
				}
				mu.Lock()
				_, _, exists := store.TrackStamp(db, j.path)
				_, err = store.UpsertTrack(db, tr, meta.Artist, meta.Album)
				mu.Unlock()
				if err != nil {
					atomic.AddInt64(&failed, 1)
					continue
				}
				if exists {
					atomic.AddInt64(&updated, 1)
				} else {
					atomic.AddInt64(&added, 1)
				}
			}
		}()
	}
	wg.Wait()

	res.Added = int(added)
	res.Updated = int(updated)
	res.Skipped = int(skipped)
	res.Failed = int(failed)
	return res, nil
}
