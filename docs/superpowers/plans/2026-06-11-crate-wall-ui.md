# Crate-wall Jukebox UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the placeholder Svelte UI with the pixel-exact neon-noir "Crate wall" jukebox interface from the Claude Design bundle, wired to the Go backend, with small backend additions (requester attribution, real listener count, shuffle).

**Architecture:** Svelte 5 SPA in `web/`, built by Vite into the embedded `internal/web/dist`. A small `store.js` holds app state; thin prop-driven components mirror the bundle's component split. The Go backend gains three small deltas threaded through `store → jukebox → api`. The visual source of truth is vendored at `docs/design-bundle/` — port styles from it verbatim.

**Tech Stack:** Go 1.x (net/http, database/sql, SQLite), Svelte 5 (runes), Vite. No new runtime deps (fonts self-hosted as `.woff2`).

**Reference material (durable, in-repo):**
- `docs/design-bundle/project/Exit 66 Jukebox.dc.html` — main app layout + state logic.
- `docs/design-bundle/project/JukeAlbumCard.dc.html`, `JukeLineup.dc.html` — child components.
- `docs/design-bundle/project/_ds/.../_ds_bundle.js` — DS component sources (React; port to Svelte). Line ranges cited per task.
- `docs/design-bundle/project/_ds/.../tokens/*.css` — design tokens.

**Conventions for every component port:**
- Reproduce **every** style value (px, color token, gap, radius) exactly from the cited source.
- The bundle implements hover/focus with React `useState`; in Svelte use CSS `:hover`/`:focus-within` in a `<style>` block instead (same visual result, less code). Dynamic values (widths, percent, conditional colors) stay as inline `style={...}`.
- All colors/sizes come from CSS custom properties (e.g. `var(--neon-magenta)`), never hard-coded hex except where the bundle itself hard-codes (e.g. art-tone gradients).
- Time formatting uses `fmt(sec)` from `lib/format.js` (Task 9).

---

## File Structure

**Backend (Go):**
- `internal/store/queue.go` — add `QueueWithRequester` (modify); `PopNext` gains shuffle (modify).
- `internal/store/queue_test.go` — new tests (create).
- `internal/jukebox/jukebox.go` — `QueuedTrack` type, thread `requestedBy`, shuffle flag (modify).
- `internal/jukebox/jukebox_test.go` — new tests (modify).
- `internal/broadcast/hub.go` — `ListenerCount()` (modify).
- `internal/broadcast/hub_test.go` — new test (create).
- `internal/api/streams.go` — `by` field, `requested_by` + `listeners` in response, shuffle endpoint (modify).
- `internal/api/server.go` — register shuffle route; hold hub refs for listener counts (modify).

**Frontend (`web/`):**
- `web/vite.config.js` — proxy `/stream` (modify).
- `web/scripts/fetch-fonts.mjs` — one-time font downloader (create).
- `web/public/fonts/*.woff2` — self-hosted fonts (create, via script).
- `web/src/app.css` — global resets, scrollbar, keyframes, token import (modify).
- `web/src/lib/tokens.css` — ported tokens + `@font-face` (create).
- `web/src/lib/format.js` — `fmt`, `slotCodes`, `gradientFor` helpers (create).
- `web/src/lib/api.js` — extended client (modify).
- `web/src/lib/store.js` — app state (create).
- `web/src/lib/components/*.svelte` — Toast, Switch, Avatar, SearchInput, TrackRow, QueueItem, NowPlayingBar, AlbumCard, AlbumGrid, ArtistList, TrackList, Lineup, Tabs, TopBar, AlbumDialog, MobilePlayer (create).
- `web/src/App.svelte` — shell assembly + wiring (rewrite).

---

# PHASE 1 — Backend deltas (Go, TDD)

## Task 1: Store — queue read with requester

**Files:**
- Modify: `internal/store/queue.go`
- Test: `internal/store/queue_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `internal/store/queue_test.go`:

```go
package store

import (
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/model"
)

func TestQueueWithRequester(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	id, _ := UpsertTrack(db, model.Track{Path: "/m/a.mp3", Title: "A"}, "Band", "LP")
	if err := EnsureStream(db, "s", "", "private"); err != nil {
		t.Fatal(err)
	}
	if err := Enqueue(db, "s", id, "Mira"); err != nil {
		t.Fatal(err)
	}

	rows, err := QueueWithRequester(db, "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if rows[0].TrackID != id || rows[0].RequestedBy != "Mira" {
		t.Fatalf("got %+v, want trackID=%d requestedBy=Mira", rows[0], id)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestQueueWithRequester -v`
Expected: FAIL — `undefined: QueueWithRequester`.

- [ ] **Step 3: Add the function**

Append to `internal/store/queue.go`:

```go
// QueuedRow is a queued track id paired with who requested it, in play order.
type QueuedRow struct {
	TrackID     int64
	RequestedBy string
}

// QueueWithRequester returns the queued rows (track id + requester) in play order.
func QueueWithRequester(db *sql.DB, streamID string) ([]QueuedRow, error) {
	rows, err := db.Query(
		`SELECT track_id, added_by FROM queue_item WHERE stream_id=? ORDER BY play_order`, streamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []QueuedRow
	for rows.Next() {
		var r QueuedRow
		if err := rows.Scan(&r.TrackID, &r.RequestedBy); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestQueueWithRequester -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/queue.go internal/store/queue_test.go
git commit -m "feat(store): read queue rows with requester"
```

---

## Task 2: Store — shuffle pop

**Files:**
- Modify: `internal/store/queue.go`
- Test: `internal/store/queue_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/store/queue_test.go`:

```go
func TestPopNextShuffleEmptiesQueue(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	EnsureStream(db, "s", "", "private")
	var ids []int64
	for _, title := range []string{"A", "B", "C"} {
		id, _ := UpsertTrack(db, model.Track{Path: "/m/" + title + ".mp3", Title: title}, "Band", "LP")
		Enqueue(db, "s", id, "")
		ids = append(ids, id)
	}
	seen := map[int64]bool{}
	for range ids {
		tid, ok := PopNextShuffle(db, "s")
		if !ok {
			t.Fatal("expected ok=true while queue non-empty")
		}
		if seen[tid] {
			t.Fatalf("popped duplicate %d", tid)
		}
		seen[tid] = true
	}
	if _, ok := PopNextShuffle(db, "s"); ok {
		t.Fatal("expected ok=false on empty queue")
	}
	if len(seen) != 3 {
		t.Fatalf("want 3 distinct pops, got %d", len(seen))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestPopNextShuffle -v`
Expected: FAIL — `undefined: PopNextShuffle`.

- [ ] **Step 3: Refactor PopNext to share logic and add shuffle variant**

In `internal/store/queue.go`, replace the existing `PopNext` function with the two below (the shared `popOrder` keeps the atomic delete + history + play-count semantics; only the row-selection SQL differs):

```go
// PopNext removes and returns the next track id in play order (FIFO).
func PopNext(db *sql.DB, streamID string) (trackID int64, ok bool) {
	return popWith(db, streamID,
		`SELECT track_id FROM queue_item WHERE stream_id=? ORDER BY play_order LIMIT 1`)
}

// PopNextShuffle removes and returns a random queued track id.
func PopNextShuffle(db *sql.DB, streamID string) (trackID int64, ok bool) {
	return popWith(db, streamID,
		`SELECT track_id FROM queue_item WHERE stream_id=? ORDER BY RANDOM() LIMIT 1`)
}

// popWith pops the row chosen by selectSQL, recording history and bumping the
// play count atomically. selectSQL must take a single stream_id parameter and
// return one track_id.
func popWith(db *sql.DB, streamID, selectSQL string) (trackID int64, ok bool) {
	tx, err := db.Begin()
	if err != nil {
		return 0, false
	}
	defer tx.Rollback()

	if err := tx.QueryRow(selectSQL, streamID).Scan(&trackID); err != nil {
		return 0, false
	}
	if _, err := tx.Exec(`DELETE FROM queue_item WHERE stream_id=? AND track_id=?`,
		streamID, trackID); err != nil {
		return 0, false
	}
	if _, err := tx.Exec(
		`INSERT INTO history(stream_id, track_id, played_at) VALUES(?,?,strftime('%s','now'))`,
		streamID, trackID); err != nil {
		return 0, false
	}
	if _, err := tx.Exec(`UPDATE track SET play_count = play_count + 1 WHERE id=?`,
		trackID); err != nil {
		return 0, false
	}
	if err := tx.Commit(); err != nil {
		return 0, false
	}
	return trackID, true
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/store/ -v`
Expected: PASS (new shuffle test + existing store tests).

- [ ] **Step 5: Commit**

```bash
git add internal/store/queue.go internal/store/queue_test.go
git commit -m "feat(store): add PopNextShuffle, share pop logic"
```

---

## Task 3: Jukebox — QueuedTrack, requester threading, shuffle flag

**Files:**
- Modify: `internal/jukebox/jukebox.go`
- Test: `internal/jukebox/jukebox_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/jukebox/jukebox_test.go`:

```go
func TestQueueReturnsRequester(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	id, _ := store.UpsertTrack(db, model.Track{Path: "/m/a.mp3", Title: "A"}, "Band", "LP")
	jb := New(db, Config{HistoryWindow: 0})
	jb.EnsureStream("s", "private")
	if _, err := jb.Request("s", id, "Mira"); err != nil {
		t.Fatal(err)
	}
	q, err := jb.Queue("s")
	if err != nil {
		t.Fatal(err)
	}
	if len(q) != 1 || q[0].RequestedBy != "Mira" || q[0].Track.ID != id {
		t.Fatalf("got %+v", q)
	}
}

func TestShuffleFlagDrivesNext(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	jb := New(db, Config{HistoryWindow: 0})
	jb.EnsureStream("s", "private")
	for _, ti := range []string{"A", "B", "C"} {
		id, _ := store.UpsertTrack(db, model.Track{Path: "/m/" + ti + ".mp3", Title: ti}, "Band", "LP")
		jb.Request("s", id, "")
	}
	jb.SetShuffle("s", true)
	got := 0
	for {
		if _, ok := jb.Next("s"); !ok {
			break
		}
		got++
	}
	if got != 3 {
		t.Fatalf("want 3 pops, got %d", got)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/jukebox/ -run 'TestQueueReturnsRequester|TestShuffleFlagDrivesNext' -v`
Expected: FAIL — `Request` takes 2 args / `RequestedBy` undefined / `SetShuffle` undefined.

- [ ] **Step 3: Update jukebox.go**

In `internal/jukebox/jukebox.go`:

(a) Add to imports `"sync"`. Add the `QueuedTrack` type after the `Config` type:

```go
// QueuedTrack is a queued track plus who requested it.
type QueuedTrack struct {
	Track       model.Track `json:"track"`
	RequestedBy string      `json:"requested_by"`
}
```

(b) Add a shuffle map to the struct and init it in `New`:

```go
type Jukebox struct {
	db      *sql.DB
	cfg     Config
	mu      sync.Mutex
	shuffle map[string]bool
}

func New(db *sql.DB, cfg Config) *Jukebox {
	if cfg.HistoryWindow < 0 {
		cfg.HistoryWindow = 0
	}
	return &Jukebox{db: db, cfg: cfg, shuffle: make(map[string]bool)}
}
```

(c) Add shuffle accessors:

```go
// SetShuffle sets the per-stream shuffle flag (affects what Next pops).
func (j *Jukebox) SetShuffle(streamID string, on bool) {
	j.mu.Lock()
	j.shuffle[streamID] = on
	j.mu.Unlock()
}

// Shuffle reports the per-stream shuffle flag.
func (j *Jukebox) Shuffle(streamID string) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.shuffle[streamID]
}
```

(d) Change `Request` to accept a requester and pass it to `Enqueue`:

```go
func (j *Jukebox) Request(streamID string, trackID int64, requestedBy string) (Result, error) {
	dup, err := store.InQueue(j.db, streamID, trackID)
	if err != nil {
		return Requested, err
	}
	if dup {
		return AlreadyQueued, nil
	}
	if j.cfg.HistoryWindow > 0 {
		recent, err := store.RecentlyPlayed(j.db, streamID, trackID, j.cfg.HistoryWindow)
		if err != nil {
			return Requested, err
		}
		if recent {
			return RecentlyPlayed, nil
		}
	}
	if err := store.Enqueue(j.db, streamID, trackID, requestedBy); err != nil {
		return Requested, err
	}
	return Requested, nil
}
```

(e) Change `Next` to honor the shuffle flag:

```go
func (j *Jukebox) Next(streamID string) (model.Track, bool) {
	var id int64
	var ok bool
	if j.Shuffle(streamID) {
		id, ok = store.PopNextShuffle(j.db, streamID)
	} else {
		id, ok = store.PopNext(j.db, streamID)
	}
	if !ok {
		return model.Track{}, false
	}
	tr, _, found := store.GetTrack(j.db, id)
	if !found {
		return model.Track{}, false
	}
	return tr, true
}
```

(f) Change `Queue` to return `[]QueuedTrack`:

```go
func (j *Jukebox) Queue(streamID string) ([]QueuedTrack, error) {
	rows, err := store.QueueWithRequester(j.db, streamID)
	if err != nil {
		return nil, err
	}
	out := make([]QueuedTrack, 0, len(rows))
	for _, r := range rows {
		if tr, _, ok := store.GetTrack(j.db, r.TrackID); ok {
			out = append(out, QueuedTrack{Track: tr, RequestedBy: r.RequestedBy})
		}
	}
	return out, nil
}
```

(g) Thread requester through bulk requests:

```go
func (j *Jukebox) RequestAlbum(streamID string, albumID int64, requestedBy string) int {
	ids, _ := store.TrackIDsByAlbum(j.db, albumID)
	return j.requestMany(streamID, ids, requestedBy)
}

func (j *Jukebox) RequestArtist(streamID string, artistID int64, requestedBy string) int {
	ids, _ := store.TrackIDsByArtist(j.db, artistID)
	return j.requestMany(streamID, ids, requestedBy)
}

func (j *Jukebox) requestMany(streamID string, ids []int64, requestedBy string) int {
	queued := 0
	for _, id := range ids {
		if res, err := j.Request(streamID, id, requestedBy); err == nil && res == Requested {
			queued++
		}
	}
	return queued
}
```

- [ ] **Step 4: Fix the existing jukebox test calls**

In `internal/jukebox/jukebox_test.go`, the existing tests call `jb.Request("sess1", id)` and `jb.RequestAlbum("s", albumID)`. Update each existing `Request` call to pass a requester arg (e.g. `jb.Request("sess1", id, "")`) and each `RequestAlbum`/`RequestArtist` call to add `""`. The existing `Queue` assertions that read `q[0].ID` must become `q[0].Track.ID`, and `len(q)` is unchanged.

- [ ] **Step 5: Run package tests**

Run: `go test ./internal/jukebox/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/jukebox/jukebox.go internal/jukebox/jukebox_test.go
git commit -m "feat(jukebox): requester threading, shuffle flag, QueuedTrack"
```

---

## Task 4: Broadcast — listener count

**Files:**
- Modify: `internal/broadcast/hub.go`
- Test: `internal/broadcast/hub_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `internal/broadcast/hub_test.go`:

```go
package broadcast

import "testing"

func TestListenerCount(t *testing.T) {
	h := NewHub(nil, func() (string, bool) { return "", false }, nil)
	if h.ListenerCount() != 0 {
		t.Fatalf("want 0, got %d", h.ListenerCount())
	}
	_, c1 := h.Listen()
	_, c2 := h.Listen()
	if h.ListenerCount() != 2 {
		t.Fatalf("want 2, got %d", h.ListenerCount())
	}
	c1()
	if h.ListenerCount() != 1 {
		t.Fatalf("want 1, got %d", h.ListenerCount())
	}
	c2()
	if h.ListenerCount() != 0 {
		t.Fatalf("want 0, got %d", h.ListenerCount())
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/broadcast/ -run TestListenerCount -v`
Expected: FAIL — `undefined: (*Hub).ListenerCount`.

- [ ] **Step 3: Add the method**

Append to `internal/broadcast/hub.go`:

```go
// ListenerCount returns the number of connected listeners.
func (h *Hub) ListenerCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.listeners)
}
```

- [ ] **Step 4: Run test**

Run: `go test ./internal/broadcast/ -run TestListenerCount -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/broadcast/hub.go internal/broadcast/hub_test.go
git commit -m "feat(broadcast): expose listener count"
```

---

## Task 5: API — requester field, listeners + requested_by in responses, shuffle route

**Files:**
- Modify: `internal/api/streams.go`, `internal/api/server.go`
- Test: `internal/api/server_test.go` (extend existing)

- [ ] **Step 1: Write the failing test**

In `internal/api/server_test.go`, add (the file already has a `newTestServer(t) *Server` helper and imports `net/url`, `strconv`, `store`, `model`):

```go
func TestRequestRecordsRequesterAndStreamReturnsIt(t *testing.T) {
	srv := newTestServer(t)
	id, _ := store.UpsertTrack(srv.db, model.Track{Path: "/m/a.mp3", Title: "Hello"}, "Band", "Album")

	form := url.Values{"kind": {"track"}, "id": {strconv.FormatInt(id, 10)}, "by": {"Mira"}}
	req := httptest.NewRequest(http.MethodPost, "/api/streams/sess/requests",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("request status %d", rec.Code)
	}

	rec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/streams/sess", nil))
	if !strings.Contains(rec2.Body.String(), `"requested_by":"Mira"`) {
		t.Fatalf("stream body missing requester: %s", rec2.Body.String())
	}
	if !strings.Contains(rec2.Body.String(), `"listeners":`) {
		t.Fatalf("stream body missing listeners: %s", rec2.Body.String())
	}
}
```

The contract: `POST .../requests` accepts `by=`, and `GET .../streams/{id}` includes `"requested_by"` and `"listeners"`.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/api/ -run TestRequestRecordsRequester -v`
Expected: FAIL — response lacks `requested_by` (queue currently serialized as bare tracks).

- [ ] **Step 3: Update `getStream` and `request` in `internal/api/streams.go`**

(a) `getStream` already calls `s.jb.Queue(id)` which now returns `[]jukebox.QueuedTrack` — the `map[string]any` response serializes it directly. Add the listener count. Replace the response block:

```go
func (s *Server) getStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.jb.EnsureStream(id, "private"); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	q, err := s.jb.Queue(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if q == nil {
		q = []jukebox.QueuedTrack{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":        id,
		"queue":     q,
		"listeners": s.listenerCount(id),
	})
}
```

(b) In `request`, read the `by` form value and pass it through:

```go
	by := r.FormValue("by")
	...
	case "album":
		n := s.jb.RequestAlbum(id, targetID, by)
	...
	case "artist":
		n := s.jb.RequestArtist(id, targetID, by)
	...
	case "track":
		res, err := s.jb.Request(id, targetID, by)
```

(c) Add a shuffle handler at the end of the file:

```go
func (s *Server) setShuffle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	r.ParseForm()
	on := r.FormValue("value") == "true" || r.FormValue("value") == "1"
	s.jb.SetShuffle(id, on)
	writeJSON(w, http.StatusOK, map[string]any{"shuffle": on})
}
```

- [ ] **Step 4: Add `listenerCount` + route + hub map in `internal/api/server.go`**

(a) The `Server` already has `hubs map[string]*broadcast.Hub`. Add a helper:

```go
// listenerCount returns connected listeners for a registered shared stream, or
// 0 for private streams with no hub.
func (s *Server) listenerCount(streamID string) int {
	if hub, ok := s.hubs[streamID]; ok {
		return hub.ListenerCount()
	}
	return 0
}
```

(b) Register the shuffle route in `Handler()` next to the other stream routes:

```go
	mux.HandleFunc("POST /api/streams/{id}/shuffle", s.setShuffle)
```

- [ ] **Step 5: Run API + full backend tests**

Run: `go test ./...`
Expected: PASS (all packages). Fix any remaining call sites the compiler flags (e.g. other callers of `jb.Request`).

- [ ] **Step 6: Commit**

```bash
git add internal/api/streams.go internal/api/server.go internal/api/server_test.go
git commit -m "feat(api): requester field, listeners + requested_by, shuffle route"
```

---

## Task 6: Wire shuffle-capable house stream (verify main.go compiles)

**Files:**
- Modify: `main.go` (only if compiler requires; `jb.Next(houseID)` signature is unchanged)

- [ ] **Step 1: Build**

Run: `go build ./...`
Expected: PASS. `main.go` needs no change — `Next` keeps its signature and reads the shuffle flag internally. If the build flags an unused import or changed signature, fix minimally.

- [ ] **Step 2: Commit (if changed)**

```bash
git add main.go
git commit -m "chore: build fixups for backend deltas"
```

---

# PHASE 2 — Frontend foundation

## Task 7: Self-host fonts

**Files:**
- Create: `web/scripts/fetch-fonts.mjs`
- Create (via script): `web/public/fonts/*.woff2`

- [ ] **Step 1: Write the downloader**

Create `web/scripts/fetch-fonts.mjs`. It resolves the Google Fonts CSS2 endpoint (which returns `@font-face` blocks with `.woff2` URLs for the latin subset) and saves each weight locally with a stable name:

```js
// Run once: node scripts/fetch-fonts.mjs
// Downloads latin .woff2 for the three brand families into public/fonts/.
import { mkdir, writeFile } from 'node:fs/promises';

const FAMILIES = [
  { css: 'Chakra+Petch:wght@400;500;600;700', slug: 'chakra-petch' },
  { css: 'Space+Grotesk:wght@400;500;600;700', slug: 'space-grotesk' },
  { css: 'Space+Mono:wght@400;700', slug: 'space-mono' },
];
const UA =
  'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36';

await mkdir('public/fonts', { recursive: true });
const faces = [];
for (const f of FAMILIES) {
  const css = await (
    await fetch(`https://fonts.googleapis.com/css2?family=${f.css}&display=swap`, {
      headers: { 'User-Agent': UA },
    })
  ).text();
  // Each @font-face block: capture weight + the latin woff2 url.
  const blocks = css.split('@font-face').slice(1);
  for (const b of blocks) {
    // keep only latin (the block right after a "/* latin */" comment)
    const url = b.match(/url\((https:[^)]+\.woff2)\)/)?.[1];
    const weight = b.match(/font-weight:\s*(\d+)/)?.[1] ?? '400';
    if (!url) continue;
    // One file per (family,weight); later unicode-range blocks overwrite with
    // the same name, leaving a working latin glyph set.
    const name = `${f.slug}-${weight}.woff2`;
    const buf = Buffer.from(await (await fetch(url)).arrayBuffer());
    await writeFile(`public/fonts/${name}`, buf);
    faces.push(name);
  }
}
console.log('saved', [...new Set(faces)].sort().join(', '));
```

> The CSS2 endpoint returns several unicode-range blocks per weight (latin, latin-ext, …). Saving the matched `.woff2` for each weight yields working latin glyphs; the exact subset name doesn't matter for a home app. If a weight produces multiple files, the latin block is sufficient — dedupe by `${slug}-${weight}.woff2`.

- [ ] **Step 2: Run the downloader**

Run: `cd web && node scripts/fetch-fonts.mjs`
Expected: prints saved filenames; `web/public/fonts/` contains `chakra-petch-{400,500,600,700}.woff2`, `space-grotesk-{400,500,600,700}.woff2`, `space-mono-{400,700}.woff2`.

- [ ] **Step 3: Verify files exist and are non-empty**

Run: `ls -l web/public/fonts/`
Expected: 10 `.woff2` files, each > 5 KB.

- [ ] **Step 4: Commit**

```bash
git add web/scripts/fetch-fonts.mjs web/public/fonts/
git commit -m "feat(web): self-host brand fonts"
```

---

## Task 8: Design tokens + global CSS

**Files:**
- Create: `web/src/lib/tokens.css`
- Modify: `web/src/app.css`

- [ ] **Step 1: Create `web/src/lib/tokens.css`**

Concatenate the contents of these four token files **verbatim** (copy their `:root { … }` bodies):
- `docs/design-bundle/project/_ds/.../tokens/colors.css`
- `docs/design-bundle/project/_ds/.../tokens/typography.css`
- `docs/design-bundle/project/_ds/.../tokens/spacing.css`
- `docs/design-bundle/project/_ds/.../tokens/effects.css`

Then **replace** the CDN `@import` (from `tokens/fonts.css`) with self-hosted `@font-face` rules at the top of `tokens.css`:

```css
/* Self-hosted brand fonts (see scripts/fetch-fonts.mjs) */
@font-face { font-family: 'Chakra Petch'; font-weight: 400; font-display: swap; src: url('/fonts/chakra-petch-400.woff2') format('woff2'); }
@font-face { font-family: 'Chakra Petch'; font-weight: 500; font-display: swap; src: url('/fonts/chakra-petch-500.woff2') format('woff2'); }
@font-face { font-family: 'Chakra Petch'; font-weight: 600; font-display: swap; src: url('/fonts/chakra-petch-600.woff2') format('woff2'); }
@font-face { font-family: 'Chakra Petch'; font-weight: 700; font-display: swap; src: url('/fonts/chakra-petch-700.woff2') format('woff2'); }
@font-face { font-family: 'Space Grotesk'; font-weight: 400; font-display: swap; src: url('/fonts/space-grotesk-400.woff2') format('woff2'); }
@font-face { font-family: 'Space Grotesk'; font-weight: 500; font-display: swap; src: url('/fonts/space-grotesk-500.woff2') format('woff2'); }
@font-face { font-family: 'Space Grotesk'; font-weight: 600; font-display: swap; src: url('/fonts/space-grotesk-600.woff2') format('woff2'); }
@font-face { font-family: 'Space Grotesk'; font-weight: 700; font-display: swap; src: url('/fonts/space-grotesk-700.woff2') format('woff2'); }
@font-face { font-family: 'Space Mono'; font-weight: 400; font-display: swap; src: url('/fonts/space-mono-400.woff2') format('woff2'); }
@font-face { font-family: 'Space Mono'; font-weight: 700; font-display: swap; src: url('/fonts/space-mono-700.woff2') format('woff2'); }
```

(Keep all the `:root` token bodies after these `@font-face` rules.)

- [ ] **Step 2: Replace `web/src/app.css`**

```css
@import './lib/tokens.css';

html, body { margin: 0; height: 100%; }
body { background: var(--bg-base); color: var(--text-body); font-family: var(--font-sans); }
#app { height: 100%; }
*, *::before, *::after { box-sizing: border-box; }

::-webkit-scrollbar { width: 8px; height: 8px; }
::-webkit-scrollbar-thumb { background: var(--ink-650); border-radius: 99px; }
::-webkit-scrollbar-track { background: transparent; }

@keyframes e66-eq { from { transform: scaleY(0.22); } to { transform: scaleY(1); } }
@keyframes e66-toast-in { from { opacity: 0; transform: translateX(18px); } to { opacity: 1; transform: translateX(0); } }
```

- [ ] **Step 3: Verify build picks up CSS**

Run: `cd web && npm install && npm run build`
Expected: build succeeds; no missing-import errors. (Visual check happens in Task 22.)

- [ ] **Step 4: Commit**

```bash
git add web/src/lib/tokens.css web/src/app.css
git commit -m "feat(web): port design tokens + self-hosted font-face"
```

---

## Task 9: Format + derivation helpers

**Files:**
- Create: `web/src/lib/format.js`

- [ ] **Step 1: Write the helpers**

Create `web/src/lib/format.js`:

```js
// fmt formats seconds as m:ss (matches the design's NowPlayingBar).
export function fmt(sec) {
  if (sec == null || isNaN(sec)) return '0:00';
  const m = Math.floor(sec / 60);
  const s = Math.floor(sec % 60);
  return `${m}:${s < 10 ? '0' : ''}${s}`;
}

// albumLetter maps a stable album index to A, B, … Z, AA, AB … (base-26).
export function albumLetter(index) {
  let n = index, out = '';
  do { out = String.fromCharCode(65 + (n % 26)) + out; n = Math.floor(n / 26) - 1; } while (n >= 0);
  return out;
}

// artTones cycles the four neon gradients used for slot-code tiles / fallbacks.
const TONES = ['cyan', 'magenta', 'amber', 'violet'];
export function toneFor(index) { return TONES[index % TONES.length]; }

// gradientFor returns a deterministic cover gradient for an id (fallback art).
export function gradientFor(id) {
  const pairs = [
    ['#0b2e3a', '#1aa6c2'], ['#2a0f1f', '#c41e6b'], ['#2a1d08', '#c98a1e'],
    ['#17112e', '#5e3fd6'], ['#2a1208', '#b8431e'], ['#141033', '#3b5bd6'],
    ['#0a2230', '#3a6fb0'], ['#241024', '#8a1f4a'],
  ];
  const [a, b] = pairs[Math.abs(Number(id)) % pairs.length];
  return `radial-gradient(circle at 28% 22%, rgba(255,255,255,0.16), transparent 46%), linear-gradient(150deg, ${a} 0%, ${b} 100%)`;
}

// toneGradient maps an art-tone name to the TrackRow/QueueItem tile gradient.
export function toneGradient(tone) {
  return {
    magenta: 'linear-gradient(135deg, #2a0f1f, #ff2e88 280%)',
    cyan: 'linear-gradient(135deg, #08252b, #1fe0ff 280%)',
    amber: 'linear-gradient(135deg, #2a1d08, #ffb02e 280%)',
    violet: 'linear-gradient(135deg, #17112e, #8a6cff 280%)',
  }[tone] || 'linear-gradient(135deg, #2a0f1f, #ff2e88 280%)';
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/lib/format.js
git commit -m "feat(web): format + slot-code/gradient helpers"
```

---

## Task 10: API client extensions

**Files:**
- Modify: `web/src/lib/api.js`

- [ ] **Step 1: Extend the client**

Add/modify in `web/src/lib/api.js`:

```js
export async function listArtists(search = '') {
  const r = await fetch(`/api/artists?search=${encodeURIComponent(search)}&limit=500`);
  return r.json();
}
export async function listAlbums(search = '') {
  const r = await fetch(`/api/albums?search=${encodeURIComponent(search)}&limit=500`);
  return r.json();
}

// requestTo now sends the requester name and a kind (track|album|artist).
export async function requestTo(streamId, id, { kind = 'track', by = 'You' } = {}) {
  const body = new URLSearchParams({ kind, id: String(id), by });
  const r = await fetch(`/api/streams/${streamId}/requests`, { method: 'POST', body });
  return r.json();
}

export async function removeRequest(streamId, trackId) {
  const r = await fetch(`/api/streams/${streamId}/requests/${trackId}`, { method: 'DELETE' });
  return r.json();
}

export async function setShuffle(streamId, on) {
  const body = new URLSearchParams({ value: on ? 'true' : 'false' });
  const r = await fetch(`/api/streams/${streamId}/shuffle`, { method: 'POST', body });
  return r.json();
}

export function albumCoverURL(albumId) { return `/api/albums/${albumId}/cover`; }
```

Keep `listTracks`, `getQueue`, `houseStreamURL`, `coverURL`, `nextTrack`, `subscribeEvents`, `audioURL`, Sonos calls, and the `HOUSE` constant. Change the private session id constant from `'me'` if desired — keep `'me'` (matches the design's "Personal" mapping).

- [ ] **Step 2: Commit**

```bash
git add web/src/lib/api.js
git commit -m "feat(web): api client for artists/albums/requester/shuffle"
```

---

## Task 11: App store

**Files:**
- Create: `web/src/lib/store.js`

- [ ] **Step 1: Write the store**

Create `web/src/lib/store.js`. It loads the library once, derives slot codes + tones per album, builds artist groupings, and exposes everything the UI binds to. Uses Svelte 5 runes via a class with `$state` (works in `.svelte.js` — **name this file `store.svelte.js`** so runes compile).

> Rename: create `web/src/lib/store.svelte.js` (runes require the `.svelte.js` extension).

```js
import {
  listTracks, listAlbums, listArtists, getQueue, requestTo, removeRequest,
  setShuffle, subscribeEvents, coverURL, albumCoverURL, HOUSE,
} from './api.js';
import { albumLetter, toneFor, gradientFor } from './format.js';

const ME = 'me';

export function createStore() {
  let tab = $state('albums');
  let query = $state('');
  let stream = $state('house');           // 'house' (shared) | 'me' (personal)
  let isPhone = $state(false);
  let lineupOpen = $state(false);
  let detailAlbumId = $state(null);
  let shuffle = $state(false);
  let displayName = $state(localStorage.getItem('e66.name') || 'You');
  let toasts = $state([]);

  // library
  let albums = $state([]);      // {id,name,artistId,artistName,letter,tone,tracks:[{...,code}]}
  let artists = $state([]);     // {id,name,albumCount,trackCount,tone,tracks:[...]}
  let tracksByCode = $state({});

  // per-stream live state
  let nowPlaying = $state({ house: null, me: null });
  let progress = $state({ house: 0, me: 0 });   // seconds
  let queues = $state({ house: [], me: [] });
  let listeners = $state({ house: 0, me: 1 });

  let _uid = 0;
  let _esHouse = null;

  async function loadLibrary() {
    const [rawTracks, rawAlbums, rawArtists] = await Promise.all([
      listTracks(''), listAlbums(''), listArtists(''),
    ]);
    // group tracks by album, assign stable letters by album order
    const albumById = new Map();
    rawAlbums.forEach((al, i) => albumById.set(al.id, {
      id: al.id, name: al.name, artistId: al.artist_id,
      letter: albumLetter(i), tone: toneFor(i), tracks: [],
    }));
    const artistName = new Map(rawArtists.map((a) => [a.id, a.name]));
    const codeMap = {};
    for (const t of rawTracks) {
      const al = albumById.get(t.album_id);
      if (!al) continue;
      const code = al.letter + (t.track_no || al.tracks.length + 1);
      const enriched = {
        ...t, code, tone: al.tone, albumName: al.name,
        artistName: artistName.get(t.artist_id) || 'Unknown',
        cover: coverURL(t.id), gradient: gradientFor(t.id),
      };
      al.tracks.push(enriched);
      codeMap[code] = enriched;
    }
    const albumList = [...albumById.values()].map((al) => ({
      ...al, artistName: artistName.get(al.artistId) || 'Unknown',
      cover: albumCoverURL(al.id), gradient: gradientFor(al.id),
      initial: (al.name[0] || '?').toUpperCase(),
    }));
    // artist groupings
    const byArtist = new Map();
    for (const al of albumList) {
      if (!byArtist.has(al.artistId)) byArtist.set(al.artistId, {
        id: al.artistId, name: al.artistName, tone: al.tone,
        gradient: gradientFor(1000 + al.artistId), albums: [], tracks: [],
      });
      const g = byArtist.get(al.artistId);
      g.albums.push(al); g.tracks.push(...al.tracks);
    }
    albums = albumList;
    artists = [...byArtist.values()].map((a) => ({
      ...a, albumCount: a.albums.length, trackCount: a.tracks.length,
      initial: (a.name[0] || '?').toUpperCase(),
    }));
    tracksByCode = codeMap;
  }

  async function refreshQueue(s) {
    const r = await getQueue(s);
    queues[s] = (r.queue || []).map(normalizeQueued);
    if (typeof r.listeners === 'number') listeners[s] = r.listeners;
  }

  // backend queue items are {track:{...}, requested_by} for /me, but the house
  // SSE/queue may send bare tracks; normalize both.
  function normalizeQueued(item) {
    const t = item.track || item;
    const code = codeForTrack(t);
    return {
      uid: ++_uid, id: t.id, title: t.title,
      artistName: nameForArtist(t.artist_id), albumName: albumNameFor(t.album_id),
      code, tone: toneForTrack(t), requester: item.requested_by || '',
      cover: coverURL(t.id), gradient: gradientFor(t.id),
    };
  }
  function codeForTrack(t) {
    const al = albums.find((a) => a.id === t.album_id);
    return al ? al.letter + (t.track_no || 1) : '··';
  }
  function toneForTrack(t) {
    const al = albums.find((a) => a.id === t.album_id);
    return al ? al.tone : 'magenta';
  }
  function nameForArtist(id) {
    const a = artists.find((x) => x.id === id);
    return a ? a.name : 'Unknown';
  }
  function albumNameFor(id) {
    const al = albums.find((a) => a.id === id);
    return al ? al.name : '';
  }

  function pushToast(tone, title, message) {
    const id = ++_uid;
    toasts = [...toasts, { id, tone, title, message }];
    setTimeout(() => { toasts = toasts.filter((t) => t.id !== id); }, 3400);
  }

  // ----- derived (getters) -----
  function match(s) { const q = query.trim().toLowerCase(); return !q || String(s).toLowerCase().includes(q); }

  return {
    // primitive state accessors
    get tab() { return tab; }, set tab(v) { tab = v; },
    get query() { return query; }, set query(v) { query = v; },
    get stream() { return stream; },
    get isPhone() { return isPhone; }, set isPhone(v) { isPhone = v; },
    get lineupOpen() { return lineupOpen; }, set lineupOpen(v) { lineupOpen = v; },
    get detailAlbumId() { return detailAlbumId; },
    get shuffle() { return shuffle; },
    get displayName() { return displayName; },
    set displayName(v) { displayName = v; localStorage.setItem('e66.name', v); },
    get toasts() { return toasts; },

    get albums() { return albums; },
    get artists() { return artists; },

    get listeners() { return listeners[stream]; },
    get queue() { return queues[stream]; },
    get nowPlaying() { return nowPlaying[stream]; },
    get progress() { return progress[stream]; },

    // filtered library
    get albumCards() {
      return albums.filter((a) => match(a.name) || match(a.artistName))
        .map((a) => ({ ...a, meta: `${a.tracks.length} tracks` }));
    },
    get artistRows() {
      return artists.filter((a) => match(a.name))
        .map((a) => ({ ...a, meta: `${a.albumCount} albums · ${a.trackCount} tracks` }));
    },
    get trackRows() {
      const all = albums.flatMap((a) => a.tracks);
      return all.filter((t) => match(t.title) || match(t.artistName) || match(t.albumName) || match(t.code));
    },
    get currentCount() {
      return this.tab === 'albums' ? this.albumCards.length
        : this.tab === 'artists' ? this.artistRows.length : this.trackRows.length;
    },
    get detailAlbum() { return albums.find((a) => a.id === detailAlbumId) || null; },

    // ----- actions -----
    async init() {
      await loadLibrary();
      await Promise.all([refreshQueue('house'), refreshQueue('me')]);
      _esHouse = subscribeEvents(HOUSE, (e) => {
        if (e.type === 'now-playing') {
          nowPlaying.house = e.data ? normalizeNP(e.data) : null;
          progress.house = 0;
        } else if (e.type === 'queue-changed') {
          refreshQueue('house');
        }
      });
    },
    teardown() { if (_esHouse) { _esHouse(); _esHouse = null; } },

    setStream(s) {
      if (s === stream) return;
      stream = s;
      setShuffle(s, shuffle);
      pushToast('cyan', 'Stream', s === 'house'
        ? 'Tuned in to the house stream — everyone hears this.'
        : 'Switched to your personal stream.');
    },
    toggleStream() { this.setStream(stream === 'house' ? 'me' : 'house'); },

    async toggleShuffle(v) {
      shuffle = typeof v === 'boolean' ? v : !shuffle;
      await setShuffle(stream, shuffle);
    },

    openAlbum(id) { detailAlbumId = id; },
    closeAlbum() { detailAlbumId = null; },
    openArtist(a) { tab = 'tracks'; query = a.name; },
    openLineup() { lineupOpen = true; },
    closeLineup() { lineupOpen = false; },
    onResize() { const ph = window.innerWidth < 760; isPhone = ph; if (!ph) lineupOpen = false; },

    async requestTrack(t) {
      await requestTo(stream, t.id, { kind: 'track', by: displayName });
      await refreshQueue(stream);
      pushToast('success', 'Queued', `${t.title} joined the lineup.`);
    },
    async requestAlbum(al) {
      await requestTo(stream, al.id, { kind: 'album', by: displayName });
      await refreshQueue(stream);
      pushToast('success', 'Queued', `${al.name} — ${al.tracks.length} tracks on the way.`);
    },
    async requestArtist(a) {
      await requestTo(stream, a.id, { kind: 'artist', by: displayName });
      await refreshQueue(stream);
      pushToast('success', 'Queued', `${a.name} — ${a.trackCount} tracks on the way.`);
    },
    async removeFromQueue(item) {
      await removeRequest(stream, item.id);
      await refreshQueue(stream);
    },
    dismissToast(id) { toasts = toasts.filter((t) => t.id !== id); },

    // progress tick (called once/sec by App for the active stream's now-playing)
    tick(seconds) { if (nowPlaying[stream]) progress[stream] = seconds; },
    setProgress(s, sec) { progress[s] = sec; },
    setNowPlaying(s, np) { nowPlaying[s] = np; },
  };

  function normalizeNP(t) {
    return {
      id: t.id, title: t.title, code: codeForTrack(t),
      artistName: nameForArtist(t.artist_id), albumName: albumNameFor(t.album_id),
      tone: toneForTrack(t), duration: t.duration || 0,
      cover: coverURL(t.id), gradient: gradientFor(t.id),
    };
  }
}
```

> Implementation note: `nowPlaying`, `progress`, `queues`, `listeners` are `$state` objects mutated by key (`nowPlaying.house = …`); Svelte 5 deep-reactivity tracks that. The getters expose the active stream's slice so components stay simple.

- [ ] **Step 2: Commit**

```bash
git add web/src/lib/store.svelte.js
git commit -m "feat(web): app store with library derivation + stream state"
```

---

# PHASE 3 — Components

> Each component is a focused `.svelte` file in `web/src/lib/components/`. Port styles **exactly** from the cited source. Where the source uses React `useState` for hover, use CSS `:hover` instead.

## Task 12: Leaf DS components — Toast, Switch, Avatar, SearchInput

**Files:** create `Toast.svelte`, `Switch.svelte`, `Avatar.svelte`, `SearchInput.svelte` in `web/src/lib/components/`.

Source: `docs/design-bundle/project/_ds/.../_ds_bundle.js` — Toast `643–740`, Switch `1196–1270`, Avatar `18–70`, Input `828–925`.

- [ ] **Step 1: `Toast.svelte`** (port of Toast)

```svelte
<script>
  let { tone = 'magenta', title = '', message = '', onClose } = $props();
  const C = {
    magenta: 'var(--neon-magenta)', cyan: 'var(--neon-cyan)', amber: 'var(--neon-amber)',
    success: 'var(--status-success)', danger: 'var(--status-danger)',
  };
</script>
<div role="status" style="display:flex; align-items:flex-start; gap:12px; min-width:280px; max-width:420px; padding:14px 16px; background:var(--bg-surface-raised); border-radius:var(--radius-md); border:1px solid var(--border-default); border-left:3px solid {C[tone] || C.magenta}; box-shadow:var(--shadow-lg);">
  <div style="flex:1; min-width:0;">
    {#if title}<div style="font-family:var(--font-display); font-weight:600; font-size:14px; letter-spacing:0.04em; text-transform:uppercase; color:var(--text-strong); margin-bottom:{message ? '3px' : '0'};">{title}</div>{/if}
    {#if message}<div style="font-family:var(--font-sans); font-size:13px; line-height:1.5; color:var(--text-muted);">{message}</div>{/if}
  </div>
  {#if onClose}<button onclick={onClose} aria-label="Dismiss" style="background:none; border:none; color:var(--text-faint); cursor:pointer; font-size:16px; line-height:1; padding:2px;">✕</button>{/if}
</div>
```

- [ ] **Step 2: `Switch.svelte`** (port of Switch — toggle only, no label)

```svelte
<script>
  let { checked = false, onChange, tone = 'magenta' } = $props();
  const T = {
    magenta: { c: 'var(--neon-magenta)', glow: 'var(--glow-soft-magenta)' },
    cyan: { c: 'var(--neon-cyan)', glow: 'var(--glow-soft-cyan)' },
    amber: { c: 'var(--neon-amber)', glow: 'var(--glow-amber)' },
  };
  const t = $derived(T[tone] || T.magenta);
</script>
<span role="switch" aria-checked={checked} onclick={() => onChange && onChange(!checked)} tabindex="0"
  style="width:46px; height:26px; flex:none; border-radius:var(--radius-pill); background:{checked ? t.c : 'var(--ink-700)'}; border:1px solid {checked ? t.c : 'var(--border-strong)'}; box-shadow:{checked ? t.glow : 'inset 0 1px 2px rgba(0,0,0,0.5)'}; cursor:pointer; position:relative; transition:all var(--dur) var(--ease-out); display:inline-block;">
  <span style="position:absolute; top:2px; left:{checked ? '22px' : '2px'}; width:20px; height:20px; border-radius:50%; background:{checked ? 'var(--text-on-accent)' : 'var(--paper-300)'}; transition:left var(--dur) var(--ease-snap); box-shadow:0 1px 3px rgba(0,0,0,0.5);"></span>
</span>
```

- [ ] **Step 3: `Avatar.svelte`** (port of Avatar)

```svelte
<script>
  let { name = '', size = 'md', ring = 'none' } = $props();
  const DIMS = { xs: 24, sm: 32, md: 40, lg: 56, xl: 80 };
  const RINGS = {
    none: 'none',
    magenta: '0 0 0 2px var(--bg-base), 0 0 0 4px var(--neon-magenta)',
    cyan: '0 0 0 2px var(--bg-base), 0 0 0 4px var(--neon-cyan)',
    amber: '0 0 0 2px var(--bg-base), 0 0 0 4px var(--neon-amber)',
  };
  const d = $derived(DIMS[size] || DIMS.md);
  const initials = $derived(name.split(' ').map((w) => w[0]).filter(Boolean).slice(0, 2).join('').toUpperCase());
</script>
<div style="width:{d}px; height:{d}px; flex:none; border-radius:var(--radius-pill); box-shadow:{RINGS[ring] || RINGS.none}; background:linear-gradient(140deg, var(--ink-700), var(--ink-850)); display:inline-flex; align-items:center; justify-content:center; overflow:hidden; font-family:var(--font-mono); font-weight:700; font-size:{d * 0.36}px; color:var(--paper-200); letter-spacing:0.02em;">{initials || '·'}</div>
```

- [ ] **Step 4: `SearchInput.svelte`** (port of Input, text-only, cyan focus)

```svelte
<script>
  let { value = '', onInput, placeholder = '', height = '42px' } = $props();
</script>
<div class="e66-input" style="display:flex; align-items:center; gap:10px; height:{height}; padding:0 14px; background:var(--bg-inset); border:1.5px solid var(--border-strong); border-radius:var(--radius-md); transition:all var(--dur) var(--ease-out);">
  <input {value} oninput={(e) => onInput && onInput(e.target.value)} {placeholder}
    style="flex:1; min-width:0; height:100%; background:transparent; border:none; outline:none; color:var(--text-strong); font-family:var(--font-sans); font-size:15px; letter-spacing:0.01em;" />
</div>
<style>
  .e66-input:focus-within { border-color: var(--neon-cyan); box-shadow: 0 0 0 2px rgba(31,224,255,0.5); }
</style>
```

- [ ] **Step 5: Build check**

Run: `cd web && npm run build`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/lib/components/{Toast,Switch,Avatar,SearchInput}.svelte
git commit -m "feat(web): Toast, Switch, Avatar, SearchInput components"
```

---

## Task 13: TrackRow + QueueItem

**Files:** create `TrackRow.svelte`, `QueueItem.svelte`.
Source: `_ds_bundle.js` TrackRow `1796–1940`, QueueItem `1643–1790`. Use `toneGradient` from `format.js`; real cover via `cover` prop with gradient fallback on `<img>` error.

- [ ] **Step 1: `TrackRow.svelte`**

```svelte
<script>
  import { toneGradient } from '../format.js';
  let { code = 'A6', title = 'Untitled', artist = 'Unknown', duration = '0:00',
        cover = null, gradient = null, tone = 'magenta', explicit = false,
        playing = false, onAdd, onClick } = $props();
  let artFailed = $state(false);
  const tile = $derived(gradient || toneGradient(tone));
</script>
<div class="tr" class:playing onclick={onClick}
  style="display:flex; align-items:center; gap:14px; padding:10px 12px; border-radius:var(--radius-md); cursor:pointer; transition:background var(--dur) var(--ease-out); border:1px solid {playing ? 'rgba(255,46,136,0.35)' : 'transparent'}; background:{playing ? 'rgba(255,46,136,0.08)' : 'transparent'};">
  <div style="position:relative; width:46px; height:46px; flex:none; border-radius:var(--radius-sm); overflow:hidden; box-shadow:{playing ? 'var(--glow-magenta)' : 'inset 0 0 0 1px rgba(255,255,255,0.08)'}; background:{tile}; display:flex; align-items:flex-end; padding:5px; box-sizing:border-box;">
    {#if cover && !artFailed}
      <img src={cover} alt="" onerror={() => (artFailed = true)} style="position:absolute; inset:0; width:100%; height:100%; object-fit:cover;" />
    {:else}
      <span style="font-family:var(--font-mono); font-size:9px; font-weight:700; letter-spacing:0.1em; color:rgba(255,255,255,0.85);">{code}</span>
    {/if}
    {#if playing}<span style="position:absolute; top:5px; right:5px; width:6px; height:6px; border-radius:50%; background:var(--neon-magenta);"></span>{/if}
  </div>
  <div style="flex:1; min-width:0;">
    <div style="display:flex; align-items:center; gap:8px;">
      <span style="font-family:var(--font-sans); font-weight:600; font-size:15px; color:{playing ? 'var(--neon-magenta-bright)' : 'var(--text-strong)'}; white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{title}</span>
      {#if explicit}<span style="font-family:var(--font-mono); font-size:9px; font-weight:700; color:var(--neon-amber); border:1px solid var(--neon-amber); border-radius:2px; padding:0 3px; line-height:13px; flex:none;">E</span>{/if}
    </div>
    <div style="font-family:var(--font-sans); font-size:13px; color:var(--text-muted); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{artist}</div>
  </div>
  <span style="font-family:var(--font-mono); font-size:13px; color:var(--text-faint); flex:none;">{duration}</span>
  {#if onAdd}
    <button class="add" aria-label="Add to queue" onclick={(e) => { e.stopPropagation(); onAdd(); }}
      style="width:34px; height:34px; flex:none; border-radius:var(--radius-sm); display:inline-flex; align-items:center; justify-content:center; background:var(--bg-surface-raised); color:var(--text-muted); border:1px solid var(--border-default); cursor:pointer; font-size:18px; line-height:1; transition:all var(--dur) var(--ease-out);">+</button>
  {/if}
</div>
<style>
  .tr:not(.playing):hover { background: var(--bg-surface-hover) !important; }
  .tr:hover .add { background: var(--neon-magenta); color: var(--text-on-accent); box-shadow: var(--glow-soft-magenta); }
</style>
```

- [ ] **Step 2: `QueueItem.svelte`**

```svelte
<script>
  import { toneGradient } from '../format.js';
  let { position = 1, title = 'Untitled', artist = 'Unknown', code = 'A6',
        requester = '', tone = 'magenta', onRemove } = $props();
  const initials = $derived(requester.split(' ').map((w) => w[0]).filter(Boolean).slice(0, 2).join('').toUpperCase());
</script>
<div class="qi" style="display:flex; align-items:center; gap:12px; padding:10px 12px; border-radius:var(--radius-md); background:var(--bg-surface); border:1px solid var(--border-default); transition:background var(--dur) var(--ease-out);">
  <span style="color:var(--text-disabled); font-size:14px; letter-spacing:-2px; flex:none;">⋮⋮</span>
  <span style="font-family:var(--font-mono); font-size:15px; font-weight:700; color:var(--neon-cyan); width:22px; text-align:center; flex:none;">{position}</span>
  <div style="width:38px; height:38px; flex:none; border-radius:var(--radius-sm); background:{toneGradient(tone)}; display:flex; align-items:flex-end; padding:4px; box-sizing:border-box;">
    <span style="font-family:var(--font-mono); font-size:8px; font-weight:700; color:rgba(255,255,255,0.85);">{code}</span>
  </div>
  <div style="flex:1; min-width:0;">
    <div style="font-family:var(--font-sans); font-weight:600; font-size:14px; color:var(--text-strong); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{title}</div>
    <div style="font-family:var(--font-sans); font-size:12px; color:var(--text-muted); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{artist}</div>
  </div>
  {#if requester}
    <div title={requester} style="width:28px; height:28px; flex:none; border-radius:50%; background:linear-gradient(140deg, var(--ink-700), var(--ink-850)); display:inline-flex; align-items:center; justify-content:center; font-family:var(--font-mono); font-size:11px; font-weight:700; color:var(--paper-200);">{initials}</div>
  {/if}
  {#if onRemove}<button class="rm" onclick={onRemove} aria-label="Remove" style="background:none; border:none; color:var(--text-faint); cursor:pointer; font-size:15px; flex:none;">✕</button>{/if}
</div>
<style>
  .qi:hover { background: var(--bg-surface-hover) !important; }
  .rm { opacity: 0; transition: opacity var(--dur) var(--ease-out); }
  .qi:hover .rm { opacity: 1; }
</style>
```

- [ ] **Step 3: Build + commit**

Run: `cd web && npm run build` → PASS.
```bash
git add web/src/lib/components/{TrackRow,QueueItem}.svelte
git commit -m "feat(web): TrackRow + QueueItem components"
```

---

## Task 14: NowPlayingBar

**Files:** create `NowPlayingBar.svelte`. Source: `_ds_bundle.js` NowPlayingBar `1377–1640`. Use `fmt`. Scrub + volume drag via pointer math.

- [ ] **Step 1: `NowPlayingBar.svelte`**

```svelte
<script>
  import { fmt, toneGradient } from '../format.js';
  let { title = 'Nothing playing', artist = '—', code = 'A6', cover = null, gradient = null,
        tone = 'magenta', current = 0, duration = 0, playing = false, volume = 70,
        onPlayPause, onPrev, onNext, onSeek, onVolume } = $props();
  let artFailed = $state(false);
  let scrub;
  const pct = $derived(duration ? Math.min(100, (current / duration) * 100) : 0);
  const tile = $derived(gradient || toneGradient(tone));
  function seekFrom(clientX) {
    if (!scrub || !onSeek) return;
    const r = scrub.getBoundingClientRect();
    onSeek(Math.max(0, Math.min(1, (clientX - r.left) / r.width)));
  }
  function volFrom(e) {
    const r = e.currentTarget.getBoundingClientRect();
    onVolume && onVolume(Math.round(Math.max(0, Math.min(1, (e.clientX - r.left) / r.width)) * 100));
  }
</script>
<div style="display:flex; align-items:center; gap:20px; height:84px; padding:0 22px; background:var(--bg-surface-raised); background-image:var(--scanline); border-top:1px solid var(--border-strong); box-shadow:0 -8px 30px rgba(0,0,0,0.5);">
  <!-- track -->
  <div style="display:flex; align-items:center; gap:14px; width:280px; flex:none;">
    <div style="position:relative; width:54px; height:54px; flex:none; border-radius:var(--radius-sm); overflow:hidden; background:{tile}; display:flex; align-items:flex-end; padding:6px; box-sizing:border-box; box-shadow:{playing ? 'var(--glow-soft-magenta)' : 'none'};">
      {#if cover && !artFailed}
        <img src={cover} alt="" onerror={() => (artFailed = true)} style="position:absolute; inset:0; width:100%; height:100%; object-fit:cover;" />
      {:else}
        <span style="font-family:var(--font-mono); font-size:10px; font-weight:700; color:rgba(255,255,255,0.85);">{code}</span>
      {/if}
    </div>
    <div style="min-width:0;">
      <div style="font-family:var(--font-sans); font-weight:600; font-size:15px; color:var(--text-strong); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{title}</div>
      <div style="font-family:var(--font-sans); font-size:13px; color:var(--text-muted); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{artist}</div>
    </div>
  </div>
  <!-- transport + scrub -->
  <div style="flex:1; min-width:0; display:flex; flex-direction:column; gap:7px; align-items:center;">
    <div style="display:flex; align-items:center; gap:10px;">
      <button class="t" aria-label="Previous" onclick={onPrev} style="width:38px; height:38px;">⏮</button>
      <button class="t primary" aria-label={playing ? 'Pause' : 'Play'} onclick={onPlayPause} style="width:46px; height:46px;">{playing ? '❚❚' : '▶'}</button>
      <button class="t" aria-label="Next" onclick={onNext} style="width:38px; height:38px;">⏭</button>
    </div>
    <div style="display:flex; align-items:center; gap:12px; width:100%; max-width:520px;">
      <span style="font-family:var(--font-mono); font-size:11px; color:var(--text-faint); width:38px; text-align:right;">{fmt(current)}</span>
      <div bind:this={scrub} onmousedown={(e) => seekFrom(e.clientX)} role="slider" tabindex="0" aria-label="Seek" aria-valuenow={Math.round(pct)} style="position:relative; flex:1; height:14px; display:flex; align-items:center; cursor:pointer;">
        <div style="position:absolute; left:0; right:0; height:4px; border-radius:var(--radius-pill); background:var(--ink-700);"></div>
        <div style="position:absolute; left:0; width:{pct}%; height:4px; border-radius:var(--radius-pill); background:var(--neon-magenta);"></div>
        <div style="position:absolute; left:calc({pct}% - 6px); width:12px; height:12px; border-radius:50%; background:var(--paper-100); border:2px solid var(--neon-magenta);"></div>
      </div>
      <span style="font-family:var(--font-mono); font-size:11px; color:var(--text-faint); width:38px;">{fmt(duration)}</span>
    </div>
  </div>
  <!-- volume -->
  <div style="display:flex; align-items:center; gap:10px; width:160px; flex:none;">
    <span style="color:var(--text-muted); font-size:16px;">♪</span>
    <div onmousedown={volFrom} role="slider" tabindex="0" aria-label="Volume" aria-valuenow={volume} style="position:relative; flex:1; height:14px; display:flex; align-items:center; cursor:pointer;">
      <div style="position:absolute; left:0; right:0; height:4px; border-radius:var(--radius-pill); background:var(--ink-700);"></div>
      <div style="position:absolute; left:0; width:{volume}%; height:4px; border-radius:var(--radius-pill); background:var(--neon-cyan);"></div>
      <div style="position:absolute; left:calc({volume}% - 6px); width:12px; height:12px; border-radius:50%; background:var(--paper-100); border:2px solid var(--neon-cyan);"></div>
    </div>
  </div>
</div>
<style>
  .t { flex:none; border-radius:50%; cursor:pointer; display:inline-flex; align-items:center; justify-content:center; font-size:17px; line-height:1; background:transparent; color:var(--text-body); border:1px solid transparent; transition:all var(--dur) var(--ease-out); }
  .t.primary { font-size:20px; background:var(--neon-magenta); color:var(--text-on-accent); border:none; }
  .t.primary:hover { box-shadow: var(--glow-magenta); }
</style>
```

- [ ] **Step 2: Build + commit**

Run: `cd web && npm run build` → PASS.
```bash
git add web/src/lib/components/NowPlayingBar.svelte
git commit -m "feat(web): NowPlayingBar component"
```

---

## Task 15: AlbumCard + AlbumGrid

**Files:** create `AlbumCard.svelte`, `AlbumGrid.svelte`. Source: `docs/design-bundle/project/JukeAlbumCard.dc.html` (lines 10–21).

- [ ] **Step 1: `AlbumCard.svelte`**

```svelte
<script>
  let { title, artist, meta, initial, cover = null, gradient = null, onOpen, onRequest } = $props();
  let artFailed = $state(false);
</script>
<div class="card" onclick={onOpen} style="position:relative; background:var(--bg-surface); background-image:var(--scanline); border:1px solid var(--border-default); border-radius:var(--radius-lg); overflow:hidden; cursor:pointer; transition:border-color var(--dur) var(--ease-out), transform var(--dur) var(--ease-out);">
  <div style="position:relative; aspect-ratio:1/1; overflow:hidden; background:{gradient};">
    {#if cover && !artFailed}
      <img src={cover} alt="" onerror={() => (artFailed = true)} style="position:absolute; inset:0; width:100%; height:100%; object-fit:cover;" />
    {/if}
    <span style="position:absolute; top:9px; left:10px; font-family:var(--font-mono); font-size:9px; letter-spacing:0.18em; color:rgba(246,243,235,0.62); text-transform:uppercase;">Art</span>
    {#if !cover || artFailed}
      <span style="position:absolute; left:50%; top:48%; transform:translate(-50%,-50%); font-family:var(--font-display); font-weight:700; font-size:72px; line-height:1; color:rgba(255,255,255,0.15);">{initial}</span>
    {/if}
    <button class="req" aria-label="Request album" onclick={(e) => { e.stopPropagation(); onRequest(); }}
      style="position:absolute; right:10px; bottom:10px; width:32px; height:32px; border-radius:var(--radius-md); border:1.5px solid var(--neon-magenta); background:var(--neon-magenta); color:var(--text-on-accent); font-size:19px; line-height:1; cursor:pointer; display:inline-flex; align-items:center; justify-content:center; box-shadow:var(--shadow-sm);">+</button>
  </div>
  <div style="padding:11px 13px 13px;">
    <div style="font-family:var(--font-sans); font-weight:600; font-size:14px; color:var(--text-strong); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{title}</div>
    <div style="font-family:var(--font-sans); font-size:13px; color:var(--text-muted); white-space:nowrap; overflow:hidden; text-overflow:ellipsis; margin-top:2px;">{artist}</div>
    <div style="font-family:var(--font-mono); font-size:10px; letter-spacing:0.13em; color:var(--text-faint); margin-top:9px; text-transform:uppercase;">{meta}</div>
  </div>
</div>
<style>
  .card:hover { border-color: var(--neon-magenta); transform: translateY(-2px); }
  .req:hover { background: var(--neon-magenta-bright); }
</style>
```

- [ ] **Step 2: `AlbumGrid.svelte`**

```svelte
<script>
  import AlbumCard from './AlbumCard.svelte';
  let { cards = [], onOpen, onRequest } = $props();
</script>
<div style="display:grid; grid-template-columns:repeat(auto-fill, minmax(150px, 1fr)); gap:14px;">
  {#each cards as a (a.id)}
    <AlbumCard title={a.name} artist={a.artistName} meta={a.meta} initial={a.initial}
      cover={a.cover} gradient={a.gradient}
      onOpen={() => onOpen(a)} onRequest={() => onRequest(a)} />
  {/each}
</div>
```

- [ ] **Step 3: Build + commit**

```bash
git add web/src/lib/components/{AlbumCard,AlbumGrid}.svelte
git commit -m "feat(web): AlbumCard + AlbumGrid"
```

---

## Task 16: ArtistList + TrackList

**Files:** create `ArtistList.svelte`, `TrackList.svelte`. Source: artist rows from `Exit 66 Jukebox.dc.html` lines 85–98; track list wraps `TrackRow`.

- [ ] **Step 1: `ArtistList.svelte`**

```svelte
<script>
  let { rows = [], onOpen, onRequest } = $props();
</script>
<div style="display:flex; flex-direction:column; gap:8px;">
  {#each rows as ar (ar.id)}
    <div class="row" onclick={() => onOpen(ar)} style="display:flex; align-items:center; gap:14px; padding:12px 14px; border:1px solid var(--border-default); border-radius:var(--radius-md); background:var(--bg-surface); cursor:pointer; transition:border-color var(--dur) var(--ease-out), background var(--dur) var(--ease-out);">
      <div style="width:46px; height:46px; flex:none; border-radius:50%; background:{ar.gradient}; display:flex; align-items:center; justify-content:center; font-family:var(--font-display); font-weight:700; font-size:18px; color:rgba(255,255,255,0.9); box-shadow:inset 0 0 0 1px rgba(255,255,255,0.1);">{ar.initial}</div>
      <div style="flex:1; min-width:0;">
        <div style="font-family:var(--font-sans); font-weight:600; font-size:15px; color:var(--text-strong); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{ar.name}</div>
        <div style="font-family:var(--font-mono); font-size:11px; letter-spacing:0.06em; color:var(--text-faint); margin-top:3px; text-transform:uppercase; white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{ar.meta}</div>
      </div>
      <button class="qa" onclick={(e) => { e.stopPropagation(); onRequest(ar); }}
        style="flex:none; height:34px; padding:0 14px; border:1px solid var(--neon-cyan); background:transparent; color:var(--neon-cyan); border-radius:var(--radius-md); cursor:pointer; font-family:var(--font-display); font-weight:600; font-size:12px; letter-spacing:0.08em; text-transform:uppercase; white-space:nowrap;">+ Queue all</button>
    </div>
  {/each}
</div>
<style>
  .row:hover { border-color: var(--neon-magenta); background: var(--bg-surface-hover); }
  .qa:hover { background: rgba(31,224,255,0.10); }
</style>
```

- [ ] **Step 2: `TrackList.svelte`**

```svelte
<script>
  import TrackRow from './TrackRow.svelte';
  import { fmt } from '../format.js';
  let { tracks = [], nowPlayingId = null, onAdd } = $props();
</script>
<div style="display:flex; flex-direction:column; gap:2px;">
  {#each tracks as t (t.code)}
    <TrackRow code={t.code} title={t.title} artist={t.artistName} duration={fmt(t.duration)}
      cover={t.cover} gradient={t.gradient} tone={t.tone}
      playing={t.id === nowPlayingId} onAdd={() => onAdd(t)} />
  {/each}
</div>
```

- [ ] **Step 3: Build + commit**

```bash
git add web/src/lib/components/{ArtistList,TrackList}.svelte
git commit -m "feat(web): ArtistList + TrackList"
```

---

## Task 17: Lineup

**Files:** create `Lineup.svelte`. Source: `docs/design-bundle/project/JukeLineup.dc.html`.

- [ ] **Step 1: `Lineup.svelte`**

```svelte
<script>
  import Switch from './Switch.svelte';
  import QueueItem from './QueueItem.svelte';
  let { streamLabel = 'House', listeners = 0, shuffle = false, onToggleShuffle,
        np = null, npPct = '0%', queue = [], isPhone = false, onClose, onRemove } = $props();
</script>
<div style="display:flex; flex-direction:column; gap:13px; height:100%; min-height:0; box-sizing:border-box;">
  <div style="display:flex; align-items:flex-start; justify-content:space-between; gap:12px;">
    <div style="min-width:0;">
      <div style="font-family:var(--font-display); font-weight:700; font-size:16px; letter-spacing:0.08em; text-transform:uppercase; color:var(--text-strong); white-space:nowrap;">The Lineup</div>
      <div style="font-family:var(--font-mono); font-size:10px; letter-spacing:0.14em; text-transform:uppercase; color:var(--text-faint); margin-top:4px;">{streamLabel} stream · {listeners} listening</div>
    </div>
    <div style="display:flex; align-items:center; gap:11px; flex:none;">
      <span style="display:inline-flex; align-items:center; gap:8px; font-family:var(--font-mono); font-size:10px; letter-spacing:0.14em; text-transform:uppercase; color:var(--text-faint); white-space:nowrap;">Shuffle<Switch checked={shuffle} onChange={onToggleShuffle} tone="magenta" /></span>
      {#if isPhone}
        <button aria-label="Close lineup" onclick={onClose} style="width:32px; height:32px; flex:none; border:1px solid var(--border-default); background:var(--bg-surface-raised); color:var(--text-muted); border-radius:var(--radius-sm); cursor:pointer; font-size:15px; line-height:1;">✕</button>
      {/if}
    </div>
  </div>

  {#if np}
    <div style="border:1.5px solid var(--neon-cyan); box-shadow:var(--glow-cyan); border-radius:var(--radius-md); background:var(--bg-inset); padding:12px; display:flex; flex-direction:column; gap:10px; flex:none;">
      <div style="display:flex; align-items:center; gap:12px;">
        <div style="width:46px; height:46px; flex:none; border-radius:var(--radius-sm); background:{np.gradient}; display:flex; align-items:flex-end; padding:5px; box-sizing:border-box; box-shadow:var(--glow-soft-cyan);"><span style="font-family:var(--font-mono); font-size:9px; font-weight:700; color:rgba(255,255,255,0.88);">{np.code}</span></div>
        <div style="flex:1; min-width:0;">
          <div style="font-family:var(--font-mono); font-size:9px; letter-spacing:0.18em; text-transform:uppercase; color:var(--neon-magenta); margin-bottom:3px; display:flex; align-items:center; gap:6px;"><span style="width:6px; height:6px; border-radius:50%; background:var(--neon-magenta);"></span>Now playing</div>
          <div style="font-family:var(--font-sans); font-weight:600; font-size:14px; color:var(--text-strong); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{np.title}</div>
          <div style="font-family:var(--font-sans); font-size:12px; color:var(--text-muted); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{np.artistName} · {np.albumName}</div>
        </div>
        <div style="display:flex; align-items:flex-end; gap:3px; height:20px; flex:none;">
          <span class="eq" style="animation-duration:.5s;"></span>
          <span class="eq" style="animation-duration:.7s;"></span>
          <span class="eq" style="animation-duration:.42s;"></span>
          <span class="eq" style="animation-duration:.62s;"></span>
        </div>
      </div>
      <div style="position:relative; height:4px; border-radius:var(--radius-pill); background:var(--ink-700);"><div style="position:absolute; left:0; top:0; bottom:0; width:{npPct}; border-radius:var(--radius-pill); background:var(--neon-magenta);"></div></div>
    </div>
  {/if}

  <div style="font-family:var(--font-mono); font-size:10px; letter-spacing:0.18em; text-transform:uppercase; color:var(--text-faint); flex:none;">Up next · {queue.length}</div>

  {#if queue.length === 0}
    <div style="flex:1; min-height:0; display:flex; flex-direction:column; align-items:center; justify-content:center; gap:10px; text-align:center; padding:24px 16px; border:1px dashed var(--border-strong); border-radius:var(--radius-md);">
      <div style="font-family:var(--font-display); font-weight:700; font-size:15px; letter-spacing:0.04em; text-transform:uppercase; color:var(--text-muted);">The floor is yours</div>
      <div style="font-family:var(--font-sans); font-size:13px; color:var(--text-faint); max-width:200px;">Nothing queued. Head to the crate and request the next track.</div>
    </div>
  {:else}
    <div style="flex:1; min-height:0; overflow-y:auto; display:flex; flex-direction:column; gap:8px; margin-right:-6px; padding-right:6px;">
      {#each queue as q, i (q.uid)}
        <QueueItem position={i + 1} code={q.code} title={q.title} artist={q.artistName}
          requester={q.requester} tone={q.tone} onRemove={() => onRemove(q)} />
      {/each}
    </div>
  {/if}
</div>
<style>
  .eq { width:3px; height:20px; background:var(--neon-cyan); transform-origin:bottom; animation-name:e66-eq; animation-timing-function:var(--ease-in-out); animation-iteration-count:infinite; animation-direction:alternate; }
</style>
```

- [ ] **Step 2: Build + commit**

```bash
git add web/src/lib/components/Lineup.svelte
git commit -m "feat(web): Lineup panel"
```

---

## Task 18: Tabs + TopBar + MobilePlayer + AlbumDialog

**Files:** create `Tabs.svelte`, `TopBar.svelte`, `MobilePlayer.svelte`, `AlbumDialog.svelte`. Source: `Exit 66 Jukebox.dc.html` (tab group 66–73, desktop bar 32–46, phone bar 48–60, mobile player 145–159, dialog 176–189).

- [ ] **Step 1: `Tabs.svelte`**

```svelte
<script>
  let { tab = 'albums', onTab } = $props();
  const TABS = [['artists', 'Artists'], ['albums', 'Albums'], ['tracks', 'Tracks']];
</script>
<div style="display:inline-flex; flex:none; border:1px solid var(--border-strong); border-radius:var(--radius-md); overflow:hidden; height:36px;">
  {#each TABS as [key, label]}
    <button onclick={() => onTab(key)}
      style="padding:0 16px; font-family:var(--font-display); font-weight:600; font-size:12px; letter-spacing:0.08em; text-transform:uppercase; border:none; cursor:pointer; background:{tab === key ? 'var(--neon-amber)' : 'transparent'}; color:{tab === key ? 'var(--text-on-accent)' : 'var(--text-muted)'};">{label}</button>
  {/each}
</div>
```

- [ ] **Step 2: `TopBar.svelte`** (desktop + phone variants by `isPhone`)

```svelte
<script>
  import SearchInput from './SearchInput.svelte';
  import Avatar from './Avatar.svelte';
  let { isPhone = false, query = '', onSearch, streamChipLabel = '', onToggleStream } = $props();
</script>
{#if !isPhone}
  <header style="height:62px; flex:none; display:flex; align-items:center; gap:24px; padding:0 24px; border-bottom:1px solid var(--border-default); background:var(--ink-950); z-index:5;">
    <div style="display:flex; align-items:center; gap:11px; flex:none;">
      <div style="width:34px; height:34px; flex:none; border-radius:var(--radius-md); border:1.5px solid var(--neon-magenta); box-shadow:0 0 0 2px var(--ink-950),0 0 0 3.5px var(--neon-magenta); display:flex; align-items:center; justify-content:center; font-family:var(--font-display); font-weight:700; font-size:17px; color:var(--neon-magenta-bright); background:rgba(255,46,136,0.05);">66</div>
      <div style="font-family:var(--font-display); font-weight:700; font-size:16px; letter-spacing:0.06em; color:var(--text-strong);">EXIT&nbsp;<span style="color:var(--neon-cyan);">66</span></div>
    </div>
    <div style="flex:1; max-width:480px; margin:0 auto;">
      <SearchInput value={query} onInput={onSearch} placeholder="Search the crate — artist, album, track, or slot code…" />
    </div>
    <div style="display:flex; align-items:center; gap:14px; flex:none;">
      <span style="display:inline-flex; align-items:center; gap:8px; padding:7px 12px; border:1px solid var(--border-default); border-radius:var(--radius-sm); font-family:var(--font-mono); font-size:11px; letter-spacing:0.1em; text-transform:uppercase; color:var(--text-muted); white-space:nowrap;"><span style="width:6px; height:6px; border-radius:50%; background:var(--neon-cyan);"></span>{streamChipLabel} listening</span>
      <Avatar name="You" ring="cyan" size="sm" />
    </div>
  </header>
{:else}
  <header style="flex:none; display:flex; flex-direction:column; gap:10px; padding:12px 14px; border-bottom:1px solid var(--border-default); background:var(--ink-950); z-index:5;">
    <div style="display:flex; align-items:center; justify-content:space-between; gap:12px;">
      <div style="display:flex; align-items:center; gap:9px;">
        <div style="width:30px; height:30px; flex:none; border-radius:var(--radius-md); border:1.5px solid var(--neon-magenta); box-shadow:0 0 0 2px var(--ink-950),0 0 0 3px var(--neon-magenta); display:flex; align-items:center; justify-content:center; font-family:var(--font-display); font-weight:700; font-size:15px; color:var(--neon-magenta-bright);">66</div>
        <div style="font-family:var(--font-display); font-weight:700; font-size:15px; letter-spacing:0.06em; color:var(--text-strong);">EXIT&nbsp;<span style="color:var(--neon-cyan);">66</span></div>
      </div>
      <button onclick={onToggleStream} style="display:inline-flex; align-items:center; gap:7px; padding:6px 11px; border:1px solid var(--border-strong); border-radius:var(--radius-sm); background:var(--bg-surface); font-family:var(--font-mono); font-size:10px; letter-spacing:0.1em; text-transform:uppercase; color:var(--text-body); cursor:pointer; white-space:nowrap;"><span style="width:6px; height:6px; border-radius:50%; background:var(--neon-cyan);"></span>{streamChipLabel}</button>
    </div>
    <SearchInput value={query} onInput={onSearch} placeholder="Search the crate…" />
  </header>
{/if}
```

- [ ] **Step 3: `MobilePlayer.svelte`**

```svelte
<script>
  let { np = null, npPct = '0%', playing = true, onPlayPause, onNext } = $props();
</script>
<div style="flex:none; position:relative; border-top:1px solid var(--border-strong); background:var(--bg-surface-raised); background-image:var(--scanline);">
  <div style="position:absolute; top:0; left:0; right:0; height:3px; background:var(--ink-700);"><div style="height:100%; width:{npPct}; background:var(--neon-magenta);"></div></div>
  <div style="display:flex; align-items:center; gap:12px; padding:11px 14px;">
    <div style="width:44px; height:44px; flex:none; border-radius:var(--radius-sm); background:{np ? np.gradient : 'var(--ink-700)'}; display:flex; align-items:flex-end; padding:5px; box-sizing:border-box;"><span style="font-family:var(--font-mono); font-size:9px; font-weight:700; color:rgba(255,255,255,0.85);">{np ? np.code : '··'}</span></div>
    <div style="flex:1; min-width:0;">
      <div style="font-family:var(--font-sans); font-weight:600; font-size:14px; color:var(--text-strong); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{np ? np.title : 'Nothing playing'}</div>
      <div style="font-family:var(--font-sans); font-size:12px; color:var(--text-muted); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{np ? `${np.artistName} · ${np.albumName}` : '—'}</div>
    </div>
    <button aria-label="Play / pause" onclick={onPlayPause} style="width:42px; height:42px; flex:none; border-radius:50%; border:none; background:var(--neon-magenta); color:var(--text-on-accent); font-size:18px; cursor:pointer;">{playing ? '❚❚' : '▶'}</button>
    <button aria-label="Next" onclick={onNext} style="width:38px; height:38px; flex:none; border-radius:50%; border:1px solid var(--border-strong); background:transparent; color:var(--text-body); font-size:16px; cursor:pointer;">⏭</button>
  </div>
</div>
```

- [ ] **Step 4: `AlbumDialog.svelte`** (port of Dialog + album track list)

```svelte
<script>
  import TrackRow from './TrackRow.svelte';
  import { fmt } from '../format.js';
  let { album = null, nowPlayingId = null, onClose, onRequestAll, onAddTrack } = $props();
</script>
{#if album}
  <div onclick={onClose} style="position:fixed; inset:0; z-index:100; display:flex; align-items:center; justify-content:center; padding:24px; background:rgba(6,6,11,0.72); backdrop-filter:blur(6px); -webkit-backdrop-filter:blur(6px);">
    <div role="dialog" aria-modal="true" onclick={(e) => e.stopPropagation()}
      style="width:100%; max-width:460px; background:var(--bg-surface-raised); background-image:var(--scanline); border:1px solid var(--border-strong); border-radius:var(--radius-xl); box-shadow:var(--shadow-xl); overflow:hidden;">
      <div style="height:3px; background:linear-gradient(90deg, var(--neon-magenta), var(--neon-cyan));"></div>
      <div style="padding:var(--space-7);">
        <div style="display:flex; align-items:flex-start; justify-content:space-between; gap:16px; margin-bottom:14px;">
          <div>
            <div style="font-family:var(--font-mono); font-size:11px; letter-spacing:0.22em; text-transform:uppercase; color:var(--neon-cyan); margin-bottom:6px;">{album.artistName}</div>
            <div style="font-family:var(--font-display); font-weight:700; font-size:24px; letter-spacing:0.02em; text-transform:uppercase; color:var(--text-strong);">{album.name}</div>
          </div>
          <button onclick={onClose} aria-label="Close" style="background:none; border:none; color:var(--text-faint); font-size:20px; cursor:pointer; line-height:1;">✕</button>
        </div>
        <div style="display:flex; flex-direction:column; gap:14px;">
          <div style="display:flex; align-items:center; justify-content:space-between; gap:12px;">
            <span style="font-family:var(--font-mono); font-size:11px; letter-spacing:0.16em; text-transform:uppercase; color:var(--text-faint);">{album.tracks.length} tracks</span>
            <button class="qall" onclick={onRequestAll} style="height:38px; padding:0 18px; border:1.5px solid var(--neon-magenta); background:var(--neon-magenta); color:var(--text-on-accent); border-radius:var(--radius-md); cursor:pointer; font-family:var(--font-display); font-weight:600; font-size:13px; letter-spacing:0.08em; text-transform:uppercase;">Queue all</button>
          </div>
          <div style="display:flex; flex-direction:column; gap:2px; max-height:336px; overflow-y:auto; margin-right:-6px; padding-right:6px;">
            {#each album.tracks as t (t.code)}
              <TrackRow code={t.code} title={t.title} artist={t.artistName} duration={fmt(t.duration)}
                cover={t.cover} gradient={t.gradient} tone={t.tone}
                playing={t.id === nowPlayingId} onAdd={() => onAddTrack(t)} />
            {/each}
          </div>
        </div>
      </div>
    </div>
  </div>
{/if}
<style>
  .qall:hover { background: var(--neon-magenta-bright); border-color: var(--neon-magenta-bright); }
</style>
```

- [ ] **Step 5: Build + commit**

```bash
git add web/src/lib/components/{Tabs,TopBar,MobilePlayer,AlbumDialog}.svelte
git commit -m "feat(web): Tabs, TopBar, MobilePlayer, AlbumDialog"
```

---

# PHASE 4 — Assembly + wiring

## Task 19: App.svelte — desktop + phone shell, players, audio, ticks

**Files:** rewrite `web/src/App.svelte`. Source layout: `Exit 66 Jukebox.dc.html` lines 29–200.

- [ ] **Step 1: Rewrite `App.svelte`**

```svelte
<script>
  import { onMount, onDestroy } from 'svelte';
  import { createStore } from './lib/store.svelte.js';
  import { audioURL, houseStreamURL, nextTrack } from './lib/api.js';
  import { fmt } from './lib/format.js';
  import TopBar from './lib/components/TopBar.svelte';
  import Tabs from './lib/components/Tabs.svelte';
  import AlbumGrid from './lib/components/AlbumGrid.svelte';
  import ArtistList from './lib/components/ArtistList.svelte';
  import TrackList from './lib/components/TrackList.svelte';
  import Lineup from './lib/components/Lineup.svelte';
  import NowPlayingBar from './lib/components/NowPlayingBar.svelte';
  import MobilePlayer from './lib/components/MobilePlayer.svelte';
  import AlbumDialog from './lib/components/AlbumDialog.svelte';
  import Toast from './lib/components/Toast.svelte';

  const s = createStore();
  let audio;
  let playing = $state(true);
  let volume = $state(68);
  let tickTimer, resizeHandler;

  // Active now-playing slice + derived progress %.
  const np = $derived(s.nowPlaying);
  const dur = $derived(np?.duration || 0);
  const cur = $derived(Math.min(s.progress, dur || s.progress));
  const npPct = $derived((dur ? Math.min(100, (cur / dur) * 100) : 0) + '%');
  const streamLabel = $derived(s.stream === 'house' ? 'House' : 'Personal');
  const chip = $derived(`${streamLabel} · ${s.listeners}`);

  // ---- personal (me) playback: client audio drives now-playing ----
  async function advancePersonal() {
    const r = await nextTrack(); // GET /api/streams/me/next
    if (r && r.ok && r.track) {
      s.setNowPlaying('me', normalize(r.track));
      s.setProgress('me', 0);
      audio.src = audioURL(r.track.id);
      if (playing) audio.play().catch(() => {});
    } else {
      s.setNowPlaying('me', null);
      playing = false;
    }
  }
  function normalize(t) {
    // mirror store.normalizeNP via exported helper if present; minimal inline:
    return { id: t.id, title: t.title, duration: t.duration || 0, ...s.npMeta(t) };
  }

  function applyStreamAudio() {
    if (!audio) return;
    if (s.stream === 'house') {
      audio.src = houseStreamURL();
      if (playing) audio.play().catch(() => {});
    } else if (!s.nowPlaying) {
      advancePersonal();
    } else {
      audio.src = audioURL(s.nowPlaying.id);
      if (playing) audio.play().catch(() => {});
    }
  }

  function togglePlay() {
    playing = !playing;
    if (!audio) return;
    if (playing) audio.play().catch(() => {}); else audio.pause();
  }
  function onNext() {
    if (s.stream === 'me') advancePersonal();
    // house: next is server-driven; SSE will update now-playing.
  }
  function onPrev() { s.setProgress(s.stream, 0); if (audio && s.stream === 'me') audio.currentTime = 0; }
  function onSeek(frac) {
    const t = Math.round(frac * dur);
    s.setProgress(s.stream, t);
    if (audio && s.stream === 'me') audio.currentTime = t;
  }

  onMount(async () => {
    await s.init();
    s.onResize();
    resizeHandler = () => s.onResize();
    window.addEventListener('resize', resizeHandler);
    applyStreamAudio();
    // 1s tick: personal reads exact audio time; house approximates.
    tickTimer = setInterval(() => {
      if (!playing || !s.nowPlaying) return;
      if (s.stream === 'me' && audio && !audio.paused) {
        s.setProgress('me', audio.currentTime);
      } else {
        s.setProgress(s.stream, s.progress + 1);
      }
    }, 1000);
    if (audio) audio.addEventListener('ended', () => { if (s.stream === 'me') advancePersonal(); });
  });
  onDestroy(() => {
    clearInterval(tickTimer);
    window.removeEventListener('resize', resizeHandler);
    s.teardown();
    if (audio) { audio.pause(); audio.src = ''; }
  });

  // re-apply audio when the user switches streams
  let lastStream = 'house';
  $effect(() => {
    if (s.stream !== lastStream) { lastStream = s.stream; playing = true; applyStreamAudio(); }
  });
</script>

<div style="position:relative; height:100vh; width:100%; display:flex; flex-direction:column; overflow:hidden; box-sizing:border-box; background:var(--grid-glow), var(--bg-base); font-family:var(--font-sans); color:var(--text-body);">

  <TopBar isPhone={s.isPhone} query={s.query} onSearch={(v) => (s.query = v)}
    streamChipLabel={chip} onToggleStream={() => s.toggleStream()} />

  <!-- BODY -->
  <div style="display:flex; flex:1; min-height:0;">
    <main style="flex:1; min-width:0; display:flex; flex-direction:column; padding:18px 22px; box-sizing:border-box;">
      <div style="display:flex; align-items:center; justify-content:space-between; gap:14px; margin-bottom:16px;">
        <Tabs tab={s.tab} onTab={(t) => (s.tab = t)} />
        <span style="font-family:var(--font-mono); font-size:11px; letter-spacing:0.16em; text-transform:uppercase; color:var(--text-faint); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{s.currentCount} in the crate</span>
      </div>

      <div style="flex:1; min-height:0; overflow-y:auto; margin-right:-8px; padding-right:8px;">
        {#if s.currentCount === 0}
          <div style="display:flex; flex-direction:column; align-items:center; justify-content:center; gap:10px; text-align:center; padding:70px 20px;">
            <div style="font-family:var(--font-display); font-weight:700; font-size:18px; letter-spacing:0.04em; text-transform:uppercase; color:var(--text-muted);">No matches on this side</div>
            <div style="font-family:var(--font-sans); font-size:14px; color:var(--text-faint); max-width:300px;">Nothing in the crate matches that. Try another artist, album, or slot code.</div>
          </div>
        {:else if s.tab === 'albums'}
          <AlbumGrid cards={s.albumCards} onOpen={(a) => s.openAlbum(a.id)} onRequest={(a) => s.requestAlbum(a)} />
        {:else if s.tab === 'artists'}
          <ArtistList rows={s.artistRows} onOpen={(a) => s.openArtist(a)} onRequest={(a) => s.requestArtist(a)} />
        {:else}
          <TrackList tracks={s.trackRows} nowPlayingId={np?.id} onAdd={(t) => s.requestTrack(t)} />
        {/if}
      </div>
    </main>

    {#if !s.isPhone}
      <aside style="width:328px; flex:none; padding:18px 18px 18px 0; min-height:0; display:flex;">
        <div style="flex:1; min-height:0; display:flex; background:var(--bg-surface); background-image:var(--scanline); border:1.5px solid var(--neon-magenta); box-shadow:var(--shadow-lg), var(--glow-soft-magenta); border-radius:var(--radius-lg); padding:16px; box-sizing:border-box;">
          <Lineup streamLabel={streamLabel} listeners={s.listeners} shuffle={s.shuffle}
            onToggleShuffle={(v) => s.toggleShuffle(v)} np={np} npPct={npPct}
            queue={s.queue} isPhone={false} onRemove={(q) => s.removeFromQueue(q)} />
        </div>
      </aside>
    {/if}
  </div>

  <!-- DESKTOP PLAYER -->
  {#if !s.isPhone}
    <div style="flex:none; display:flex; align-items:stretch;">
      <div style="flex:1; min-width:0;">
        <NowPlayingBar title={np?.title || 'Nothing playing'} artist={np?.artistName || '—'}
          code={np?.code || 'A6'} cover={np?.cover} gradient={np?.gradient} tone={np?.tone || 'magenta'}
          current={cur} duration={dur} {playing} {volume}
          onPlayPause={togglePlay} onPrev={onPrev} onNext={onNext} onSeek={onSeek}
          onVolume={(v) => { volume = v; if (audio) audio.volume = v / 100; }} />
      </div>
      <div style="width:220px; flex:none; border-top:1px solid var(--border-strong); border-left:1px solid var(--border-default); background:var(--bg-surface-raised); background-image:var(--scanline); display:flex; flex-direction:column; justify-content:center; gap:8px; padding:0 18px; box-sizing:border-box;">
        <span style="font-family:var(--font-mono); font-size:9px; letter-spacing:0.22em; text-transform:uppercase; color:var(--text-faint);">Stream</span>
        <div style="display:inline-flex; border:1px solid var(--border-strong); border-radius:var(--radius-sm); overflow:hidden; width:fit-content;">
          <button onclick={() => s.setStream('house')} style="padding:5px 12px; border:none; cursor:pointer; font-family:var(--font-mono); font-size:11px; font-weight:700; letter-spacing:0.08em; text-transform:uppercase; background:{s.stream === 'house' ? 'var(--neon-cyan)' : 'transparent'}; color:{s.stream === 'house' ? 'var(--text-on-accent)' : 'var(--text-muted)'};">House</button>
          <button onclick={() => s.setStream('me')} style="padding:5px 12px; border:none; cursor:pointer; font-family:var(--font-mono); font-size:11px; font-weight:700; letter-spacing:0.08em; text-transform:uppercase; background:{s.stream === 'me' ? 'var(--neon-cyan)' : 'transparent'}; color:{s.stream === 'me' ? 'var(--text-on-accent)' : 'var(--text-muted)'};">Personal</button>
        </div>
        <span style="display:inline-flex; align-items:center; gap:7px; font-family:var(--font-mono); font-size:10px; letter-spacing:0.1em; color:var(--text-faint);"><span style="width:6px; height:6px; border-radius:50%; background:var(--neon-cyan);"></span>{s.listeners} TUNED IN</span>
      </div>
    </div>
  {/if}

  <!-- MOBILE PLAYER -->
  {#if s.isPhone}
    <MobilePlayer {np} {npPct} {playing} onPlayPause={togglePlay} onNext={onNext} />
  {/if}

  <!-- PHONE LINEUP FAB -->
  {#if s.isPhone && !s.lineupOpen}
    <button onclick={() => s.openLineup()} style="position:absolute; right:16px; bottom:86px; z-index:60; height:46px; padding:0 16px; border-radius:var(--radius-md); background:var(--neon-magenta); color:var(--text-on-accent); border:none; box-shadow:var(--shadow-lg), var(--glow-soft-magenta); font-family:var(--font-display); font-weight:600; font-size:13px; letter-spacing:0.08em; text-transform:uppercase; display:inline-flex; align-items:center; gap:9px; cursor:pointer;">The Lineup<span style="font-family:var(--font-mono); font-weight:700; font-size:12px; padding:2px 7px; border-radius:var(--radius-sm); background:rgba(11,11,20,0.25);">{s.queue.length}</span></button>
  {/if}

  <!-- PHONE LINEUP SHEET -->
  {#if s.isPhone && s.lineupOpen}
    <div style="position:absolute; inset:0; z-index:75; display:flex; flex-direction:column; justify-content:flex-end;">
      <div onclick={() => s.closeLineup()} style="position:absolute; inset:0; background:rgba(6,6,11,0.72); backdrop-filter:blur(6px);"></div>
      <div style="position:relative; height:74vh; background:var(--bg-surface); background-image:var(--scanline); border-top:1.5px solid var(--neon-magenta); border-radius:var(--radius-lg) var(--radius-lg) 0 0; padding:18px; box-shadow:var(--shadow-xl); display:flex; box-sizing:border-box;">
        <Lineup streamLabel={streamLabel} listeners={s.listeners} shuffle={s.shuffle}
          onToggleShuffle={(v) => s.toggleShuffle(v)} np={np} npPct={npPct}
          queue={s.queue} isPhone={true} onClose={() => s.closeLineup()} onRemove={(q) => s.removeFromQueue(q)} />
      </div>
    </div>
  {/if}

  <!-- ALBUM DIALOG -->
  <AlbumDialog album={s.detailAlbum} nowPlayingId={np?.id}
    onClose={() => s.closeAlbum()} onRequestAll={() => { s.requestAlbum(s.detailAlbum); s.closeAlbum(); }}
    onAddTrack={(t) => s.requestTrack(t)} />

  <!-- TOASTS -->
  <div style="position:absolute; top:16px; right:16px; display:flex; flex-direction:column; gap:10px; z-index:95; max-width:320px;">
    {#each s.toasts as t (t.id)}
      <div style="animation:e66-toast-in .28s var(--ease-out);">
        <Toast tone={t.tone} title={t.title} message={t.message} onClose={() => s.dismissToast(t.id)} />
      </div>
    {/each}
  </div>

  <audio bind:this={audio} style="display:none;"></audio>
</div>
```

> The `normalize`/`s.npMeta` helper: add a small `npMeta(track)` method to the store that returns `{ code, artistName, albumName, tone, cover, gradient }` for a raw backend track (reuse the private `codeForTrack`/`nameForArtist`/`albumNameFor`/`toneForTrack`/`coverURL`/`gradientFor` already in the store). Export it on the returned object alongside the others. This keeps all derivation in the store.

- [ ] **Step 2: Add `npMeta` to the store**

In `web/src/lib/store.svelte.js`, add to the returned object:

```js
    npMeta(t) {
      return {
        code: codeForTrack(t), artistName: nameForArtist(t.artist_id),
        albumName: albumNameFor(t.album_id), tone: toneForTrack(t),
        cover: coverURL(t.id), gradient: gradientFor(t.id),
      };
    },
```

- [ ] **Step 3: Build**

Run: `cd web && npm run build`
Expected: PASS, no Svelte compile errors.

- [ ] **Step 4: Commit**

```bash
git add web/src/App.svelte web/src/lib/store.svelte.js
git commit -m "feat(web): assemble Crate-wall app shell + playback wiring"
```

---

## Task 20: Vite dev proxy for /stream

**Files:** modify `web/vite.config.js`.

- [ ] **Step 1: Add the proxy entry**

```js
  server: {
    proxy: {
      '/api': 'http://localhost:8066',
      '/stream': 'http://localhost:8066',
    },
  },
```

- [ ] **Step 2: Commit**

```bash
git add web/vite.config.js
git commit -m "chore(web): proxy /stream in dev"
```

---

## Task 21: Full backend test sweep + binary build

- [ ] **Step 1: Backend tests**

Run: `go test ./...`
Expected: PASS across all packages.

- [ ] **Step 2: Vet**

Run: `go vet ./...`
Expected: clean.

- [ ] **Step 3: Full build (UI + binary)**

Run: `make build`
Expected: `make ui` builds the Svelte app into `internal/web/dist`, then `go build` produces `exit66jukebox` with the UI embedded.

- [ ] **Step 4: Commit the rebuilt embedded UI**

```bash
git add internal/web/dist
git commit -m "build: embed Crate-wall UI"
```

---

## Task 22: Manual verification

> No JS test harness exists; verify by running the app. (Per project README, only screenshot if needed.)

- [ ] **Step 1: Run with a small library**

Run: `./exit66jukebox --library <path-to-a-few-albums>` (see `internal/config` for the flag name; use whatever the existing run path expects). Open `http://localhost:8066`.

- [ ] **Step 2: Desktop checklist** — confirm visually:
  - Top bar: 66 badge, EXIT 66 wordmark, centered search, stream chip + avatar.
  - Tabs switch Artists/Albums/Tracks; count label updates; search filters live (including a slot code like `A1`).
  - Album grid shows real covers where present, gradient + big initial where not.
  - Click a card → album dialog with track rows + "Queue all".
  - `+` on a card / "Queue all" on an artist / `+` on a track row → toast + item appears in the lineup with your name's avatar.
  - Lineup rail: now-playing card with equalizer + progress; remove (✕) on hover.
  - Player: play/pause, scrub, volume; House/Personal switch changes now-playing + queue + "TUNED IN" count.
  - Shuffle toggle flips; next track after current ends is random (personal stream, queue ≥2).

- [ ] **Step 3: Phone checklist** — narrow window < 760px:
  - Compact top bar with tap-to-toggle stream chip; single-column search.
  - 2-up album grid; mobile player with progress bar; "The Lineup" FAB with count.
  - Tap FAB → bottom sheet lineup with shuffle + close; scrim tap closes.

- [ ] **Step 4: Console check** — no errors in devtools console during the above.

- [ ] **Step 5: Final commit (if any verification fixups)**

```bash
git add -A
git commit -m "fix: verification fixups for Crate-wall UI"
```

---

## Done criteria

- `go test ./...` and `go vet ./...` pass; CI green.
- `make build` produces a binary serving the neon-noir Crate-wall UI.
- Desktop and phone layouts match the bundle; request/queue/remove/stream-switch/shuffle/search all work against the real backend; requester avatars and house listener count are real; fonts load with no internet.
