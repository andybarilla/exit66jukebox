# Music Discovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add local music discovery — Rediscover and Recently-Added browse lists, plus continuous genre "radio" stations that auto-refill a stream's queue — built only on the existing library + play stats.

**Architecture:** A single parameterized store query (`DiscoverTracks`) ranks tracks by play stats for all three surfaces. Genre stations persist in a new `station` table; refill is pull-driven inside `jukebox.Next()` (no background goroutines), with an immediate fill on station start so empty queues begin playing. A guarded migration adds `track.added_at` to existing databases.

**Tech Stack:** Go 1.26 stdlib + `modernc.org/sqlite`, existing `internal/store`, `internal/jukebox`, `internal/api` packages; Svelte UI in `web/src/`.

**Spec:** `docs/superpowers/specs/2026-06-11-music-discovery-design.md`

---

## File Structure

- `internal/store/schema.sql` (modify) — add `added_at` to `track`; add `station` table.
- `internal/store/store.go` (modify) — run `migrate(db)` after schema apply.
- `internal/store/migrate.go` (create) — guarded `added_at` migration + `columnExists` helper.
- `internal/store/library.go` (modify) — `UpsertTrack` writes `added_at` on insert.
- `internal/store/discover.go` (create) — `DiscoverTracks`, `GenreCounts`.
- `internal/store/station.go` (create) — `Station` struct, `GetStation`, `UpsertStation`, `DeleteStation`, `QueueLen`.
- `internal/jukebox/jukebox.go` (modify) — `StartStation`, `StopStation`, `GetStation`, refill in `Next`.
- `internal/api/discover.go` (create) — discover + station HTTP handlers.
- `internal/api/server.go` (modify) — register routes.
- `web/src/lib/api.js` (modify) — discover/station client functions.
- `web/src/App.svelte` (modify) — Discover view.

Tests live beside their packages: `discover_test.go`, `station_test.go`, `migrate_test.go` (store), `jukebox_test.go` (jukebox), `discover_test.go` (api).

---

## Task 1: Add `added_at` to the schema

**Files:**
- Modify: `internal/store/schema.sql`

- [ ] **Step 1: Add `added_at` column to the `track` CREATE statement**

In `internal/store/schema.sql`, change the `track` table definition to add an `added_at` column after `play_count`:

```sql
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
    play_count INTEGER NOT NULL DEFAULT 0,
    added_at   INTEGER NOT NULL DEFAULT 0
);
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: builds cleanly (schema is embedded; no Go change yet).

- [ ] **Step 3: Commit**

```bash
git add internal/store/schema.sql
git commit -m "feat: add track.added_at column to schema"
```

---

## Task 2: Guarded migration for existing databases

A fresh DB gets `added_at` from the CREATE statement, but `CREATE TABLE IF NOT EXISTS` skips existing tables — so existing databases need an `ALTER TABLE`.

**Files:**
- Create: `internal/store/migrate.go`
- Modify: `internal/store/store.go:38` (the `if _, err := db.Exec(schema)` block in `Open`)
- Test: `internal/store/migrate_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/store/migrate_test.go`:

```go
package store

import "testing"

func TestMigrateAddsAddedAtToOldDB(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	// Simulate an old DB: drop added_at by recreating track without it is hard
	// in-place, so instead verify the column exists and migrate is idempotent.
	has, err := columnExists(db, "track", "added_at")
	if err != nil {
		t.Fatalf("columnExists: %v", err)
	}
	if !has {
		t.Fatalf("expected added_at column to exist after Open")
	}

	// Running migrate again must be a no-op (idempotent).
	if err := migrate(db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestMigrateBackfillsFromModTime(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	// Insert a track, then zero its added_at to mimic a pre-migration row.
	if _, err := db.Exec(`INSERT INTO artist(name) VALUES('A')`); err != nil {
		t.Fatalf("artist: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO album(name, artist_id) VALUES('X', 1)`); err != nil {
		t.Fatalf("album: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO track(path, mod_time, size, title, artist_id, album_id, added_at)
		 VALUES('/m/a.mp3', 555, 10, 'A', 1, 1, 0)`); err != nil {
		t.Fatalf("track: %v", err)
	}

	if err := migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var addedAt int64
	if err := db.QueryRow(`SELECT added_at FROM track WHERE path='/m/a.mp3'`).Scan(&addedAt); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if addedAt != 555 {
		t.Fatalf("expected added_at backfilled to mod_time 555, got %d", addedAt)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestMigrate -v`
Expected: FAIL — `columnExists`/`migrate` undefined.

- [ ] **Step 3: Write `migrate.go`**

Create `internal/store/migrate.go`:

```go
package store

import "database/sql"

// columnExists reports whether a column is present on a table.
func columnExists(db *sql.DB, table, col string) (bool, error) {
	var n int
	err := db.QueryRow(
		`SELECT count(*) FROM pragma_table_info(?) WHERE name = ?`, table, col,
	).Scan(&n)
	return n > 0, err
}

// migrate brings an existing database up to the current schema. It is
// idempotent: safe to run on every Open. CREATE TABLE IF NOT EXISTS in
// schema.sql cannot add columns to a pre-existing table, so additive column
// changes are applied here.
func migrate(db *sql.DB) error {
	has, err := columnExists(db, "track", "added_at")
	if err != nil {
		return err
	}
	if !has {
		if _, err := db.Exec(`ALTER TABLE track ADD COLUMN added_at INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	// Backfill any rows that predate added_at (value 0) from mod_time.
	if _, err := db.Exec(`UPDATE track SET added_at = mod_time WHERE added_at = 0`); err != nil {
		return err
	}
	return nil
}
```

- [ ] **Step 4: Call `migrate` from `Open`**

In `internal/store/store.go`, after the schema `db.Exec(schema)` block succeeds and before `return db, nil`, add:

```go
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/store/ -run TestMigrate -v`
Expected: PASS (both).

- [ ] **Step 6: Run the full store suite (catch backfill side effects)**

Run: `go test ./internal/store/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/store/migrate.go internal/store/migrate_test.go internal/store/store.go
git commit -m "feat: migrate existing DBs to add track.added_at"
```

---

## Task 3: Stamp `added_at` on insert in `UpsertTrack`

**Files:**
- Modify: `internal/store/library.go:56-65` (the `UpsertTrack` INSERT)
- Test: `internal/store/discover_test.go` (created here, extended in Task 4)

- [ ] **Step 1: Write the failing test**

Create `internal/store/discover_test.go`:

```go
package store

import (
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/model"
)

func TestUpsertStampsAddedAt(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()

	id, err := UpsertTrack(db, model.Track{Path: "/m/a.mp3", Title: "A"}, "Band", "Album")
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	var addedAt int64
	if err := db.QueryRow(`SELECT added_at FROM track WHERE id=?`, id).Scan(&addedAt); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if addedAt <= 0 {
		t.Fatalf("expected added_at to be stamped on insert, got %d", addedAt)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestUpsertStampsAddedAt -v`
Expected: FAIL — `added_at` is 0 (default), not stamped.

- [ ] **Step 3: Update the INSERT in `UpsertTrack`**

In `internal/store/library.go`, replace the INSERT statement inside `UpsertTrack` with one that stamps `added_at` on insert and preserves it on conflict (note `added_at` is intentionally absent from the `DO UPDATE SET` list):

```go
	_, err = tx.Exec(
		`INSERT INTO track(path, mod_time, size, title, artist_id, album_id, track_no, genre, duration, added_at)
		 VALUES(?,?,?,?,?,?,?,?,?, strftime('%s','now'))
		 ON CONFLICT(path) DO UPDATE SET
		   mod_time=excluded.mod_time, size=excluded.size, title=excluded.title,
		   artist_id=excluded.artist_id, album_id=excluded.album_id,
		   track_no=excluded.track_no, genre=excluded.genre, duration=excluded.duration`,
		t.Path, t.ModTime, t.Size, t.Title, artistID, albumID, t.TrackNo, t.Genre, t.Duration,
	)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestUpsertStampsAddedAt -v`
Expected: PASS.

- [ ] **Step 5: Confirm idempotent re-upsert preserves added_at**

Run: `go test ./internal/store/ -run TestUpsertTrackIsIdempotent -v`
Expected: PASS (existing test still green; `added_at` not in UPDATE set so re-upsert keeps the original).

- [ ] **Step 6: Commit**

```bash
git add internal/store/library.go internal/store/discover_test.go
git commit -m "feat: stamp added_at when inserting tracks"
```

---

## Task 4: The selection seam — `DiscoverTracks` + `GenreCounts`

**Files:**
- Create: `internal/store/discover.go`
- Test: `internal/store/discover_test.go` (extend)

- [ ] **Step 1: Write the failing tests**

Append to `internal/store/discover_test.go`:

```go
func seedTrack(t *testing.T, db *sql.DB, path, title, genre string, playCount int) int64 {
	t.Helper()
	id, err := UpsertTrack(db, model.Track{Path: path, Title: title, Genre: genre}, "Band", "Album")
	if err != nil {
		t.Fatalf("seed %s: %v", path, err)
	}
	if _, err := db.Exec(`UPDATE track SET play_count=? WHERE id=?`, playCount, id); err != nil {
		t.Fatalf("set play_count: %v", err)
	}
	return id
}

func TestDiscoverRediscoverOrdersByPlayCount(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	low := seedTrack(t, db, "/m/low.mp3", "Low", "Rock", 0)
	seedTrack(t, db, "/m/high.mp3", "High", "Rock", 50)

	got, err := DiscoverTracks(db, DiscoverOpts{OrderBy: "rediscover", Limit: 10})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(got) != 2 || got[0].ID != low {
		t.Fatalf("expected least-played track first, got %+v", got)
	}
}

func TestDiscoverRecentOrdersByAddedAt(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	seedTrack(t, db, "/m/old.mp3", "Old", "Rock", 0)
	newID := seedTrack(t, db, "/m/new.mp3", "New", "Rock", 0)
	// Force ordering: make old older, new newer.
	db.Exec(`UPDATE track SET added_at=100 WHERE path='/m/old.mp3'`)
	db.Exec(`UPDATE track SET added_at=200 WHERE id=?`, newID)

	got, err := DiscoverTracks(db, DiscoverOpts{OrderBy: "recent", Limit: 10})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(got) != 2 || got[0].ID != newID {
		t.Fatalf("expected newest first, got %+v", got)
	}
}

func TestDiscoverGenreFilter(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	rock := seedTrack(t, db, "/m/r.mp3", "R", "Rock", 0)
	seedTrack(t, db, "/m/j.mp3", "J", "Jazz", 0)

	got, err := DiscoverTracks(db, DiscoverOpts{OrderBy: "rediscover", Genre: "Rock", Limit: 10})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(got) != 1 || got[0].ID != rock {
		t.Fatalf("expected only the Rock track, got %+v", got)
	}
}

func TestDiscoverExcludeStreamSkipsRecentHistory(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	played := seedTrack(t, db, "/m/p.mp3", "P", "Rock", 0)
	fresh := seedTrack(t, db, "/m/f.mp3", "F", "Rock", 0)
	// Mark `played` as recently played on stream "s".
	db.Exec(`INSERT INTO history(stream_id, track_id, played_at) VALUES('s', ?, 999)`, played)

	got, err := DiscoverTracks(db, DiscoverOpts{
		OrderBy: "random", Genre: "Rock", ExcludeStream: "s", Window: 5, Limit: 10,
	})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(got) != 1 || got[0].ID != fresh {
		t.Fatalf("expected recently-played track excluded, got %+v", got)
	}
}

func TestGenreCounts(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	seedTrack(t, db, "/m/r1.mp3", "R1", "Rock", 0)
	seedTrack(t, db, "/m/r2.mp3", "R2", "Rock", 0)
	seedTrack(t, db, "/m/j1.mp3", "J1", "Jazz", 0)
	seedTrack(t, db, "/m/blank.mp3", "B", "", 0)

	got, err := GenreCounts(db)
	if err != nil {
		t.Fatalf("genres: %v", err)
	}
	// Empty-genre tracks are excluded; Rock=2, Jazz=1, ordered by name.
	if len(got) != 2 || got[0].Genre != "Jazz" || got[0].Count != 1 ||
		got[1].Genre != "Rock" || got[1].Count != 2 {
		t.Fatalf("unexpected genre counts: %+v", got)
	}
}
```

Add `"database/sql"` to the test file's imports (used by `seedTrack`).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/store/ -run 'TestDiscover|TestGenreCounts' -v`
Expected: FAIL — `DiscoverTracks`, `DiscoverOpts`, `GenreCounts` undefined.

- [ ] **Step 3: Write `discover.go`**

Create `internal/store/discover.go`:

```go
package store

import (
	"database/sql"
	"fmt"

	"github.com/andybarilla/exit66jukebox/internal/model"
)

// DiscoverOpts parameterizes the discovery selection query.
type DiscoverOpts struct {
	Genre         string // "" = all genres
	OrderBy       string // "rediscover" | "recent" | "random"
	ExcludeStream string // "" = no exclusion; otherwise skip this stream's recent history
	Window        int    // size of the recent-history window for ExcludeStream
	Limit, Offset int
}

// GenreCount is a genre and how many tracks carry it.
type GenreCount struct {
	Genre string `json:"genre"`
	Count int    `json:"count"`
}

// DiscoverTracks ranks/filters tracks by play stats for the discovery surfaces.
// last_played is MAX(history.played_at) across all streams (0 = never played).
func DiscoverTracks(db *sql.DB, opts DiscoverOpts) ([]model.Track, error) {
	var order string
	switch opts.OrderBy {
	case "recent":
		order = "t.added_at DESC, t.id DESC"
	case "random":
		order = "RANDOM()"
	default: // "rediscover"
		order = "t.play_count ASC, last_played ASC, t.id ASC"
	}

	args := []any{}
	where := "WHERE 1=1"
	if opts.Genre != "" {
		where += " AND t.genre = ?"
		args = append(args, opts.Genre)
	}
	if opts.ExcludeStream != "" {
		where += ` AND t.id NOT IN (
			SELECT track_id FROM history WHERE stream_id = ?
			ORDER BY played_at DESC LIMIT ?
		)`
		args = append(args, opts.ExcludeStream, opts.Window)
	}

	lim := opts.Limit
	if lim <= 0 {
		lim = -1
	}
	args = append(args, lim, opts.Offset)

	q := fmt.Sprintf(`
		SELECT t.id, t.title, t.artist_id, t.album_id, t.track_no, t.genre,
		       t.duration, t.play_count,
		       coalesce((SELECT MAX(h.played_at) FROM history h WHERE h.track_id = t.id), 0) AS last_played
		FROM track t
		%s
		ORDER BY %s
		LIMIT ? OFFSET ?`, where, order)

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Track
	for rows.Next() {
		var t model.Track
		var lastPlayed int64
		if err := rows.Scan(&t.ID, &t.Title, &t.ArtistID, &t.AlbumID, &t.TrackNo,
			&t.Genre, &t.Duration, &t.PlayCount, &lastPlayed); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GenreCounts returns non-empty genres with their track counts, ordered by name.
func GenreCounts(db *sql.DB) ([]GenreCount, error) {
	rows, err := db.Query(
		`SELECT genre, count(*) FROM track WHERE genre <> '' GROUP BY genre ORDER BY genre`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GenreCount
	for rows.Next() {
		var g GenreCount
		if err := rows.Scan(&g.Genre, &g.Count); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/store/ -run 'TestDiscover|TestGenreCounts' -v`
Expected: PASS (all).

- [ ] **Step 5: Commit**

```bash
git add internal/store/discover.go internal/store/discover_test.go
git commit -m "feat: DiscoverTracks selection seam + GenreCounts"
```

---

## Task 5: Station persistence + `QueueLen`

**Files:**
- Modify: `internal/store/schema.sql` (add `station` table)
- Create: `internal/store/station.go`
- Test: `internal/store/station_test.go`

- [ ] **Step 1: Add the `station` table to the schema**

Append to `internal/store/schema.sql`:

```sql
CREATE TABLE IF NOT EXISTS station (
    stream_id TEXT PRIMARY KEY REFERENCES stream(id),
    genre     TEXT NOT NULL,
    threshold INTEGER NOT NULL,
    batch     INTEGER NOT NULL
);
```

- [ ] **Step 2: Write the failing test**

Create `internal/store/station_test.go`:

```go
package store

import "testing"

func TestStationRoundTrip(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	if err := EnsureStream(db, "s", "", "private"); err != nil {
		t.Fatalf("ensure stream: %v", err)
	}

	if _, ok := GetStation(db, "s"); ok {
		t.Fatalf("expected no station initially")
	}

	if err := UpsertStation(db, Station{StreamID: "s", Genre: "Rock", Threshold: 3, Batch: 10}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	st, ok := GetStation(db, "s")
	if !ok || st.Genre != "Rock" || st.Threshold != 3 || st.Batch != 10 {
		t.Fatalf("unexpected station: %+v ok=%v", st, ok)
	}

	// Upsert again changes genre in place.
	if err := UpsertStation(db, Station{StreamID: "s", Genre: "Jazz", Threshold: 3, Batch: 10}); err != nil {
		t.Fatalf("upsert 2: %v", err)
	}
	st, _ = GetStation(db, "s")
	if st.Genre != "Jazz" {
		t.Fatalf("expected genre updated to Jazz, got %q", st.Genre)
	}

	if err := DeleteStation(db, "s"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := GetStation(db, "s"); ok {
		t.Fatalf("expected station gone after delete")
	}
}

func TestQueueLen(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	EnsureStream(db, "s", "", "private")
	if n, _ := QueueLen(db, "s"); n != 0 {
		t.Fatalf("expected empty queue len 0, got %d", n)
	}
	id, _ := UpsertTrack(db, model.Track{Path: "/m/a.mp3", Title: "A"}, "B", "X")
	if err := Enqueue(db, "s", id, ""); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if n, _ := QueueLen(db, "s"); n != 1 {
		t.Fatalf("expected queue len 1, got %d", n)
	}
}
```

Add the import `"github.com/andybarilla/exit66jukebox/internal/model"` to `station_test.go` (used by `TestQueueLen`).

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/store/ -run 'TestStation|TestQueueLen' -v`
Expected: FAIL — `Station`, `GetStation`, `UpsertStation`, `DeleteStation`, `QueueLen` undefined.

- [ ] **Step 4: Write `station.go`**

Create `internal/store/station.go`:

```go
package store

import "database/sql"

// Station is a continuous genre radio attached to one stream. When the stream's
// queue falls below Threshold, Batch more tracks of Genre are enqueued.
type Station struct {
	StreamID  string `json:"stream_id"`
	Genre     string `json:"genre"`
	Threshold int    `json:"threshold"`
	Batch     int    `json:"batch"`
}

// GetStation returns the station for a stream, ok=false if none is set.
func GetStation(db *sql.DB, streamID string) (Station, bool) {
	var s Station
	err := db.QueryRow(
		`SELECT stream_id, genre, threshold, batch FROM station WHERE stream_id = ?`,
		streamID).Scan(&s.StreamID, &s.Genre, &s.Threshold, &s.Batch)
	if err != nil {
		return Station{}, false
	}
	return s, true
}

// UpsertStation creates or replaces the station for a stream.
func UpsertStation(db *sql.DB, s Station) error {
	_, err := db.Exec(
		`INSERT INTO station(stream_id, genre, threshold, batch) VALUES(?,?,?,?)
		 ON CONFLICT(stream_id) DO UPDATE SET
		   genre=excluded.genre, threshold=excluded.threshold, batch=excluded.batch`,
		s.StreamID, s.Genre, s.Threshold, s.Batch)
	return err
}

// DeleteStation removes a stream's station, halting future refills.
func DeleteStation(db *sql.DB, streamID string) error {
	_, err := db.Exec(`DELETE FROM station WHERE stream_id = ?`, streamID)
	return err
}

// QueueLen returns the number of tracks currently queued on a stream.
func QueueLen(db *sql.DB, streamID string) (int, error) {
	var n int
	err := db.QueryRow(
		`SELECT count(*) FROM queue_item WHERE stream_id = ?`, streamID).Scan(&n)
	return n, err
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/store/ -run 'TestStation|TestQueueLen' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/store/schema.sql internal/store/station.go internal/store/station_test.go
git commit -m "feat: station table + store helpers"
```

---

## Task 6: Jukebox station control + refill in `Next`

**Files:**
- Modify: `internal/jukebox/jukebox.go`
- Test: `internal/jukebox/jukebox_test.go` (append)

- [ ] **Step 1: Write the failing tests**

Append to `internal/jukebox/jukebox_test.go`:

```go
func TestStartStationFillsEmptyQueue(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	jb := New(db, Config{HistoryWindow: 5})
	jb.EnsureStream("s", "private")
	for i := 0; i < 20; i++ {
		store.UpsertTrack(db, model.Track{
			Path: fmt.Sprintf("/m/%d.mp3", i), Title: fmt.Sprintf("T%d", i), Genre: "Rock",
		}, "Band", "Album")
	}

	if err := jb.StartStation("s", "Rock", 3, 10); err != nil {
		t.Fatalf("start: %v", err)
	}
	q, _ := jb.Queue("s")
	if len(q) != 10 {
		t.Fatalf("expected immediate fill of 10, got %d", len(q))
	}
}

func TestNextRefillsBelowThreshold(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	jb := New(db, Config{HistoryWindow: 0}) // disable fairness to keep counting simple
	jb.EnsureStream("s", "private")
	for i := 0; i < 30; i++ {
		store.UpsertTrack(db, model.Track{
			Path: fmt.Sprintf("/m/%d.mp3", i), Title: fmt.Sprintf("T%d", i), Genre: "Rock",
		}, "Band", "Album")
	}
	jb.StartStation("s", "Rock", 3, 10) // fills to 10

	// Pop down to the threshold boundary. After popping to <3, Next refills.
	for i := 0; i < 8; i++ {
		if _, ok := jb.Next("s"); !ok {
			t.Fatalf("unexpected empty queue at pop %d", i)
		}
	}
	n, _ := store.QueueLen(db, "s")
	if n < 3 {
		t.Fatalf("expected queue refilled to >=3 after draining, got %d", n)
	}
}

func TestStopStationStopsRefill(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	jb := New(db, Config{HistoryWindow: 0})
	jb.EnsureStream("s", "private")
	for i := 0; i < 30; i++ {
		store.UpsertTrack(db, model.Track{
			Path: fmt.Sprintf("/m/%d.mp3", i), Title: fmt.Sprintf("T%d", i), Genre: "Rock",
		}, "Band", "Album")
	}
	jb.StartStation("s", "Rock", 3, 10)
	if err := jb.StopStation("s"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	// Drain fully; with no station, queue must reach 0 and stay there.
	for {
		if _, ok := jb.Next("s"); !ok {
			break
		}
	}
	n, _ := store.QueueLen(db, "s")
	if n != 0 {
		t.Fatalf("expected drained queue to stay empty after stop, got %d", n)
	}
}
```

Ensure `jukebox_test.go` imports `"fmt"` and `"github.com/andybarilla/exit66jukebox/internal/model"` (add if missing).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/jukebox/ -run 'Station|Refill' -v`
Expected: FAIL — `StartStation`, `StopStation` undefined; refill not happening.

- [ ] **Step 3: Add station methods and refill to `jukebox.go`**

In `internal/jukebox/jukebox.go`, add these methods (anywhere after `New`):

```go
// StartStation attaches a genre radio to the stream and immediately fills the
// queue. An empty queue never drains (Next is never called), so the initial
// fill is what gets playback going.
func (j *Jukebox) StartStation(streamID, genre string, threshold, batch int) error {
	if err := j.EnsureStream(streamID, "private"); err != nil {
		return err
	}
	if err := store.UpsertStation(j.db, store.Station{
		StreamID: streamID, Genre: genre, Threshold: threshold, Batch: batch,
	}); err != nil {
		return err
	}
	j.refill(streamID)
	return nil
}

// StopStation detaches the radio; queued tracks keep playing.
func (j *Jukebox) StopStation(streamID string) error {
	return store.DeleteStation(j.db, streamID)
}

// GetStation returns the stream's station, ok=false if none.
func (j *Jukebox) GetStation(streamID string) (store.Station, bool) {
	return store.GetStation(j.db, streamID)
}

// refill tops the queue up to the station's batch size when it has fallen below
// the threshold. No-op when no station is attached or the genre is exhausted.
// Reuses Request so fairness rules (dedupe + HistoryWindow) apply.
func (j *Jukebox) refill(streamID string) {
	st, ok := store.GetStation(j.db, streamID)
	if !ok {
		return
	}
	n, err := store.QueueLen(j.db, streamID)
	if err != nil || n >= st.Threshold {
		return
	}
	tracks, err := store.DiscoverTracks(j.db, store.DiscoverOpts{
		Genre:         st.Genre,
		OrderBy:       "random",
		ExcludeStream: streamID,
		Window:        j.cfg.HistoryWindow,
		Limit:         st.Batch,
	})
	if err != nil {
		return
	}
	for _, tr := range tracks {
		j.Request(streamID, tr.ID) // fairness-checked; ignore per-track result
	}
}
```

- [ ] **Step 4: Call `refill` after a successful pop in `Next`**

In `internal/jukebox/jukebox.go`, modify `Next` so it refills after popping. Replace the body of `Next` with:

```go
func (j *Jukebox) Next(streamID string) (model.Track, bool) {
	id, ok := store.PopNext(j.db, streamID)
	if !ok {
		return model.Track{}, false
	}
	tr, _, found := store.GetTrack(j.db, id)
	if !found {
		return model.Track{}, false
	}
	j.refill(streamID)
	return tr, true
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/jukebox/ -run 'Station|Refill' -v`
Expected: PASS (all three).

- [ ] **Step 6: Run the full jukebox suite**

Run: `go test ./internal/jukebox/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/jukebox/jukebox.go internal/jukebox/jukebox_test.go
git commit -m "feat: genre station control + pull-on-drain refill"
```

---

## Task 7: HTTP endpoints for discover + stations

**Files:**
- Create: `internal/api/discover.go`
- Modify: `internal/api/server.go` (register routes)
- Test: `internal/api/discover_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/api/discover_test.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

func TestDiscoverRediscoverEndpoint(t *testing.T) {
	srv := newTestServer(t)
	store.UpsertTrack(srv.db, model.Track{Path: "/m/a.mp3", Title: "A", Genre: "Rock"}, "B", "X")

	req := httptest.NewRequest(http.MethodGet, "/api/discover/rediscover?genre=Rock", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d", rec.Code)
	}
	var got []model.Track
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	if len(got) != 1 || got[0].Title != "A" {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestDiscoverGenresEndpoint(t *testing.T) {
	srv := newTestServer(t)
	store.UpsertTrack(srv.db, model.Track{Path: "/m/a.mp3", Title: "A", Genre: "Rock"}, "B", "X")

	req := httptest.NewRequest(http.MethodGet, "/api/discover/genres", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Rock") {
		t.Fatalf("unexpected genres response: %d %s", rec.Code, rec.Body.String())
	}
}

func TestStationStartGetStopEndpoints(t *testing.T) {
	srv := newTestServer(t)
	for _, p := range []string{"/m/1.mp3", "/m/2.mp3", "/m/3.mp3"} {
		store.UpsertTrack(srv.db, model.Track{Path: p, Title: p, Genre: "Rock"}, "B", "X")
	}

	// Start
	body := strings.NewReader(`{"genre":"Rock"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/streams/s/station", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("start status: %d body=%s", rec.Code, rec.Body.String())
	}

	// Get
	req2 := httptest.NewRequest(http.MethodGet, "/api/streams/s/station", nil)
	rec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK || !strings.Contains(rec2.Body.String(), "Rock") {
		t.Fatalf("get station: %d %s", rec2.Code, rec2.Body.String())
	}

	// Queue should have been filled immediately.
	n, _ := store.QueueLen(srv.db, "s")
	if n == 0 {
		t.Fatalf("expected immediate fill, queue empty")
	}

	// Stop
	req3 := httptest.NewRequest(http.MethodDelete, "/api/streams/s/station", nil)
	rec3 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("stop status: %d", rec3.Code)
	}
	if _, ok := store.GetStation(srv.db, "s"); ok {
		t.Fatalf("expected station removed after stop")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/ -run 'Discover|Station' -v`
Expected: FAIL — routes 404 / handlers undefined.

- [ ] **Step 3: Write `discover.go` handlers**

Create `internal/api/discover.go`:

```go
package api

import (
	"encoding/json"
	"net/http"

	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

func (s *Server) discover(w http.ResponseWriter, r *http.Request, orderBy string) {
	list, err := store.DiscoverTracks(s.db, store.DiscoverOpts{
		Genre:   r.URL.Query().Get("genre"),
		OrderBy: orderBy,
		Limit:   queryInt(r, "limit", 50),
		Offset:  queryInt(r, "offset", 0),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []model.Track{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) discoverRediscover(w http.ResponseWriter, r *http.Request) {
	s.discover(w, r, "rediscover")
}

func (s *Server) discoverRecent(w http.ResponseWriter, r *http.Request) {
	s.discover(w, r, "recent")
}

func (s *Server) discoverGenres(w http.ResponseWriter, r *http.Request) {
	list, err := store.GenreCounts(s.db)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []store.GenreCount{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) getStationHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if st, ok := s.jb.GetStation(id); ok {
		writeJSON(w, http.StatusOK, st)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}

func (s *Server) startStationHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Genre string `json:"genre"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Genre == "" {
		writeErr(w, http.StatusBadRequest, "missing genre")
		return
	}
	// Defaults per spec: refill when fewer than 3 remain, add 10 at a time.
	if err := s.jb.StartStation(id, body.Genre, 3, 10); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.publishQueueChanged(id)
	st, _ := s.jb.GetStation(id)
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) stopStationHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.jb.StopStation(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
```

- [ ] **Step 4: Publish `queue-changed` after `Next` so SSE clients see refills**

In `internal/api/streams.go`, the `nextTrack` handler currently publishes nothing. Station refills happen inside `Next`, so the queue can grow without notifying SSE listeners. Add a publish after a successful pop. Replace the success branch of `nextTrack`:

```go
	tr, ok := s.jb.Next(id)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false})
		return
	}
	s.publishQueueChanged(id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "track": tr})
```

- [ ] **Step 5: Register the routes**

In `internal/api/server.go`, inside `Handler()`, add these lines alongside the other `mux.HandleFunc` calls (before the `s.ui` block):

```go
	mux.HandleFunc("GET /api/discover/rediscover", s.discoverRediscover)
	mux.HandleFunc("GET /api/discover/recent", s.discoverRecent)
	mux.HandleFunc("GET /api/discover/genres", s.discoverGenres)
	mux.HandleFunc("GET /api/streams/{id}/station", s.getStationHandler)
	mux.HandleFunc("POST /api/streams/{id}/station", s.startStationHandler)
	mux.HandleFunc("DELETE /api/streams/{id}/station", s.stopStationHandler)
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/api/ -run 'Discover|Station' -v`
Expected: PASS.

- [ ] **Step 7: Run the full backend suite + vet**

Run: `go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/api/discover.go internal/api/server.go internal/api/streams.go internal/api/discover_test.go
git commit -m "feat: discover + station HTTP endpoints"
```

---

## Task 8: Frontend — Discover view

The UI is a Svelte SPA in `web/src/`, built into `internal/web/dist`. This task adds API client functions and a Discover view, then rebuilds.

**Files:**
- Modify: `web/src/lib/api.js`
- Modify: `web/src/App.svelte`

- [ ] **Step 1: Inspect the current API client and component**

Run: `sed -n '1,80p' web/src/lib/api.js && echo '---APP---' && wc -l web/src/App.svelte`
Read enough to match the existing fetch-wrapper style (base URL handling, JSON parsing, how `request`/`getStream` are written). Mirror that style in the next step rather than inventing a new one.

- [ ] **Step 2: Add discover/station client functions to `api.js`**

Append to `web/src/lib/api.js`, matching the file's existing export/fetch convention (adjust the base-path prefix to match neighbours — e.g. if existing calls use `api('/streams/...')`, use that helper instead of raw `fetch`):

```js
export async function discoverRediscover(genre = '') {
  const q = genre ? `?genre=${encodeURIComponent(genre)}` : '';
  const r = await fetch(`/api/discover/rediscover${q}`);
  return r.json();
}

export async function discoverRecent(genre = '') {
  const q = genre ? `?genre=${encodeURIComponent(genre)}` : '';
  const r = await fetch(`/api/discover/recent${q}`);
  return r.json();
}

export async function discoverGenres() {
  const r = await fetch('/api/discover/genres');
  return r.json();
}

export async function getStation(streamID) {
  const r = await fetch(`/api/streams/${streamID}/station`);
  return r.json();
}

export async function startStation(streamID, genre) {
  const r = await fetch(`/api/streams/${streamID}/station`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ genre }),
  });
  return r.json();
}

export async function stopStation(streamID) {
  const r = await fetch(`/api/streams/${streamID}/station`, { method: 'DELETE' });
  return r.json();
}
```

- [ ] **Step 3: Add the Discover view to `App.svelte`**

In `web/src/App.svelte`:

1. Import the new functions alongside the existing api imports:
   ```js
   import { discoverRediscover, discoverRecent, discoverGenres, getStation, startStation, stopStation } from './lib/api.js';
   ```
2. Add component state near the other `let` declarations:
   ```js
   let discoverGenresList = [];
   let rediscoverTracks = [];
   let recentTracks = [];
   let discoverGenreFilter = '';
   let activeStation = null;
   let stationGenre = '';

   async function loadDiscover() {
     discoverGenresList = await discoverGenres();
     rediscoverTracks = await discoverRediscover(discoverGenreFilter);
     recentTracks = await discoverRecent(discoverGenreFilter);
     activeStation = await getStation(currentStreamId); // reuse the app's existing stream id variable
   }
   async function onStartStation() {
     if (!stationGenre) return;
     activeStation = await startStation(currentStreamId, stationGenre);
     await refreshQueue();                                // reuse the app's existing queue refresh
   }
   async function onStopStation() {
     await stopStation(currentStreamId);
     activeStation = null;
   }
   ```
   Replace `currentStreamId` and `refreshQueue()` with the identifiers this component already uses for the active stream id and queue reload (discovered in Step 1).
3. Add a Discover section to the markup. Reuse the existing track-row component / request action used by the browse view so each row can be queued. Minimal structure:
   ```svelte
   <section class="discover">
     <h2>Discover</h2>

     <label>Genre filter:
       <select bind:value={discoverGenreFilter} on:change={loadDiscover}>
         <option value="">All</option>
         {#each discoverGenresList as g}
           <option value={g.genre}>{g.genre} ({g.count})</option>
         {/each}
       </select>
     </label>

     <h3>Genre station</h3>
     {#if activeStation && activeStation.genre}
       <p>Playing <strong>{activeStation.genre}</strong> radio</p>
       <button on:click={onStopStation}>Stop station</button>
     {:else}
       <select bind:value={stationGenre}>
         <option value="">Pick a genre…</option>
         {#each discoverGenresList as g}
           <option value={g.genre}>{g.genre}</option>
         {/each}
       </select>
       <button on:click={onStartStation} disabled={!stationGenre}>Start station</button>
     {/if}

     <h3>Rediscover</h3>
     {#each rediscoverTracks as t}
       <!-- reuse existing track-row + request control here -->
       <div>{t.title}</div>
     {/each}

     <h3>Recently added</h3>
     {#each recentTracks as t}
       <div>{t.title}</div>
     {/each}
   </section>
   ```
4. Call `loadDiscover()` from the component's existing `onMount` (or wherever browse data is first loaded), so the view populates on open.

- [ ] **Step 4: Build the frontend**

Run: `cd web && mise exec -- npm install && mise exec -- npm run build`
Expected: build succeeds, emitting updated assets into `internal/web/dist`.

(If the project uses a different build invocation, check `web/package.json` scripts and the v3 plan's build step; match whatever those use.)

- [ ] **Step 5: Manual smoke check**

Run: `go run . ` (or the project's normal start command), open the UI, and confirm: the Discover section lists genres, Rediscover/Recently-added populate, starting a genre station fills the queue and begins playback, and Stop halts further refills. Note: the SPA build is required for the embedded UI to reflect changes (`internal/web/dist` is embedded at compile time).

- [ ] **Step 6: Commit**

```bash
git add web/src/ internal/web/dist
git commit -m "feat: Discover view (rediscover, recently-added, genre stations)"
```

---

## Task 9: Final verification

- [ ] **Step 1: Full build, vet, and test**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all PASS.

- [ ] **Step 2: Confirm no stray discovery TODOs**

Run: `git grep -n "TODO\|FIXME" internal/store/discover.go internal/store/station.go internal/api/discover.go internal/jukebox/jukebox.go`
Expected: no discovery-related TODOs.

- [ ] **Step 3: Push and open PR**

```bash
git push -u origin plan-issue-19-discovery
gh pr create --title "Music discovery (#19)" --body "Implements issue #19: local Rediscover + Recently-added browse lists and continuous genre stations. Spec: docs/superpowers/specs/2026-06-11-music-discovery-design.md. Closes #19."
```
