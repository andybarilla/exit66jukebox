# Exit 66 Jukebox v1 — Plan 1 of 2: Foundation & Private Jukebox

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A runnable Go daemon that scans a local music library, serves a browsable web UI, and lets a browser session build a fairness-governed request queue and play it back track-by-track (private stream mode).

**Architecture:** Single Go binary. SQLite (pure-Go `modernc.org/sqlite`, no CGo) stores the library index and per-stream queues/history. A `jukebox` service holds the fairness logic (the "soul") over the store. A stdlib `net/http` server (Go 1.22+ routed `ServeMux`) exposes a JSON browse/request API and serves track files for client-side playback. A Svelte SPA is built to static files and embedded via `go:embed`. The shared-stream broadcaster and SSE are deliberately deferred to Plan 2; the `jukebox.Next` seam is designed so the private path is complete without them.

**Tech Stack:** Go 1.26, `modernc.org/sqlite`, `github.com/dhowden/tag`, stdlib `net/http`/`testing`, Svelte + Vite.

**Module path:** `github.com/andybarilla/exit66jukebox`

---

## File Structure

| Path | Responsibility |
|------|----------------|
| `go.mod`, `.gitignore` | Module + ignore build artifacts |
| `main.go` | Flag/config parse, open DB, run migrations, start scanner, start HTTP server |
| `internal/config/config.go` | Config struct + flag parsing (port, db path, library roots, history window, shuffle top-N) |
| `internal/model/model.go` | Plain data structs shared across packages (no behavior, no imports) |
| `internal/store/store.go` | Open SQLite, run migrations |
| `internal/store/schema.sql` | Embedded DDL |
| `internal/store/library.go` | Artist/album/track upserts + browse queries |
| `internal/store/queue.go` | Stream/queue_item/history persistence |
| `internal/scan/tags.go` | Read tags from one file via `dhowden/tag` |
| `internal/scan/scanner.go` | Incremental, concurrent, batched library scan |
| `internal/jukebox/jukebox.go` | Fairness service: request/next/remove/clear/queue |
| `internal/api/server.go` | Mux wiring, embed UI, JSON helpers |
| `internal/api/browse.go` | `/api/artists`, `/api/albums`, `/api/tracks` |
| `internal/api/streams.go` | `/api/streams/{id}` + requests |
| `internal/api/audio.go` | `/api/tracks/{id}/audio`, cover art |
| `web/` | Svelte source; built to `web/dist`, embedded |

Types defined once in `internal/model` and reused everywhere to keep signatures consistent.

---

## Phase 0 — Repo reset & Go scaffold

### Task 0.1: Preserve the legacy code under a tag

**Files:** none (git only)

- [ ] **Step 1: Tag the current (Java) state**

```bash
git tag -a legacy/java-v5 -m "Exit 66 Jukebox 5.0.0 — final Java/Jetty/HSQLDB version"
```

- [ ] **Step 2: Verify the tag exists**

Run: `git tag --list 'legacy/*'`
Expected: prints `legacy/java-v5`

### Task 0.2: Remove the Java tree from the working branch

**Files:** delete `src/`, `lib/`, `etc/`, `distr/`, `support/`, `web/` (old), `build.xml`, root images

- [ ] **Step 1: Delete legacy files (preserved in the tag)**

```bash
git rm -r --quiet src lib etc distr support web build.xml exit66jb.jpg exit66jbicon.png
```

- [ ] **Step 2: Verify only docs remain tracked**

Run: `git ls-files | grep -v '^docs/'`
Expected: no output (everything left is under `docs/`)

- [ ] **Step 3: Commit the reset**

```bash
git add -A
git commit -m "chore: reset working tree for Go rewrite (Java preserved in legacy/java-v5 tag)"
```

### Task 0.3: Initialize the Go module

**Files:** Create `go.mod`, `.gitignore`

- [ ] **Step 1: Init module**

Run: `go mod init github.com/andybarilla/exit66jukebox`
Expected: creates `go.mod` with `go 1.26`

- [ ] **Step 2: Write `.gitignore`**

```gitignore
/exit66jukebox
/exit66.db
/web/dist/
/web/node_modules/
*.test
```

- [ ] **Step 3: Add dependencies**

```bash
go get modernc.org/sqlite@latest
go get github.com/dhowden/tag@latest
```

Expected: `go.mod`/`go.sum` updated with both modules.

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum .gitignore
git commit -m "chore: initialize Go module and dependencies"
```

---

## Phase 1 — Models & storage

### Task 1.1: Shared data structs

**Files:** Create `internal/model/model.go`

- [ ] **Step 1: Write the structs**

```go
package model

// Artist is a distinct performer name.
type Artist struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Album belongs to one artist and may carry a cover image path.
type Album struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	ArtistID int64  `json:"artist_id"`
	Cover    string `json:"-"`
}

// Track is one audio file plus its indexed tags.
type Track struct {
	ID        int64  `json:"id"`
	Path      string `json:"-"`
	ModTime   int64  `json:"-"`
	Size      int64  `json:"-"`
	Title     string `json:"title"`
	ArtistID  int64  `json:"artist_id"`
	AlbumID   int64  `json:"album_id"`
	TrackNo   int    `json:"track_no"`
	Genre     string `json:"genre"`
	Duration  int    `json:"duration"`
	PlayCount int    `json:"play_count"`
}

// Stream owns a queue + fairness config. Kind is "private" or "shared".
type Stream struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/model/`
Expected: no output (success)

- [ ] **Step 3: Commit**

```bash
git add internal/model/model.go
git commit -m "feat: shared data models"
```

### Task 1.2: SQLite schema + open/migrate

**Files:** Create `internal/store/schema.sql`, `internal/store/store.go`, `internal/store/store_test.go`

- [ ] **Step 1: Write the schema**

`internal/store/schema.sql`:

```sql
CREATE TABLE IF NOT EXISTS artist (
    id   INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE
);
CREATE TABLE IF NOT EXISTS album (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    name      TEXT NOT NULL,
    artist_id INTEGER NOT NULL REFERENCES artist(id),
    cover     TEXT NOT NULL DEFAULT '',
    UNIQUE(name, artist_id)
);
CREATE TABLE IF NOT EXISTS track (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    path       TEXT NOT NULL UNIQUE,
    mod_time   INTEGER NOT NULL,
    size       INTEGER NOT NULL,
    title      TEXT NOT NULL,
    artist_id  INTEGER NOT NULL REFERENCES artist(id),
    album_id   INTEGER NOT NULL REFERENCES album(id),
    track_no   INTEGER NOT NULL DEFAULT 0,
    genre      TEXT NOT NULL DEFAULT '',
    duration   INTEGER NOT NULL DEFAULT 0,
    play_count INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_track_artist ON track(artist_id);
CREATE INDEX IF NOT EXISTS idx_track_album  ON track(album_id);

CREATE TABLE IF NOT EXISTS stream (
    id   TEXT PRIMARY KEY,
    name TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL DEFAULT 'private'
);
CREATE TABLE IF NOT EXISTS queue_item (
    stream_id  TEXT NOT NULL REFERENCES stream(id),
    track_id   INTEGER NOT NULL REFERENCES track(id),
    play_order INTEGER NOT NULL,
    added_by   TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (stream_id, track_id)
);
CREATE TABLE IF NOT EXISTS history (
    stream_id TEXT NOT NULL,
    track_id  INTEGER NOT NULL,
    played_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_history_stream ON history(stream_id, played_at);
```

- [ ] **Step 2: Write the failing test**

`internal/store/store_test.go`:

```go
package store

import "testing"

func TestOpenRunsMigrations(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	var n int
	err = db.QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='track'`,
	).Scan(&n)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected track table to exist, got count %d", n)
	}
}
```

- [ ] **Step 3: Run the test, expect failure**

Run: `go test ./internal/store/`
Expected: FAIL — `undefined: Open`

- [ ] **Step 4: Implement `Open`**

`internal/store/store.go`:

```go
package store

import (
	"database/sql"
	_ "embed"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

// Open opens (or creates) the SQLite database at path and applies the schema.
// Pass ":memory:" for an ephemeral test database.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// modernc :memory: lives per-connection; pin to one so tests see the schema.
	if path == ":memory:" {
		db.SetMaxOpenConns(1)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
```

- [ ] **Step 5: Run the test, expect pass**

Run: `go test ./internal/store/`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/store/schema.sql internal/store/store.go internal/store/store_test.go
git commit -m "feat: SQLite open + schema migration"
```

### Task 1.3: Library upserts

**Files:** Create `internal/store/library.go`, `internal/store/library_test.go`

- [ ] **Step 1: Write the failing test**

`internal/store/library_test.go`:

```go
package store

import (
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/model"
)

func TestUpsertTrackIsIdempotent(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	tr := model.Track{
		Path: "/music/a.mp3", ModTime: 100, Size: 2048,
		Title: "Song A", TrackNo: 1, Genre: "Rock", Duration: 180,
	}
	id1, err := UpsertTrack(db, tr, "The Band", "First Album")
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	id2, err := UpsertTrack(db, tr, "The Band", "First Album")
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("expected same track id on re-upsert, got %d then %d", id1, id2)
	}

	var artists int
	db.QueryRow(`SELECT count(*) FROM artist`).Scan(&artists)
	if artists != 1 {
		t.Fatalf("expected 1 artist, got %d", artists)
	}
}
```

- [ ] **Step 2: Run the test, expect failure**

Run: `go test ./internal/store/ -run TestUpsertTrack`
Expected: FAIL — `undefined: UpsertTrack`

- [ ] **Step 3: Implement upserts**

`internal/store/library.go`:

```go
package store

import (
	"database/sql"

	"github.com/andybarilla/exit66jukebox/internal/model"
)

func upsertArtist(db *sql.DB, name string) (int64, error) {
	if _, err := db.Exec(
		`INSERT INTO artist(name) VALUES(?) ON CONFLICT(name) DO NOTHING`, name,
	); err != nil {
		return 0, err
	}
	var id int64
	err := db.QueryRow(`SELECT id FROM artist WHERE name = ?`, name).Scan(&id)
	return id, err
}

func upsertAlbum(db *sql.DB, name string, artistID int64) (int64, error) {
	if _, err := db.Exec(
		`INSERT INTO album(name, artist_id) VALUES(?, ?)
		 ON CONFLICT(name, artist_id) DO NOTHING`, name, artistID,
	); err != nil {
		return 0, err
	}
	var id int64
	err := db.QueryRow(
		`SELECT id FROM album WHERE name = ? AND artist_id = ?`, name, artistID,
	).Scan(&id)
	return id, err
}

// UpsertTrack inserts or updates a track by its path, creating the artist and
// album rows as needed. Returns the track id.
func UpsertTrack(db *sql.DB, t model.Track, artistName, albumName string) (int64, error) {
	artistID, err := upsertArtist(db, artistName)
	if err != nil {
		return 0, err
	}
	albumID, err := upsertAlbum(db, albumName, artistID)
	if err != nil {
		return 0, err
	}
	_, err = db.Exec(
		`INSERT INTO track(path, mod_time, size, title, artist_id, album_id, track_no, genre, duration)
		 VALUES(?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(path) DO UPDATE SET
		   mod_time=excluded.mod_time, size=excluded.size, title=excluded.title,
		   artist_id=excluded.artist_id, album_id=excluded.album_id,
		   track_no=excluded.track_no, genre=excluded.genre, duration=excluded.duration`,
		t.Path, t.ModTime, t.Size, t.Title, artistID, albumID, t.TrackNo, t.Genre, t.Duration,
	)
	if err != nil {
		return 0, err
	}
	var id int64
	err = db.QueryRow(`SELECT id FROM track WHERE path = ?`, t.Path).Scan(&id)
	return id, err
}

// TrackStamp returns the stored mod_time and size for a path, or ok=false if
// the path is not indexed. Used by the scanner to skip unchanged files.
func TrackStamp(db *sql.DB, path string) (modTime, size int64, ok bool) {
	err := db.QueryRow(
		`SELECT mod_time, size FROM track WHERE path = ?`, path,
	).Scan(&modTime, &size)
	if err != nil {
		return 0, 0, false
	}
	return modTime, size, true
}
```

- [ ] **Step 4: Run the test, expect pass**

Run: `go test ./internal/store/ -run TestUpsertTrack`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/library.go internal/store/library_test.go
git commit -m "feat: track/artist/album upserts with idempotency"
```

### Task 1.4: Browse queries

**Files:** Modify `internal/store/library.go`, `internal/store/library_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/store/library_test.go`:

```go
func TestListTracksSearchAndPage(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	UpsertTrack(db, model.Track{Path: "/m/1.mp3", Title: "Blue Sky"}, "A", "X")
	UpsertTrack(db, model.Track{Path: "/m/2.mp3", Title: "Red Moon"}, "B", "Y")
	UpsertTrack(db, model.Track{Path: "/m/3.mp3", Title: "Blue Moon"}, "C", "Z")

	all, err := ListTracks(db, "", 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 tracks, got %d", len(all))
	}

	blue, _ := ListTracks(db, "Blue", 10, 0)
	if len(blue) != 2 {
		t.Fatalf("expected 2 'Blue' tracks, got %d", len(blue))
	}

	page, _ := ListTracks(db, "", 1, 1)
	if len(page) != 1 {
		t.Fatalf("expected 1 track on page, got %d", len(page))
	}
}
```

- [ ] **Step 2: Run the test, expect failure**

Run: `go test ./internal/store/ -run TestListTracks`
Expected: FAIL — `undefined: ListTracks`

- [ ] **Step 3: Implement browse queries**

Append to `internal/store/library.go`:

```go
// ListTracks returns tracks whose title matches the search substring (empty =
// all), ordered by title, paged by limit/offset. A limit <= 0 means no limit.
func ListTracks(db *sql.DB, search string, limit, offset int) ([]model.Track, error) {
	q := `SELECT id, title, artist_id, album_id, track_no, genre, duration, play_count
	      FROM track WHERE title LIKE ? ORDER BY title LIMIT ? OFFSET ?`
	lim := limit
	if lim <= 0 {
		lim = -1 // SQLite: no limit
	}
	rows, err := db.Query(q, "%"+search+"%", lim, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Track
	for rows.Next() {
		var t model.Track
		if err := rows.Scan(&t.ID, &t.Title, &t.ArtistID, &t.AlbumID,
			&t.TrackNo, &t.Genre, &t.Duration, &t.PlayCount); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListArtists returns artists matching search (empty = all), ordered by name.
func ListArtists(db *sql.DB, search string, limit, offset int) ([]model.Artist, error) {
	lim := limit
	if lim <= 0 {
		lim = -1
	}
	rows, err := db.Query(
		`SELECT id, name FROM artist WHERE name LIKE ? ORDER BY name LIMIT ? OFFSET ?`,
		"%"+search+"%", lim, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Artist
	for rows.Next() {
		var a model.Artist
		if err := rows.Scan(&a.ID, &a.Name); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListAlbums returns albums matching search (empty = all), ordered by name.
func ListAlbums(db *sql.DB, search string, limit, offset int) ([]model.Album, error) {
	lim := limit
	if lim <= 0 {
		lim = -1
	}
	rows, err := db.Query(
		`SELECT id, name, artist_id FROM album WHERE name LIKE ? ORDER BY name LIMIT ? OFFSET ?`,
		"%"+search+"%", lim, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Album
	for rows.Next() {
		var a model.Album
		if err := rows.Scan(&a.ID, &a.Name, &a.ArtistID); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetTrack returns a single track and its file path. ok=false if not found.
func GetTrack(db *sql.DB, id int64) (t model.Track, path string, ok bool) {
	err := db.QueryRow(
		`SELECT id, path, title, artist_id, album_id, track_no, genre, duration, play_count
		 FROM track WHERE id = ?`, id).Scan(
		&t.ID, &path, &t.Title, &t.ArtistID, &t.AlbumID,
		&t.TrackNo, &t.Genre, &t.Duration, &t.PlayCount)
	if err != nil {
		return model.Track{}, "", false
	}
	return t, path, true
}
```

- [ ] **Step 4: Run the test, expect pass**

Run: `go test ./internal/store/ -run TestListTracks`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/library.go internal/store/library_test.go
git commit -m "feat: browse queries for artists/albums/tracks"
```

---

## Phase 2 — Scanner

### Task 2.1: Tag reader

**Files:** Create `internal/scan/tags.go`, `internal/scan/tags_test.go`, `internal/scan/testdata/` (add a tiny sample mp3)

- [ ] **Step 1: Add a sample audio file with tags**

Place any small tagged `.mp3` at `internal/scan/testdata/sample.mp3` (a few seconds is fine). Confirm it has an artist/title set, e.g. with `ffprobe internal/scan/testdata/sample.mp3`.

- [ ] **Step 2: Write the failing test**

`internal/scan/tags_test.go`:

```go
package scan

import "testing"

func TestReadTagsReturnsArtistAndTitle(t *testing.T) {
	meta, err := ReadTags("testdata/sample.mp3")
	if err != nil {
		t.Fatalf("ReadTags: %v", err)
	}
	if meta.Artist == "" {
		t.Errorf("expected a non-empty artist")
	}
	if meta.Title == "" {
		t.Errorf("expected a non-empty title")
	}
}

func TestReadTagsUnknownFallback(t *testing.T) {
	meta, err := ReadTags("testdata/sample.mp3")
	if err != nil {
		t.Fatalf("ReadTags: %v", err)
	}
	// album may be blank in tags; normalize() must never leave it empty.
	if meta.Album == "" {
		t.Errorf("album should fall back to a placeholder, got empty")
	}
}
```

- [ ] **Step 3: Run the test, expect failure**

Run: `go test ./internal/scan/ -run TestReadTags`
Expected: FAIL — `undefined: ReadTags`

- [ ] **Step 4: Implement the tag reader**

`internal/scan/tags.go`:

```go
package scan

import (
	"os"

	"github.com/dhowden/tag"
)

// Meta is the subset of tag data the index stores.
type Meta struct {
	Title   string
	Artist  string
	Album   string
	Genre   string
	TrackNo int
}

// ReadTags reads tags from a single audio file, filling blanks with placeholders
// so the index never stores empty artist/album/title.
func ReadTags(path string) (Meta, error) {
	f, err := os.Open(path)
	if err != nil {
		return Meta{}, err
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err != nil {
		return Meta{}, err
	}
	trackNo, _ := m.Track()
	meta := Meta{
		Title:   m.Title(),
		Artist:  m.Artist(),
		Album:   m.Album(),
		Genre:   m.Genre(),
		TrackNo: trackNo,
	}
	return normalize(meta, path), nil
}

func normalize(m Meta, path string) Meta {
	if m.Title == "" {
		m.Title = baseName(path)
	}
	if m.Artist == "" {
		m.Artist = "Unknown Artist"
	}
	if m.Album == "" {
		m.Album = "Unknown Album"
	}
	return m
}

func baseName(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[i+1:]
		}
	}
	return path
}
```

- [ ] **Step 5: Run the test, expect pass**

Run: `go test ./internal/scan/ -run TestReadTags`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/scan/tags.go internal/scan/tags_test.go internal/scan/testdata/sample.mp3
git commit -m "feat: per-file tag reader with placeholder fallback"
```

### Task 2.2: Incremental concurrent scan

**Files:** Create `internal/scan/scanner.go`, `internal/scan/scanner_test.go`

- [ ] **Step 1: Write the failing test**

`internal/scan/scanner_test.go`:

```go
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

	// Second scan with no file changes must add/update nothing.
	res2, _ := Scan(db, []string{dir}, 4)
	if res2.Added != 0 || res2.Updated != 0 {
		t.Fatalf("expected no changes on re-scan, got added=%d updated=%d",
			res2.Added, res2.Updated)
	}
	if res2.Skipped != 2 {
		t.Fatalf("expected 2 skipped on re-scan, got %d", res2.Skipped)
	}
}
```

- [ ] **Step 2: Run the test, expect failure**

Run: `go test ./internal/scan/ -run TestScanIndexes`
Expected: FAIL — `undefined: Scan`

- [ ] **Step 3: Implement the scanner**

`internal/scan/scanner.go`:

```go
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
// `workers` goroutines, and upserts them in batches. Unchanged files (same
// mod_time and size) are skipped without reading tags.
func Scan(db *sql.DB, roots []string, workers int) (Result, error) {
	if workers < 1 {
		workers = 1
	}
	var res Result
	jobs := make(chan job)

	// Producer: walk roots, decide skip vs read.
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
				_, isNew := store.TrackStamp(db, j.path)
				_, err = store.UpsertTrack(db, tr, meta.Artist, meta.Album)
				mu.Unlock()
				if err != nil {
					atomic.AddInt64(&failed, 1)
					continue
				}
				if isNew {
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
```

> Note: `store.TrackStamp` returns `ok=true` when the path is already indexed; `isNew` here is the negation of that "already present" check, evaluated under the write lock to classify add vs update.

- [ ] **Step 4: Run the test, expect pass**

Run: `go test ./internal/scan/ -run TestScanIndexes`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/scan/scanner.go internal/scan/scanner_test.go
git commit -m "feat: incremental concurrent batched library scanner"
```

---

## Phase 3 — Jukebox fairness (the soul)

### Task 3.1: Request fairness rules

**Files:** Create `internal/jukebox/jukebox.go`, `internal/jukebox/jukebox_test.go`

- [ ] **Step 1: Write the failing test**

`internal/jukebox/jukebox_test.go`:

```go
package jukebox

import (
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

func seedTrack(t *testing.T, db interface {
	Exec(string, ...any) (any, error)
}) {}

func TestRequestRejectsDuplicateAndRecent(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	id, _ := store.UpsertTrack(db, model.Track{Path: "/m/a.mp3", Title: "A"}, "Band", "Album")

	jb := New(db, Config{HistoryWindow: 5})
	jb.EnsureStream("sess1", "private")

	if got := jb.Request("sess1", id); got != Requested {
		t.Fatalf("first request: want Requested, got %v", got)
	}
	if got := jb.Request("sess1", id); got != AlreadyQueued {
		t.Fatalf("duplicate request: want AlreadyQueued, got %v", got)
	}

	// Play it (moves to history), then re-request -> RecentlyPlayed.
	tr, ok := jb.Next("sess1")
	if !ok || tr.ID != id {
		t.Fatalf("Next: want track %d, ok=true; got %d ok=%v", id, tr.ID, ok)
	}
	if got := jb.Request("sess1", id); got != RecentlyPlayed {
		t.Fatalf("recent request: want RecentlyPlayed, got %v", got)
	}
}

func TestNextEmptyQueue(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	jb := New(db, Config{HistoryWindow: 5})
	jb.EnsureStream("s", "private")
	if _, ok := jb.Next("s"); ok {
		t.Fatalf("expected ok=false on empty queue")
	}
}
```

- [ ] **Step 2: Run the test, expect failure**

Run: `go test ./internal/jukebox/`
Expected: FAIL — `undefined: New`

- [ ] **Step 3: Implement queue persistence helpers**

`internal/store/queue.go`:

```go
package store

import "database/sql"

// EnsureStream creates the stream row if absent.
func EnsureStream(db *sql.DB, id, name, kind string) error {
	_, err := db.Exec(
		`INSERT INTO stream(id, name, kind) VALUES(?,?,?)
		 ON CONFLICT(id) DO NOTHING`, id, name, kind)
	return err
}

// InQueue reports whether a track is already queued in the stream.
func InQueue(db *sql.DB, streamID string, trackID int64) (bool, error) {
	var n int
	err := db.QueryRow(
		`SELECT count(*) FROM queue_item WHERE stream_id=? AND track_id=?`,
		streamID, trackID).Scan(&n)
	return n > 0, err
}

// RecentlyPlayed reports whether a track is within the last `window` plays of
// the stream's history.
func RecentlyPlayed(db *sql.DB, streamID string, trackID int64, window int) (bool, error) {
	var n int
	err := db.QueryRow(
		`SELECT count(*) FROM (
		    SELECT track_id FROM history WHERE stream_id=?
		    ORDER BY played_at DESC LIMIT ?
		 ) WHERE track_id=?`, streamID, window, trackID).Scan(&n)
	return n > 0, err
}

// Enqueue appends a track to the end of the stream's queue.
func Enqueue(db *sql.DB, streamID string, trackID int64, addedBy string) error {
	var next int
	db.QueryRow(
		`SELECT coalesce(max(play_order),0)+1 FROM queue_item WHERE stream_id=?`,
		streamID).Scan(&next)
	_, err := db.Exec(
		`INSERT INTO queue_item(stream_id, track_id, play_order, added_by) VALUES(?,?,?,?)`,
		streamID, trackID, next, addedBy)
	return err
}

// PopNext removes and returns the next track id in play order, records it in
// history, and bumps its play count. Returns ok=false if the queue is empty.
func PopNext(db *sql.DB, streamID string) (trackID int64, ok bool) {
	err := db.QueryRow(
		`SELECT track_id FROM queue_item WHERE stream_id=? ORDER BY play_order LIMIT 1`,
		streamID).Scan(&trackID)
	if err != nil {
		return 0, false
	}
	db.Exec(`DELETE FROM queue_item WHERE stream_id=? AND track_id=?`, streamID, trackID)
	db.Exec(`INSERT INTO history(stream_id, track_id, played_at) VALUES(?,?,strftime('%s','now'))`,
		streamID, trackID)
	db.Exec(`UPDATE track SET play_count = play_count + 1 WHERE id=?`, trackID)
	return trackID, true
}

// RemoveFromQueue drops a single track from the stream's queue.
func RemoveFromQueue(db *sql.DB, streamID string, trackID int64) error {
	_, err := db.Exec(
		`DELETE FROM queue_item WHERE stream_id=? AND track_id=?`, streamID, trackID)
	return err
}

// ClearQueue empties the stream's queue.
func ClearQueue(db *sql.DB, streamID string) error {
	_, err := db.Exec(`DELETE FROM queue_item WHERE stream_id=?`, streamID)
	return err
}

// QueueTrackIDs returns the queued track ids in play order.
func QueueTrackIDs(db *sql.DB, streamID string) ([]int64, error) {
	rows, err := db.Query(
		`SELECT track_id FROM queue_item WHERE stream_id=? ORDER BY play_order`, streamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
```

- [ ] **Step 4: Implement the jukebox service**

`internal/jukebox/jukebox.go`:

```go
package jukebox

import (
	"database/sql"

	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

// Result is the outcome of a single track request.
type Result int

const (
	Requested Result = iota
	AlreadyQueued
	RecentlyPlayed
)

func (r Result) Message() string {
	switch r {
	case AlreadyQueued:
		return "That track is already in your queue."
	case RecentlyPlayed:
		return "That track was just played. Try something else."
	default:
		return "Thanks for the request!"
	}
}

// Config holds per-stream fairness tuning.
type Config struct {
	HistoryWindow int // how many recent plays block a re-request
}

// Jukebox applies fairness rules over the store. Safe for concurrent use because
// SQLite serializes writes; callers may share one instance.
type Jukebox struct {
	db  *sql.DB
	cfg Config
}

func New(db *sql.DB, cfg Config) *Jukebox {
	if cfg.HistoryWindow < 0 {
		cfg.HistoryWindow = 0
	}
	return &Jukebox{db: db, cfg: cfg}
}

// EnsureStream creates the stream if it does not yet exist.
func (j *Jukebox) EnsureStream(id, kind string) error {
	return store.EnsureStream(j.db, id, "", kind)
}

// Request applies the fairness rules and enqueues the track if it passes.
func (j *Jukebox) Request(streamID string, trackID int64) Result {
	if dup, _ := store.InQueue(j.db, streamID, trackID); dup {
		return AlreadyQueued
	}
	if j.cfg.HistoryWindow > 0 {
		if recent, _ := store.RecentlyPlayed(j.db, streamID, trackID, j.cfg.HistoryWindow); recent {
			return RecentlyPlayed
		}
	}
	store.Enqueue(j.db, streamID, trackID, streamID)
	return Requested
}

// Next pops the next track in play order. ok=false if the queue is empty.
func (j *Jukebox) Next(streamID string) (model.Track, bool) {
	id, ok := store.PopNext(j.db, streamID)
	if !ok {
		return model.Track{}, false
	}
	tr, _, found := store.GetTrack(j.db, id)
	if !found {
		return model.Track{}, false
	}
	return tr, true
}

// Queue returns the queued tracks in play order.
func (j *Jukebox) Queue(streamID string) ([]model.Track, error) {
	ids, err := store.QueueTrackIDs(j.db, streamID)
	if err != nil {
		return nil, err
	}
	out := make([]model.Track, 0, len(ids))
	for _, id := range ids {
		if tr, _, ok := store.GetTrack(j.db, id); ok {
			out = append(out, tr)
		}
	}
	return out, nil
}

// Remove drops one track from the queue.
func (j *Jukebox) Remove(streamID string, trackID int64) error {
	return store.RemoveFromQueue(j.db, streamID, trackID)
}

// Clear empties the queue.
func (j *Jukebox) Clear(streamID string) error {
	return store.ClearQueue(j.db, streamID)
}
```

- [ ] **Step 5: Run the tests, expect pass**

Run: `go test ./internal/jukebox/ ./internal/store/`
Expected: PASS (both packages)

- [ ] **Step 6: Remove the unused test stub**

Delete the empty `seedTrack` helper from `jukebox_test.go` (it was a leftover; the test does not use it).

- [ ] **Step 7: Commit**

```bash
git add internal/store/queue.go internal/jukebox/
git commit -m "feat: jukebox fairness service (dedup, recently-played, next)"
```

### Task 3.2: Request album / artist (bulk)

**Files:** Modify `internal/jukebox/jukebox.go`, `internal/store/library.go`, `internal/jukebox/jukebox_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/jukebox/jukebox_test.go`:

```go
func TestRequestAlbumQueuesAllTracks(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	store.UpsertTrack(db, model.Track{Path: "/m/1.mp3", Title: "One", TrackNo: 1}, "Band", "LP")
	store.UpsertTrack(db, model.Track{Path: "/m/2.mp3", Title: "Two", TrackNo: 2}, "Band", "LP")

	var albumID int64
	db.QueryRow(`SELECT id FROM album WHERE name='LP'`).Scan(&albumID)

	jb := New(db, Config{HistoryWindow: 5})
	jb.EnsureStream("s", "private")
	n := jb.RequestAlbum("s", albumID)
	if n != 2 {
		t.Fatalf("expected 2 tracks queued, got %d", n)
	}
	q, _ := jb.Queue("s")
	if len(q) != 2 {
		t.Fatalf("expected queue length 2, got %d", len(q))
	}
}
```

- [ ] **Step 2: Run the test, expect failure**

Run: `go test ./internal/jukebox/ -run TestRequestAlbum`
Expected: FAIL — `undefined: ... RequestAlbum`

- [ ] **Step 3: Add the store helpers**

Append to `internal/store/library.go`:

```go
// TrackIDsByAlbum returns track ids for an album in track-number order.
func TrackIDsByAlbum(db *sql.DB, albumID int64) ([]int64, error) {
	return scanIDs(db,
		`SELECT id FROM track WHERE album_id=? ORDER BY track_no, title`, albumID)
}

// TrackIDsByArtist returns track ids for an artist in title order.
func TrackIDsByArtist(db *sql.DB, artistID int64) ([]int64, error) {
	return scanIDs(db,
		`SELECT id FROM track WHERE artist_id=? ORDER BY title`, artistID)
}

func scanIDs(db *sql.DB, q string, arg any) ([]int64, error) {
	rows, err := db.Query(q, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
```

- [ ] **Step 4: Add the jukebox bulk methods**

Append to `internal/jukebox/jukebox.go`:

```go
// RequestAlbum requests every track on an album, returning how many were newly
// queued (tracks rejected by fairness are not counted).
func (j *Jukebox) RequestAlbum(streamID string, albumID int64) int {
	ids, _ := store.TrackIDsByAlbum(j.db, albumID)
	return j.requestMany(streamID, ids)
}

// RequestArtist requests every track by an artist, returning how many were newly
// queued.
func (j *Jukebox) RequestArtist(streamID string, artistID int64) int {
	ids, _ := store.TrackIDsByArtist(j.db, artistID)
	return j.requestMany(streamID, ids)
}

func (j *Jukebox) requestMany(streamID string, ids []int64) int {
	queued := 0
	for _, id := range ids {
		if j.Request(streamID, id) == Requested {
			queued++
		}
	}
	return queued
}
```

- [ ] **Step 5: Run the test, expect pass**

Run: `go test ./internal/jukebox/ -run TestRequestAlbum`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/store/library.go internal/jukebox/jukebox.go internal/jukebox/jukebox_test.go
git commit -m "feat: bulk request album/artist"
```

---

## Phase 4 — HTTP API

### Task 4.1: Server, JSON helpers, config

**Files:** Create `internal/config/config.go`, `internal/api/server.go`, `internal/api/server_test.go`

- [ ] **Step 1: Write config**

`internal/config/config.go`:

```go
package config

import "flag"

// Config holds runtime options sourced from flags.
type Config struct {
	Addr          string
	DBPath        string
	Roots         multiFlag
	HistoryWindow int
	ScanWorkers   int
}

type multiFlag []string

func (m *multiFlag) String() string { return "" }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

// Roots returns the configured library roots.
func (c Config) Library() []string { return c.Roots }

// Parse reads flags from the given argument list.
func Parse(args []string) Config {
	fs := flag.NewFlagSet("exit66", flag.ContinueOnError)
	var c Config
	fs.StringVar(&c.Addr, "addr", ":8066", "listen address")
	fs.StringVar(&c.DBPath, "db", "exit66.db", "SQLite database path")
	fs.IntVar(&c.HistoryWindow, "history", 25, "recently-played window")
	fs.IntVar(&c.ScanWorkers, "workers", 8, "scan worker goroutines")
	fs.Var(&c.Roots, "root", "library root (repeatable)")
	fs.Parse(args)
	return c
}
```

- [ ] **Step 2: Write the failing test**

`internal/api/server_test.go`:

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/jukebox"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	jb := jukebox.New(db, jukebox.Config{HistoryWindow: 5})
	return NewServer(db, jb)
}

func TestArtistsEndpointReturnsJSON(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/artists", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type: want application/json, got %q", ct)
	}
	if rec.Body.String() != "[]\n" && rec.Body.String() != "[]" {
		t.Fatalf("want empty JSON array, got %q", rec.Body.String())
	}
}
```

- [ ] **Step 3: Run the test, expect failure**

Run: `go test ./internal/api/`
Expected: FAIL — `undefined: NewServer`

- [ ] **Step 4: Implement server skeleton + JSON helpers**

`internal/api/server.go`:

```go
package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/andybarilla/exit66jukebox/internal/jukebox"
)

// Server holds dependencies and builds the HTTP handler.
type Server struct {
	db *sql.DB
	jb *jukebox.Jukebox
}

func NewServer(db *sql.DB, jb *jukebox.Jukebox) *Server {
	return &Server{db: db, jb: jb}
}

// Handler returns the routed mux. Routes are registered here; handlers live in
// sibling files (browse.go, streams.go, audio.go).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/artists", s.listArtists)
	mux.HandleFunc("GET /api/albums", s.listAlbums)
	mux.HandleFunc("GET /api/tracks", s.listTracks)
	mux.HandleFunc("GET /api/streams/{id}", s.getStream)
	mux.HandleFunc("GET /api/streams/{id}/next", s.nextTrack)
	mux.HandleFunc("POST /api/streams/{id}/requests", s.request)
	mux.HandleFunc("DELETE /api/streams/{id}/requests/{trackID}", s.removeRequest)
	mux.HandleFunc("DELETE /api/streams/{id}/requests", s.clearRequests)
	mux.HandleFunc("GET /api/tracks/{id}/audio", s.trackAudio)
	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

// queryInt reads an int query parameter, returning def when absent or invalid.
func queryInt(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
```

- [ ] **Step 5: Implement browse handlers**

`internal/api/browse.go`:

```go
package api

import (
	"net/http"

	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

func (s *Server) listArtists(w http.ResponseWriter, r *http.Request) {
	list, err := store.ListArtists(s.db,
		r.URL.Query().Get("search"), queryInt(r, "limit", 50), queryInt(r, "offset", 0))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []model.Artist{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) listAlbums(w http.ResponseWriter, r *http.Request) {
	list, err := store.ListAlbums(s.db,
		r.URL.Query().Get("search"), queryInt(r, "limit", 50), queryInt(r, "offset", 0))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []model.Album{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) listTracks(w http.ResponseWriter, r *http.Request) {
	list, err := store.ListTracks(s.db,
		r.URL.Query().Get("search"), queryInt(r, "limit", 50), queryInt(r, "offset", 0))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []model.Track{}
	}
	writeJSON(w, http.StatusOK, list)
}
```

- [ ] **Step 6: Add stream/audio handler stubs so the package compiles**

`internal/api/streams.go`:

```go
package api

import (
	"net/http"
	"strconv"

	"github.com/andybarilla/exit66jukebox/internal/jukebox"
)

func (s *Server) getStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.jb.EnsureStream(id, "private")
	q, err := s.jb.Queue(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if q == nil {
		q = nil
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "queue": q})
}

func (s *Server) nextTrack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.jb.EnsureStream(id, "private")
	tr, ok := s.jb.Next(id)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "track": tr})
}

func (s *Server) request(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.jb.EnsureStream(id, "private")
	r.ParseForm()
	kind := r.FormValue("kind") // "track" | "album" | "artist"
	targetID, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)

	switch kind {
	case "album":
		n := s.jb.RequestAlbum(id, targetID)
		writeJSON(w, http.StatusOK, map[string]any{"queued": n})
	case "artist":
		n := s.jb.RequestArtist(id, targetID)
		writeJSON(w, http.StatusOK, map[string]any{"queued": n})
	default:
		res := s.jb.Request(id, targetID)
		writeJSON(w, http.StatusOK, map[string]any{
			"queued":  res == jukebox.Requested,
			"message": res.Message(),
		})
	}
}

func (s *Server) removeRequest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	trackID, _ := strconv.ParseInt(r.PathValue("trackID"), 10, 64)
	if err := s.jb.Remove(id, trackID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) clearRequests(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.jb.Clear(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
```

`internal/api/audio.go`:

```go
package api

import (
	"net/http"
	"strconv"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

func (s *Server) trackAudio(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	_, path, ok := store.GetTrack(s.db, id)
	if !ok {
		writeErr(w, http.StatusNotFound, "track not found")
		return
	}
	http.ServeFile(w, r, path) // sets type + supports Range for <audio> seeking
}
```

- [ ] **Step 7: Run the test, expect pass**

Run: `go test ./internal/api/`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/config/config.go internal/api/
git commit -m "feat: HTTP API — browse, stream, request, audio endpoints"
```

### Task 4.2: Request endpoint integration test

**Files:** Modify `internal/api/server_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/api/server_test.go`:

```go
import "net/url"

func TestRequestThenNextRoundTrip(t *testing.T) {
	srv := newTestServer(t)
	id, _ := store.UpsertTrack(srv.db,
		modelTrack("/m/a.mp3", "Hello"), "Band", "Album")

	// Request the track.
	form := url.Values{"kind": {"track"}, "id": {itoa(id)}}
	req := httptest.NewRequest(http.MethodPost, "/api/streams/sess/requests",
		stringsReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("request status: %d", rec.Code)
	}

	// Pop it back via next.
	req2 := httptest.NewRequest(http.MethodGet, "/api/streams/sess/next", nil)
	rec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("next status: %d", rec2.Code)
	}
	if !contains(rec2.Body.String(), "\"ok\":true") {
		t.Fatalf("expected ok:true, got %s", rec2.Body.String())
	}
}
```

- [ ] **Step 2: Add the small test helpers**

Append to `internal/api/server_test.go`:

```go
import (
	"strconv"
	"strings"

	"github.com/andybarilla/exit66jukebox/internal/model"
)

func modelTrack(path, title string) model.Track {
	return model.Track{Path: path, Title: title}
}
func itoa(n int64) string              { return strconv.FormatInt(n, 10) }
func stringsReader(s string) *strings.Reader { return strings.NewReader(s) }
func contains(haystack, needle string) bool  { return strings.Contains(haystack, needle) }
```

> Consolidate the `import` blocks at the top of the test file rather than leaving multiple `import` statements; Go allows only grouped imports per file.

- [ ] **Step 3: Run the test, expect pass**

Run: `go test ./internal/api/ -run TestRequestThenNext`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/api/server_test.go
git commit -m "test: request->next API round-trip"
```

---

## Phase 5 — Wire-up: main + UI embed

### Task 5.1: main.go daemon

**Files:** Create `main.go`

- [ ] **Step 1: Write main**

`main.go`:

```go
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/andybarilla/exit66jukebox/internal/api"
	"github.com/andybarilla/exit66jukebox/internal/config"
	"github.com/andybarilla/exit66jukebox/internal/jukebox"
	"github.com/andybarilla/exit66jukebox/internal/scan"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

func main() {
	cfg := config.Parse(os.Args[1:])

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	jb := jukebox.New(db, jukebox.Config{HistoryWindow: cfg.HistoryWindow})

	// Initial scan in the background so the server comes up immediately.
	if roots := cfg.Library(); len(roots) > 0 {
		go func() {
			log.Printf("scanning %v ...", roots)
			res, err := scan.Scan(db, roots, cfg.ScanWorkers)
			if err != nil {
				log.Printf("scan error: %v", err)
				return
			}
			log.Printf("scan done: added=%d updated=%d skipped=%d failed=%d",
				res.Added, res.Updated, res.Skipped, res.Failed)
		}()
	}

	srv := api.NewServer(db, jb)
	log.Printf("Exit 66 Jukebox listening on %s", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, srv.Handler()); err != nil {
		log.Fatalf("server: %v", err)
	}
}
```

- [ ] **Step 2: Build and smoke-test**

```bash
go build -o exit66jukebox .
./exit66jukebox -root /path/to/some/music -db /tmp/exit66.db &
sleep 2
curl -s localhost:8066/api/tracks | head -c 200
kill %1
```

Expected: a JSON array (possibly empty if the scan is still running), no crash.

- [ ] **Step 3: Commit**

```bash
git add main.go
git commit -m "feat: main daemon — background scan + HTTP server"
```

### Task 5.2: Svelte UI scaffold

**Files:** Create `web/` (Svelte + Vite project)

- [ ] **Step 1: Scaffold the Vite/Svelte app**

```bash
cd web
npm create vite@latest . -- --template svelte
npm install
```

- [ ] **Step 2: Configure the build output and dev proxy**

Edit `web/vite.config.js` so production builds emit to `web/dist` and dev proxies the API to the Go server:

```js
import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

export default defineConfig({
  plugins: [svelte()],
  build: { outDir: 'dist', emptyOutDir: true },
  server: { proxy: { '/api': 'http://localhost:8066' } },
})
```

- [ ] **Step 3: Commit the scaffold**

```bash
cd ..
git add web/ .gitignore
git commit -m "chore: scaffold Svelte/Vite web UI"
```

### Task 5.3: Minimal jukebox UI

**Files:** Replace `web/src/App.svelte`; create `web/src/lib/api.js`, `web/src/lib/player.js`

- [ ] **Step 1: API client**

`web/src/lib/api.js`:

```js
const SESSION = 'me'; // single private stream id for v1; replaced by real session later

export async function listArtists(search = '') {
  const r = await fetch(`/api/artists?search=${encodeURIComponent(search)}`);
  return r.json();
}
export async function listTracks(search = '') {
  const r = await fetch(`/api/tracks?search=${encodeURIComponent(search)}`);
  return r.json();
}
export async function requestTrack(trackId) {
  const body = new URLSearchParams({ kind: 'track', id: String(trackId) });
  const r = await fetch(`/api/streams/${SESSION}/requests`, { method: 'POST', body });
  return r.json();
}
export async function nextTrack() {
  const r = await fetch(`/api/streams/${SESSION}/next`);
  return r.json();
}
export function audioURL(trackId) {
  return `/api/tracks/${trackId}/audio`;
}
```

- [ ] **Step 2: Player that pulls from the queue**

`web/src/lib/player.js`:

```js
import { nextTrack, audioURL } from './api.js';

// Player wires an <audio> element to the private queue: when a track ends, it
// fetches the next one. Returns a start() that kicks off playback.
export function createPlayer(audio, onNowPlaying) {
  async function playNext() {
    const res = await nextTrack();
    if (res.ok) {
      audio.src = audioURL(res.track.id);
      audio.play();
      onNowPlaying(res.track);
    } else {
      onNowPlaying(null);
      setTimeout(playNext, 2000); // queue empty: poll
    }
  }
  audio.addEventListener('ended', playNext);
  return { start: playNext };
}
```

- [ ] **Step 3: App shell**

`web/src/App.svelte`:

```svelte
<script>
  import { onMount } from 'svelte';
  import { listTracks, requestTrack } from './lib/api.js';
  import { createPlayer } from './lib/player.js';

  let search = '';
  let tracks = [];
  let nowPlaying = null;
  let audio;

  async function doSearch() { tracks = await listTracks(search); }
  async function add(t) { await requestTrack(t.id); }

  onMount(async () => {
    tracks = await listTracks('');
    const player = createPlayer(audio, (t) => (nowPlaying = t));
    player.start();
  });
</script>

<main>
  <h1>Exit 66 Jukebox</h1>
  <p class="now">{nowPlaying ? `Now playing: ${nowPlaying.title}` : 'Queue empty — request something'}</p>

  <input bind:value={search} on:keydown={(e) => e.key === 'Enter' && doSearch()} placeholder="Search songs" />
  <button on:click={doSearch}>Search</button>

  <ul>
    {#each tracks as t (t.id)}
      <li><button on:click={() => add(t)}>＋</button> {t.title}</li>
    {/each}
  </ul>

  <audio bind:this={audio} controls></audio>
</main>

<style>
  main { font-family: system-ui; max-width: 640px; margin: 2rem auto; color: #eee; background: #181818; padding: 1.5rem; border-radius: 8px; }
  .now { color: #6cf; }
  li { list-style: none; margin: .25rem 0; }
  button { cursor: pointer; }
</style>
```

- [ ] **Step 4: Manual verification**

```bash
# terminal 1
./exit66jukebox -root /path/to/music -db /tmp/exit66.db
# terminal 2
cd web && npm run dev
```

Open the dev URL, search a song, click ＋, confirm it plays and "Now playing" updates. When it ends, the next queued track should auto-play.

- [ ] **Step 5: Commit**

```bash
git add web/src/
git commit -m "feat: minimal Svelte jukebox UI (browse, request, play queue)"
```

### Task 5.4: Embed the built UI in the binary

**Files:** Create `internal/web/embed.go`; modify `internal/api/server.go`, `main.go`

- [ ] **Step 1: Build the UI**

```bash
cd web && npm run build && cd ..
```

Expected: `web/dist/index.html` and assets exist.

- [ ] **Step 2: Add the embed**

`internal/web/embed.go`:

```go
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the built UI rooted at dist/.
func FS() (fs.FS, error) {
	return fs.Sub(dist, "dist")
}
```

- [ ] **Step 3: Serve the UI as the fallback route**

In `internal/api/server.go`, change `NewServer` to accept the UI filesystem and register a catch-all. Replace the `NewServer` func and add to `Handler`:

```go
// add import: "io/fs"

type Server struct {
	db *sql.DB
	jb *jukebox.Jukebox
	ui fs.FS
}

func NewServer(db *sql.DB, jb *jukebox.Jukebox, ui fs.FS) *Server {
	return &Server{db: db, jb: jb, ui: ui}
}
```

At the end of `Handler()`, before `return mux`, add the static fallback:

```go
	if s.ui != nil {
		mux.Handle("GET /", http.FileServerFS(s.ui))
	}
```

- [ ] **Step 4: Update the test constructor and main**

In `internal/api/server_test.go`, update `NewServer(db, jb)` calls to `NewServer(db, jb, nil)`.

In `main.go`, build the UI FS and pass it:

```go
	// add import: "github.com/andybarilla/exit66jukebox/internal/web"
	uiFS, err := web.FS()
	if err != nil {
		log.Fatalf("ui fs: %v", err)
	}
	srv := api.NewServer(db, jb, uiFS)
```

- [ ] **Step 5: Build and verify single-binary serving**

```bash
go build -o exit66jukebox .
./exit66jukebox -root /path/to/music -db /tmp/exit66.db &
sleep 2
curl -s localhost:8066/ | grep -qi "<!doctype html" && echo "UI served"
kill %1
```

Expected: prints `UI served`.

- [ ] **Step 6: Run the whole suite**

Run: `go test ./...`
Expected: PASS across all packages.

- [ ] **Step 7: Commit**

```bash
git add internal/web/embed.go internal/api/server.go internal/api/server_test.go main.go
git commit -m "feat: embed built UI into the single binary"
```

---

## Self-Review notes (addressed)

- **Spec coverage:** library scan (Phase 2), incremental/concurrent/batched (Task 2.2), fairness dedup + recently-played + bulk (Phase 3), browse API + paging + search (Phase 4), private playback via `next` + file serving (Phases 4–5), SQLite data model (Phase 1), single binary + embedded UI (Task 5.4). **Deferred to Plan 2 by design:** shared-stream FFmpeg broadcaster, `/stream/{id}.mp3`, SSE push, cover-art endpoints, shuffle-top-N. These are listed here so the gap is explicit, not accidental.
- **Type consistency:** `model.Track`/`Artist`/`Album` reused everywhere; `jukebox.Result` values (`Requested`/`AlreadyQueued`/`RecentlyPlayed`) consistent across service, tests, and API; store signatures `(db, streamID, trackID, ...)` uniform.
- **Cover art:** `album.cover` column exists in the schema but no endpoint ships in Plan 1; the UI shows no art yet. Pulled forward into Plan 2 with the stream UI work.

## Definition of done for Plan 1

`go test ./...` passes; `go build` produces one binary; running it against a real music folder lets a browser search, queue (with dedup + recently-played enforcement), and auto-play tracks end to end. Shared streams and live updates follow in Plan 2.
