# Exit 66 Jukebox v1 — Plan 2 of 2: Shared Stream & Live Updates

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an always-on shared "house" stream — one continuous server-side MP3 feed fed by the shared fairness queue that any browser, VLC, or (later) Sonos can tune into — plus live now-playing/queue updates over SSE, a Local/House toggle in the UI, and the two deferred library polish items (track duration, cover art).

**Architecture:** A fixed shared stream `house` always exists. Its queue uses the existing `jukebox` fairness logic. A `broadcast.Hub` runs a loop that pops the next track from the house queue and transcodes it to MP3 **paced to real-time** (`ffmpeg -re`), fanning the bytes out to every HTTP listener on `GET /stream/house.mp3`; when the queue is empty it emits pre-generated MP3 silence so listeners (and Sonos) stay connected. Real-time pacing makes everyone hear the same position and lets late-joiners drop into "now" — no per-track timing logic needed (ffmpeg exits at end-of-track, the loop advances). A per-stream `events.Bus` pushes `now-playing` and `queue-changed` events to browsers over SSE. The Svelte UI gains a Local/House toggle: Local keeps Plan 1's per-device pull playback; House plays the stream URL, sends requests to the house queue, and renders now-playing + queue from SSE. Duration is probed with `ffprobe` during the (incremental, concurrent) scan; cover art is extracted from embedded tags on demand.

**Tech Stack:** Go 1.26 stdlib (`os/exec` for ffmpeg/ffprobe, `net/http` SSE via `text/event-stream`), existing deps only. Svelte UI (existing).

**Module path:** `github.com/andybarilla/exit66jukebox`. **Builds on Plan 1** (branch `revival`, PR #7).

---

## Design decisions (settled)

- **One house stream**, id `house`, created at startup. The data model already supports more (named streams) for later; this plan wires exactly one but keeps the lookup a map keyed by stream id so adding more is additive.
- **Real-time pacing via `ffmpeg -re`** is what makes the stream "shared" (synchronized). Without it ffmpeg would blast the whole track to buffers instantly and the queue would race ahead.
- **Silence when idle** (pre-generated ~1s MP3 looped) keeps the HTTP/Sonos connection alive between/without tracks.
- **The broadcast hub is dumb and testable**: it depends on an injected `Source` (real = ffmpeg, fake = canned bytes) and a `next func() (path string, ok bool)` callback. The wiring closure in `main` does the actual `jukebox.Next` pop + event publish, so the hub has no DB/jukebox coupling.
- **Server gains a stream registry** (`map[string]*broadcast.Hub` + `map[string]*events.Bus`) populated via a method, so `NewServer`'s signature is unchanged and Plan 1's API tests keep passing.

## File Structure

| Path | Responsibility |
|------|----------------|
| `internal/scan/duration.go` | `probeDuration(path) int` via ffprobe |
| `internal/scan/scanner.go` (modify) | set `Duration` on scanned tracks |
| `internal/api/cover.go` | `GET /api/tracks/{id}/cover`, `GET /api/albums/{id}/cover` |
| `internal/store/library.go` (modify) | `FirstTrackIDOfAlbum` helper |
| `internal/events/bus.go` | per-stream publish/subscribe event bus |
| `internal/broadcast/source.go` | `Source` interface + `FFmpegSource` + `GenerateSilence` |
| `internal/broadcast/hub.go` | fan-out hub: `Listen`, `Run` loop, advance/idle |
| `internal/api/stream.go` | `GET /stream/{id}.mp3` (hub fan-out), `GET /api/streams/{id}/events` (SSE); stream registry on `Server` |
| `internal/api/streams.go` (modify) | publish `queue-changed` on request/remove/clear |
| `internal/api/server.go` (modify) | register the two new routes; add registry fields + `RegisterStream` |
| `main.go` (modify) | create house bus + hub, wire `next` closure, register, run hub |
| `web/src/lib/api.js` (modify) | house request/stream URLs, SSE helper, cover URL |
| `web/src/lib/player.js` (modify) | dual-mode (local pull vs house stream) |
| `web/src/App.svelte` (modify) | Local/House toggle, SSE now-playing + queue, cover art |

---

## Phase 1 — Library polish: duration + cover art

### Task 1.1: Probe track duration with ffprobe

**Files:** Create `internal/scan/duration.go`, `internal/scan/duration_test.go`; modify `internal/scan/scanner.go`.

- [ ] **Step 1: Write the failing test**

`internal/scan/duration_test.go`:

```go
package scan

import "testing"

func TestProbeDurationReadsFixtureLength(t *testing.T) {
	d := probeDuration("testdata/sample.mp3")
	if d <= 0 {
		t.Fatalf("expected positive duration for 1s fixture, got %d", d)
	}
}

func TestProbeDurationMissingFileReturnsZero(t *testing.T) {
	if d := probeDuration("testdata/does-not-exist.mp3"); d != 0 {
		t.Fatalf("expected 0 for missing file, got %d", d)
	}
}
```

- [ ] **Step 2: Run the test, expect failure**

Run: `mise exec -- go test ./internal/scan/ -run TestProbeDuration`
Expected: FAIL — `undefined: probeDuration`

- [ ] **Step 3: Implement**

`internal/scan/duration.go`:

```go
package scan

import (
	"os/exec"
	"strconv"
	"strings"
)

// probeDuration returns the track length in whole seconds via ffprobe, or 0 if
// ffprobe is unavailable or fails. Best-effort: never errors the scan.
func probeDuration(path string) int {
	out, err := exec.Command("ffprobe", "-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", path).Output()
	if err != nil {
		return 0
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0
	}
	return int(f)
}
```

- [ ] **Step 4: Run the test, expect pass**

Run: `mise exec -- go test ./internal/scan/ -run TestProbeDuration`
Expected: PASS

- [ ] **Step 5: Wire duration into the scan worker**

In `internal/scan/scanner.go`, inside the worker loop where the `model.Track` is built (currently sets Path/ModTime/Size/Title/TrackNo/Genre), add the duration probe. Change the track construction to:

```go
				tr := model.Track{
					Path: j.path, ModTime: j.modTime, Size: j.size,
					Title: meta.Title, TrackNo: meta.TrackNo, Genre: meta.Genre,
					Duration: probeDuration(j.path),
				}
```

- [ ] **Step 6: Extend the scan test to assert duration is stored**

Append to `internal/scan/scanner_test.go`:

```go
func TestScanStoresDuration(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	dir := t.TempDir()
	src, _ := os.ReadFile("testdata/sample.mp3")
	os.WriteFile(filepath.Join(dir, "a.mp3"), src, 0o644)

	if _, err := Scan(db, []string{dir}, 2); err != nil {
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
```

- [ ] **Step 7: Run scan tests, expect pass**

Run: `mise exec -- go test ./internal/scan/`
Expected: PASS (including TestScanStoresDuration)

- [ ] **Step 8: Commit**

```bash
git add internal/scan/
git commit -m "feat: probe track duration with ffprobe during scan"
```

> Note: this adds one `ffprobe` exec per new/changed file. Scanning stays incremental (unchanged files skipped) and concurrent (worker pool), so it is a one-time cost per file. Acceptable for a household library.

### Task 1.2: Cover-art endpoints

**Files:** Create `internal/api/cover.go`; modify `internal/store/library.go` (add `FirstTrackIDOfAlbum`); modify `internal/api/server.go` (register two routes); add `internal/api/cover_test.go`.

- [ ] **Step 1: Add the store helper**

Append to `internal/store/library.go`:

```go
// FirstTrackIDOfAlbum returns the lowest-numbered track id for an album, or
// ok=false if the album has no tracks.
func FirstTrackIDOfAlbum(db *sql.DB, albumID int64) (int64, bool) {
	var id int64
	err := db.QueryRow(
		`SELECT id FROM track WHERE album_id=? ORDER BY track_no, title LIMIT 1`,
		albumID).Scan(&id)
	if err != nil {
		return 0, false
	}
	return id, true
}
```

- [ ] **Step 2: Write the failing test**

`internal/api/cover_test.go`:

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

func TestTrackCoverMissingArtIs404(t *testing.T) {
	srv := newTestServer(t)
	// Track whose file has no embedded art (path need not exist for the 404 path:
	// the handler must 404 when no picture is found).
	id, _ := store.UpsertTrack(srv.db, model.Track{Path: "/no/such/file.mp3", Title: "X"}, "A", "B")

	req := httptest.NewRequest(http.MethodGet, "/api/tracks/"+itoa(id)+"/cover", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 when no cover, got %d", rec.Code)
	}
}

func TestTrackCoverUnknownIdIs404(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/tracks/9999/cover", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 for unknown track, got %d", rec.Code)
	}
}
```

Add a tiny helper at the bottom of `cover_test.go` if `itoa` is not already shared in the package's test files — it is defined in `server_test.go` only if present; to avoid duplicate-symbol errors, use `strconv.FormatInt` inline instead:

```go
// (use strconv.FormatInt(id, 10) inline; do not redeclare itoa)
```

Rewrite the two `"/api/tracks/"+itoa(id)+"/cover"` usages to `"/api/tracks/"+strconv.FormatInt(id,10)+"/cover"` and import `"strconv"`.

- [ ] **Step 2b: Run, expect failure**

Run: `mise exec -- go test ./internal/api/ -run TestTrackCover`
Expected: FAIL — route not registered / handler undefined.

- [ ] **Step 3: Implement the handler**

`internal/api/cover.go`:

```go
package api

import (
	"bytes"
	"net/http"
	"os"
	"strconv"

	"github.com/dhowden/tag"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

// trackCover serves a track's embedded cover image, or 404 if none.
func (s *Server) trackCover(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	serveCover(w, s, id)
}

// albumCover serves the cover of an album's first track.
func (s *Server) albumCover(w http.ResponseWriter, r *http.Request) {
	albumID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	trackID, ok := store.FirstTrackIDOfAlbum(s.db, albumID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	serveCover(w, s, trackID)
}

func serveCover(w http.ResponseWriter, s *Server, trackID int64) {
	_, path, ok := store.GetTrack(s.db, trackID)
	if !ok {
		http.NotFound(w, nil)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	defer f.Close()
	m, err := tag.ReadFrom(f)
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	pic := m.Picture()
	if pic == nil || len(pic.Data) == 0 {
		http.NotFound(w, nil)
		return
	}
	w.Header().Set("Content-Type", pic.MIMEType)
	w.Header().Set("Cache-Control", "max-age=86400")
	http.ServeContent(w, &http.Request{}, "", timeZero(), bytes.NewReader(pic.Data))
}
```

> Note: `http.NotFound(w, nil)` and the `http.ServeContent(w, &http.Request{}, ...)` calls pass a throwaway request because the helper doesn't need the original. If `go vet` or runtime complains about a nil request in `http.NotFound`, replace those calls with `w.WriteHeader(http.StatusNotFound)` and a direct `w.Write(pic.Data)` after setting the header (simpler and avoids the synthetic request). Implement it that simpler way:

Replace `serveCover`'s not-found and serve lines accordingly — final form:

```go
func serveCover(w http.ResponseWriter, s *Server, trackID int64) {
	_, path, ok := store.GetTrack(s.db, trackID)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	defer f.Close()
	m, err := tag.ReadFrom(f)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	pic := m.Picture()
	if pic == nil || len(pic.Data) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", pic.MIMEType)
	w.Header().Set("Cache-Control", "max-age=86400")
	w.Write(pic.Data)
}
```

Delete the unused `bytes`, and `timeZero` references and the first `serveCover` draft; keep only the simpler final version. Final imports for `cover.go`: `net/http`, `os`, `strconv`, `github.com/dhowden/tag`, `github.com/andybarilla/exit66jukebox/internal/store`.

- [ ] **Step 4: Register the routes**

In `internal/api/server.go` `Handler()`, add before the `if s.ui != nil` block:

```go
	mux.HandleFunc("GET /api/tracks/{id}/cover", s.trackCover)
	mux.HandleFunc("GET /api/albums/{id}/cover", s.albumCover)
```

- [ ] **Step 5: Run, expect pass**

Run: `mise exec -- go test ./internal/api/ -run TestTrackCover`
Expected: PASS (both 404 cases)

- [ ] **Step 6: Commit**

```bash
git add internal/api/cover.go internal/api/cover_test.go internal/api/server.go internal/store/library.go
git commit -m "feat: cover-art endpoints (embedded tag art, 404 fallback)"
```

---

## Phase 2 — Event bus

### Task 2.1: Per-stream publish/subscribe bus

**Files:** Create `internal/events/bus.go`, `internal/events/bus_test.go`.

- [ ] **Step 1: Write the failing test**

`internal/events/bus_test.go`:

```go
package events

import "testing"

func TestBusDeliversToSubscriber(t *testing.T) {
	b := NewBus()
	ch, cancel := b.Subscribe()
	defer cancel()

	b.Publish(Event{Type: "now-playing", Data: "song"})

	select {
	case e := <-ch:
		if e.Type != "now-playing" {
			t.Fatalf("want now-playing, got %q", e.Type)
		}
	default:
		t.Fatal("expected an event, got none")
	}
}

func TestBusCancelStopsDelivery(t *testing.T) {
	b := NewBus()
	ch, cancel := b.Subscribe()
	cancel()
	b.Publish(Event{Type: "x"})
	if _, open := <-ch; open {
		t.Fatal("channel should be closed after cancel")
	}
}

func TestBusDropsWhenSubscriberFull(t *testing.T) {
	b := NewBus()
	_, cancel := b.Subscribe()
	defer cancel()
	// Publishing many events without draining must not block/panic.
	for i := 0; i < 1000; i++ {
		b.Publish(Event{Type: "spam"})
	}
}
```

- [ ] **Step 2: Run, expect failure**

Run: `mise exec -- go test ./internal/events/`
Expected: FAIL — `undefined: NewBus`

- [ ] **Step 3: Implement**

`internal/events/bus.go`:

```go
package events

import "sync"

// Event is a single SSE message: a type and an arbitrary JSON-serializable body.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// Bus is a per-stream fan-out of events to SSE subscribers. Non-blocking:
// a subscriber that can't keep up drops events rather than stalling publishers.
type Bus struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

func NewBus() *Bus {
	return &Bus{subs: make(map[chan Event]struct{})}
}

// Subscribe returns a buffered event channel and a cancel func that unsubscribes
// and closes the channel. Always call cancel (e.g. defer) when done.
func (b *Bus) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 16)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subs, ch)
			b.mu.Unlock()
			close(ch)
		})
	}
	return ch, cancel
}

// Publish delivers an event to all current subscribers, dropping it for any
// subscriber whose buffer is full.
func (b *Bus) Publish(e Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- e:
		default: // subscriber is behind; drop
		}
	}
}
```

- [ ] **Step 4: Run, expect pass**

Run: `mise exec -- go test -race ./internal/events/`
Expected: PASS, no races

- [ ] **Step 5: Commit**

```bash
git add internal/events/
git commit -m "feat: per-stream SSE event bus (non-blocking fan-out)"
```

---

## Phase 3 — Broadcast hub

### Task 3.1: Hub with injectable source (fan-out, advance, idle silence)

**Files:** Create `internal/broadcast/hub.go`, `internal/broadcast/hub_test.go`.

- [ ] **Step 1: Write the failing test**

`internal/broadcast/hub_test.go`:

```go
package broadcast

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"
)

// fakeSource serves canned bytes per "path".
type fakeSource struct{ data map[string][]byte }

func (f fakeSource) Open(path string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(f.data[path])), nil
}

// collect reads up to n bytes from a listener channel within a deadline.
func collect(ch <-chan []byte, want int, deadline time.Duration) []byte {
	var out []byte
	timer := time.After(deadline)
	for len(out) < want {
		select {
		case b, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, b...)
		case <-timer:
			return out
		}
	}
	return out
}

func TestHubFansOutQueuedTracksInOrder(t *testing.T) {
	src := fakeSource{data: map[string][]byte{
		"A": []byte("aaaa"),
		"B": []byte("bbbb"),
	}}
	queue := []string{"A", "B"}
	next := func() (string, bool) {
		if len(queue) == 0 {
			return "", false
		}
		p := queue[0]
		queue = queue[1:]
		return p, true
	}

	h := NewHub(src, next, []byte("S"))
	h.idlePace = 5 * time.Millisecond

	ch, cancel := h.Listen()
	defer cancel()

	ctx, stop := context.WithCancel(context.Background())
	go h.Run(ctx)
	defer stop()

	got := collect(ch, 8, time.Second)
	if !bytes.Contains(got, []byte("aaaa")) || !bytes.Contains(got, []byte("bbbb")) {
		t.Fatalf("expected both tracks' bytes, got %q", got)
	}
	ai := bytes.Index(got, []byte("aaaa"))
	bi := bytes.Index(got, []byte("bbbb"))
	if ai > bi {
		t.Fatalf("expected A before B, got %q", got)
	}
}

func TestHubStreamsSilenceWhenEmpty(t *testing.T) {
	src := fakeSource{data: map[string][]byte{}}
	next := func() (string, bool) { return "", false }

	h := NewHub(src, next, []byte("S"))
	h.idlePace = time.Millisecond

	ch, cancel := h.Listen()
	defer cancel()
	ctx, stop := context.WithCancel(context.Background())
	go h.Run(ctx)
	defer stop()

	got := collect(ch, 3, time.Second)
	if !bytes.Contains(got, []byte("S")) {
		t.Fatalf("expected silence bytes when empty, got %q", got)
	}
}
```

- [ ] **Step 2: Run, expect failure**

Run: `mise exec -- go test ./internal/broadcast/`
Expected: FAIL — `undefined: NewHub`

- [ ] **Step 3: Implement**

`internal/broadcast/hub.go`:

```go
package broadcast

import (
	"context"
	"io"
	"sync"
	"time"
)

// Source opens a real-time-paced MP3 byte stream for a track path.
type Source interface {
	Open(path string) (io.ReadCloser, error)
}

// Hub fans one shared MP3 feed out to many HTTP listeners. It pulls tracks via
// next(); when the queue is empty it emits silence so listeners stay connected.
type Hub struct {
	src      Source
	next     func() (path string, ok bool)
	silence  []byte
	idlePace time.Duration

	mu        sync.Mutex
	listeners map[chan []byte]struct{}
}

func NewHub(src Source, next func() (string, bool), silence []byte) *Hub {
	return &Hub{
		src:       src,
		next:      next,
		silence:   silence,
		idlePace:  time.Second,
		listeners: make(map[chan []byte]struct{}),
	}
}

// Listen registers a listener, returning its byte channel and an unsubscribe
// func. The channel is buffered; a listener that falls behind drops chunks.
func (h *Hub) Listen() (<-chan []byte, func()) {
	ch := make(chan []byte, 64)
	h.mu.Lock()
	h.listeners[ch] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.listeners, ch)
			h.mu.Unlock()
			close(ch)
		})
	}
	return ch, cancel
}

func (h *Hub) broadcast(b []byte) {
	// Copy: the caller reuses its read buffer.
	chunk := make([]byte, len(b))
	copy(chunk, b)
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.listeners {
		select {
		case ch <- chunk:
		default: // listener behind; drop
		}
	}
}

// Run is the broadcast loop. It blocks until ctx is cancelled.
func (h *Hub) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		path, ok := h.next()
		if !ok {
			h.idle(ctx)
			continue
		}
		h.play(ctx, path)
	}
}

func (h *Hub) play(ctx context.Context, path string) {
	rc, err := h.src.Open(path)
	if err != nil {
		return
	}
	defer rc.Close()
	buf := make([]byte, 32*1024)
	for {
		if ctx.Err() != nil {
			return
		}
		n, err := rc.Read(buf)
		if n > 0 {
			h.broadcast(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

func (h *Hub) idle(ctx context.Context) {
	if len(h.silence) > 0 {
		h.broadcast(h.silence)
	}
	select {
	case <-ctx.Done():
	case <-time.After(h.idlePace):
	}
}
```

- [ ] **Step 4: Run, expect pass**

Run: `mise exec -- go test -race ./internal/broadcast/`
Expected: PASS, no races

- [ ] **Step 5: Commit**

```bash
git add internal/broadcast/hub.go internal/broadcast/hub_test.go
git commit -m "feat: broadcast hub — fan-out, queue advance, idle silence"
```

### Task 3.2: Real ffmpeg source + silence generator

**Files:** Create `internal/broadcast/source.go`. (No unit test — exercised via the manual stream smoke test in Phase 4. Build-only verification here.)

- [ ] **Step 1: Implement**

`internal/broadcast/source.go`:

```go
package broadcast

import (
	"io"
	"os/exec"
	"strconv"
)

// FFmpegSource transcodes any audio file to a real-time-paced MP3 byte stream
// using ffmpeg's -re flag (read input at native rate), so the shared feed
// advances in real time and late joiners hear the current position.
type FFmpegSource struct{}

func (FFmpegSource) Open(path string) (io.ReadCloser, error) {
	cmd := exec.Command("ffmpeg",
		"-re", "-i", path,
		"-vn", "-f", "mp3", "-b:a", "192k", "-")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &procReadCloser{cmd: cmd, r: stdout}, nil
}

type procReadCloser struct {
	cmd *exec.Cmd
	r   io.ReadCloser
}

func (p *procReadCloser) Read(b []byte) (int, error) { return p.r.Read(b) }

func (p *procReadCloser) Close() error {
	_ = p.cmd.Process.Kill()
	return p.cmd.Wait()
}

// GenerateSilence renders `seconds` of MP3 silence via ffmpeg, used by the hub
// to keep listeners connected when the queue is empty. Returns nil on failure;
// the hub treats empty silence as "send nothing".
func GenerateSilence(seconds int) []byte {
	out, err := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo",
		"-t", strconv.Itoa(seconds), "-f", "mp3", "-b:a", "192k", "-").Output()
	if err != nil {
		return nil
	}
	return out
}
```

- [ ] **Step 2: Verify build**

Run: `mise exec -- go build ./...`
Expected: clean

- [ ] **Step 3: Commit**

```bash
git add internal/broadcast/source.go
git commit -m "feat: ffmpeg real-time source + silence generator"
```

---

## Phase 4 — Wire the house stream (HTTP stream + SSE + events)

### Task 4.1: Stream registry, /stream/{id}.mp3, and SSE endpoint

**Files:** Create `internal/api/stream.go`; modify `internal/api/server.go` (registry fields + routes); modify `internal/api/streams.go` (publish queue-changed); add `internal/api/stream_test.go`.

- [ ] **Step 1: Add registry fields + registration + routes**

In `internal/api/server.go`, add imports `"github.com/andybarilla/exit66jukebox/internal/broadcast"` and `"github.com/andybarilla/exit66jukebox/internal/events"`. Add fields to `Server`:

```go
type Server struct {
	db    *sql.DB
	jb    *jukebox.Jukebox
	ui    fs.FS
	hubs  map[string]*broadcast.Hub
	buses map[string]*events.Bus
}
```

Update `NewServer` to initialize the maps (signature unchanged):

```go
func NewServer(db *sql.DB, jb *jukebox.Jukebox, ui fs.FS) *Server {
	return &Server{
		db: db, jb: jb, ui: ui,
		hubs:  make(map[string]*broadcast.Hub),
		buses: make(map[string]*events.Bus),
	}
}

// RegisterStream attaches a broadcast hub and event bus for a shared stream id.
func (s *Server) RegisterStream(id string, hub *broadcast.Hub, bus *events.Bus) {
	s.hubs[id] = hub
	s.buses[id] = bus
}
```

In `Handler()`, add (before `if s.ui != nil`):

```go
	mux.HandleFunc("GET /stream/{id}.mp3", s.streamAudio)
	mux.HandleFunc("GET /api/streams/{id}/events", s.streamEvents)
```

- [ ] **Step 2: Write the failing test**

`internal/api/stream_test.go`:

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStreamAudioUnknownStreamIs404(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/stream/nope.mp3", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 for unknown stream, got %d", rec.Code)
	}
}

func TestEventsUnknownStreamIs404(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/streams/nope/events", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 for unknown stream events, got %d", rec.Code)
	}
}
```

- [ ] **Step 3: Run, expect failure**

Run: `mise exec -- go test ./internal/api/ -run 'TestStreamAudio|TestEvents'`
Expected: FAIL — handlers undefined.

- [ ] **Step 4: Implement the handlers**

`internal/api/stream.go`:

```go
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// streamAudio fans the shared stream's continuous MP3 feed to this listener.
func (s *Server) streamAudio(w http.ResponseWriter, r *http.Request) {
	hub, ok := s.hubs[r.PathValue("id")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch, cancel := hub.Listen()
	defer cancel()
	for {
		select {
		case <-r.Context().Done():
			return
		case chunk, open := <-ch:
			if !open {
				return
			}
			if _, err := w.Write(chunk); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// streamEvents is an SSE endpoint pushing now-playing/queue-changed events.
func (s *Server) streamEvents(w http.ResponseWriter, r *http.Request) {
	bus, ok := s.buses[r.PathValue("id")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch, cancel := bus.Subscribe()
	defer cancel()
	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-ch:
			data, _ := json.Marshal(e)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}
```

- [ ] **Step 5: Publish queue-changed on mutations**

In `internal/api/streams.go`, add a helper and call it after successful request/remove/clear. Add at the bottom of the file:

```go
func (s *Server) publishQueueChanged(streamID string) {
	if bus, ok := s.buses[streamID]; ok {
		bus.Publish(events.Event{Type: "queue-changed", Data: streamID})
	}
}
```

Add import `"github.com/andybarilla/exit66jukebox/internal/events"` to `streams.go`. Then call `s.publishQueueChanged(id)` at the end of the success paths in `request` (in each of the album/artist/track branches after writing the response — or once before the switch's writes; simplest: call it right after `r.ParseForm()`/validation passes and before the switch is not correct since nothing queued yet — instead call it inside each branch after the queue mutation). Concretely: in `request`, after computing `n` / `res` and before/after `writeJSON`, call `s.publishQueueChanged(id)`. In `removeRequest` and `clearRequests`, call `s.publishQueueChanged(id)` after the successful `Remove`/`Clear` and before `writeJSON`.

Place the call so it runs only on success (not on the 400/500 error returns).

- [ ] **Step 6: Run, expect pass**

Run: `mise exec -- go test -race ./internal/api/`
Expected: PASS (all api tests, including the new 404 cases)

- [ ] **Step 7: Commit**

```bash
git add internal/api/stream.go internal/api/server.go internal/api/streams.go internal/api/stream_test.go
git commit -m "feat: /stream/{id}.mp3 fan-out, SSE events, queue-changed publishing"
```

### Task 4.2: Create and run the house stream in main

**Files:** modify `main.go`.

- [ ] **Step 1: Wire the house stream**

In `main.go`, after `jb := jukebox.New(...)` and after the scan goroutine, before building `srv`, add the house stream wiring. Add imports `"context"`, `"github.com/andybarilla/exit66jukebox/internal/broadcast"`, `"github.com/andybarilla/exit66jukebox/internal/events"`. Insert:

```go
	const houseID = "house"
	jb.EnsureStream(houseID, "shared")
	houseBus := events.NewBus()
	silence := broadcast.GenerateSilence(1)

	// next pops the house queue and publishes now-playing. Returns the file path
	// for the broadcaster. Called repeatedly; when empty it has no side effects.
	next := func() (string, bool) {
		tr, ok := jb.Next(houseID)
		if !ok {
			return "", false
		}
		_, path, found := store.GetTrack(db, tr.ID)
		if !found {
			return "", false
		}
		houseBus.Publish(events.Event{Type: "now-playing", Data: tr})
		return path, true
	}

	houseHub := broadcast.NewHub(broadcast.FFmpegSource{}, next, silence)
	go houseHub.Run(context.Background())
```

Then change the server construction block to register the stream:

```go
	srv := api.NewServer(db, jb, uiFS)
	srv.RegisterStream(houseID, houseHub, houseBus)
```

- [ ] **Step 2: Build + vet**

Run: `mise exec -- go build ./...` and `mise exec -- go vet ./...`
Expected: clean

- [ ] **Step 3: Manual stream smoke test** (requires ffmpeg; uses the 1s fixture)

```bash
mise exec -- go build -o exit66jukebox .
rm -f /tmp/e66stream.db
./exit66jukebox -root internal/scan/testdata -db /tmp/e66stream.db -addr :8095 &
SRV=$!; sleep 2
# queue the fixture track into the house stream, then pull ~2s of the live MP3 feed
curl -s -X POST localhost:8095/api/streams/house/requests -d 'kind=track&id=1' ; echo
timeout 3 curl -s localhost:8095/stream/house.mp3 -o /tmp/house.mp3
echo "stream bytes: $(wc -c < /tmp/house.mp3)"
file /tmp/house.mp3
kill $SRV
```
Expected: the request returns `{"queued":1,...}`; `/tmp/house.mp3` is non-empty and `file` reports MPEG/MP3 audio (silence + the fixture). A non-zero byte count confirms the fan-out works.

- [ ] **Step 4: Commit**

```bash
git add main.go
git commit -m "feat: run the always-on house shared stream"
```

---

## Phase 5 — UI: Local/House toggle, SSE, cover art

### Task 5.1: API client additions

**Files:** modify `web/src/lib/api.js`.

- [ ] **Step 1: Add house + SSE + cover helpers**

Append to `web/src/lib/api.js`:

```js
export const HOUSE = 'house';

export async function requestTo(streamId, trackId) {
  const body = new URLSearchParams({ kind: 'track', id: String(trackId) });
  const r = await fetch(`/api/streams/${streamId}/requests`, { method: 'POST', body });
  return r.json();
}

export async function getQueue(streamId) {
  const r = await fetch(`/api/streams/${streamId}`);
  return r.json(); // { id, queue: [...] }
}

export function houseStreamURL() {
  return `/stream/${HOUSE}.mp3`;
}

export function coverURL(trackId) {
  return `/api/tracks/${trackId}/cover`;
}

// subscribeEvents opens an SSE connection; onEvent receives parsed {type,data}.
// Returns a close function.
export function subscribeEvents(streamId, onEvent) {
  const es = new EventSource(`/api/streams/${streamId}/events`);
  es.onmessage = (m) => {
    try { onEvent(JSON.parse(m.data)); } catch (_) {}
  };
  return () => es.close();
}
```

- [ ] **Step 2: Build check (UI compiles in build step later) + commit**

```bash
git add web/src/lib/api.js
git commit -m "feat(ui): api client for house stream, SSE, cover art"
```

### Task 5.2: Dual-mode App with Local/House toggle

**Files:** overwrite `web/src/App.svelte`; modify `web/src/lib/player.js` (local mode unchanged, just ensure exported).

- [ ] **Step 1: Overwrite `web/src/App.svelte`**

```svelte
<script>
  import { onMount, onDestroy } from 'svelte';
  import {
    listTracks, requestTrack, requestTo, getQueue,
    houseStreamURL, coverURL, subscribeEvents, HOUSE,
  } from './lib/api.js';
  import { createPlayer } from './lib/player.js';

  let mode = 'local';        // 'local' | 'house'
  let search = '';
  let tracks = [];
  let nowPlaying = null;
  let houseQueue = [];
  let audio;
  let localPlayer = null;
  let closeEvents = null;

  async function doSearch() { tracks = await listTracks(search); }

  async function add(t) {
    if (mode === 'house') await requestTo(HOUSE, t.id);
    else await requestTrack(t.id);
  }

  function startLocal() {
    if (closeEvents) { closeEvents(); closeEvents = null; }
    nowPlaying = null;
    audio.src = '';
    localPlayer = createPlayer(audio, (t) => (nowPlaying = t));
    localPlayer.start();
  }

  async function startHouse() {
    nowPlaying = null;
    audio.src = houseStreamURL();
    audio.play().catch(() => {});
    const q = await getQueue(HOUSE);
    houseQueue = q.queue || [];
    closeEvents = subscribeEvents(HOUSE, (e) => {
      if (e.type === 'now-playing') nowPlaying = e.data;
      else if (e.type === 'queue-changed') getQueue(HOUSE).then((q) => (houseQueue = q.queue || []));
    });
  }

  function setMode(m) {
    if (m === mode) return;
    mode = m;
    if (m === 'house') startHouse();
    else startLocal();
  }

  onMount(async () => {
    tracks = await listTracks('');
    startLocal();
  });
  onDestroy(() => { if (closeEvents) closeEvents(); });
</script>

<main>
  <h1>Exit 66 Jukebox</h1>

  <div class="modes">
    <button class:active={mode === 'local'} on:click={() => setMode('local')}>Local</button>
    <button class:active={mode === 'house'} on:click={() => setMode('house')}>House</button>
  </div>

  <div class="now">
    {#if nowPlaying}
      <img class="cover" src={coverURL(nowPlaying.id)} alt="" on:error={(e) => (e.target.style.visibility = 'hidden')} />
      <span>Now playing: {nowPlaying.title}</span>
    {:else}
      <span>{mode === 'house' ? 'House stream idle — request something' : 'Queue empty — request something'}</span>
    {/if}
  </div>

  <input bind:value={search} on:keydown={(e) => e.key === 'Enter' && doSearch()} placeholder="Search songs" />
  <button on:click={doSearch}>Search</button>

  <ul>
    {#each tracks as t (t.id)}
      <li><button on:click={() => add(t)}>＋</button> {t.title}</li>
    {/each}
  </ul>

  {#if mode === 'house' && houseQueue.length}
    <h2>Up next (house)</h2>
    <ol>
      {#each houseQueue as t (t.id)}<li>{t.title}</li>{/each}
    </ol>
  {/if}

  <audio bind:this={audio} controls></audio>
</main>

<style>
  main { font-family: system-ui; max-width: 640px; margin: 2rem auto; color: #eee; background: #181818; padding: 1.5rem; border-radius: 8px; }
  .modes button { margin-right: .5rem; }
  .modes button.active { background: #6cf; color: #000; }
  .now { color: #6cf; display: flex; align-items: center; gap: .5rem; min-height: 40px; }
  .cover { width: 40px; height: 40px; object-fit: cover; border-radius: 4px; }
  li { list-style: none; margin: .25rem 0; }
  ol li { list-style: decimal; margin-left: 1.5rem; }
  button { cursor: pointer; }
</style>
```

- [ ] **Step 2: Rebuild the embedded UI**

```bash
cd web && mise exec -- npm run build && cd ..
```

- [ ] **Step 3: Full build + tests**

Run: `mise exec -- go build ./...` and `mise exec -- go test ./...`
Expected: clean / all pass.

- [ ] **Step 4: Manual end-to-end** (two browser tabs)

```bash
mise exec -- go build -o exit66jukebox .
./exit66jukebox -root /path/to/music -db /tmp/e66ui2.db &
```
Open the UI in two tabs, switch both to **House**, request a song in one tab, confirm: both tabs’ "Now playing" updates via SSE, both `<audio>` elements play the same feed, and the "Up next (house)" list updates. Switch one tab to **Local** and confirm it plays its own independent queue. Kill the server when done.

- [ ] **Step 5: Commit (source + rebuilt dist)**

```bash
git add web/src/App.svelte internal/web/dist
git commit -m "feat(ui): Local/House toggle, SSE now-playing + queue, cover art"
```

---

## Self-Review notes (addressed)

- **Spec coverage:** shared stream as continuous server MP3 feed (Phases 3–4); `/stream/{id}.mp3` (Task 4.1); late-joiners + synchronization via `ffmpeg -re` real-time pacing (Task 3.2, design note); SSE replacing polling (Task 4.1); cover art (Task 1.2); duration (Task 1.1); Local/House selectable output with the queue belonging to the stream (Task 5.2 — Local uses the private per-session queue, House uses the shared `house` queue). Sonos/discovery/integrations/named-multi-streams remain out of scope (future plans).
- **No signature churn:** `NewServer(db, jb, ui)` is unchanged; the stream registry is attached via `RegisterStream`, so Plan 1's API tests keep compiling. New routes 404 cleanly when no stream is registered (covered by tests).
- **Type/contract consistency:** SSE `now-playing` `Data` is a `model.Track` (same JSON the browse/queue endpoints return, so the UI reads `.id`/`.title` uniformly); `queue-changed` carries the stream id. `coverURL(track.id)` matches `GET /api/tracks/{id}/cover`.
- **Testable seam:** the hub is unit-tested with a fake `Source` and a fake `next`; the real ffmpeg path and the live feed are covered by the Phase 4 manual smoke test (a non-zero MP3 byte count over HTTP), since spawning ffmpeg in unit tests is undesirable.
- **Known limitation carried forward:** no graceful shutdown of the hub goroutine (process-lifetime, as in Plan 1); a `context.Background()` is passed so a future signal-based shutdown can swap it in.

## Definition of done for Plan 2

`go test ./...` passes (race-clean); `go build` is clean; the running binary serves a live `GET /stream/house.mp3` MP3 feed fed by the shared queue with real-time pacing; two browsers in House mode hear the same feed and see now-playing + queue update live over SSE; Local mode still plays an independent per-device queue; tracks have durations and cover art is served when embedded. Sonos casting, discovery, and external-service integration remain for later plans.
