# MusicBrainz + Cover Art Archive Enrichment — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enrich poorly-tagged tracks with canonical MusicBrainz IDs (MBIDs) and
backfill placeholder tags; fall back to the Cover Art Archive for albums with no
embedded cover. Read-only external calls, no auth, ≤1 req/sec.

**Architecture:** A new `internal/external` package holds a hand-rolled,
rate-limited HTTP client and a thin `musicbrainz` client. MBIDs are stored in
new nullable `mbid` columns on `artist`/`album`/`track` (guarded migration, per
the `#19` pattern). An enrichment pass — triggered by `POST /api/enrich` and run
in a background goroutine like the startup scan — walks tracks with no MBID,
matches each against MusicBrainz by artist/title/album text search, and on a
high-confidence match records the MBIDs, replaces placeholder names, and (if the
album has no cover) downloads the CAA front image to a local covers cache.
`serveCover` gains a fallback to that cached file.

**Tech Stack:** Go 1.26 stdlib only (no new dependencies — limiter and clients
are hand-rolled); existing `internal/store`, `internal/api`, `internal/scan`
packages.

**Spec:** `docs/superpowers/specs/2026-06-12-external-service-integration-design.md`
(epic architecture — this is child #38).

---

## Design decisions (enrichment-specific; the epic spec owns the shared infra)

- **Trigger = API endpoint.** `POST /api/enrich` starts a single background pass
  and returns its status (202-style). A second POST while running returns the
  in-progress status instead of starting a duplicate. `GET /api/enrich` reports
  current status + progress counts. Chosen over a CLI subcommand because
  `main.go` has no subcommand dispatch and the server-centric design lets #41
  trigger it from the UI later.
- **What gets enriched:** tracks with `track.mbid = ''`. This makes the pass
  **resumable** — a multi-hour ≤1 req/sec run can stop and restart, re-processing
  only un-matched tracks. A track that matched keeps its MBID and is skipped.
- **Never clobber good data.** On a match, fill `artist.mbid`/`album.mbid`/
  `track.mbid` always; replace a name only when it is a placeholder
  (`Unknown Artist`, `Unknown Album`) or `genre`/`title` is blank/equals the
  filename. Real existing tags are left untouched.
- **Matching heuristic:** MusicBrainz recording search with the non-placeholder
  fields (`recording:"<title>" AND artist:"<artist>" AND release:"<album>"`).
  MusicBrainz returns a `score` (0–100); accept the top hit only at `score >= 90`
  (`matchThreshold` constant). Below threshold → leave the track un-matched
  (retried on a future pass). **AcoustID fingerprinting is out of scope** — it
  needs the external `fpcalc` binary; note as future work.
- **Cover storage:** when a matched album has a release MBID and `album.cover`
  is empty, GET `https://coverartarchive.org/release/{mbid}/front`, follow the
  redirect, download the image to `<db-dir>/covers/<album-id>.<ext>`, and store
  that path in `album.cover`. `serveCover` falls back to this file when no
  embedded art exists. The covers dir is derived from the DB path (no new flag).
- **No new dependency.** The rate limiter (mutex + next-allowed timestamp) and
  the JSON clients are hand-rolled per the user's "ask before adding deps" rule.

---

## File Structure

- `internal/store/schema.sql` (modify) — add `mbid` to `artist`, `album`, `track`.
- `internal/store/migrate.go` (modify) — guarded `mbid` column adds.
- `internal/external/client.go` (create) — rate-limited HTTP `Client` + limiter.
- `internal/external/musicbrainz.go` (create) — `SearchRecording`, response types.
- `internal/external/coverart.go` (create) — `FetchFrontCover`.
- `internal/external/client_test.go`, `musicbrainz_test.go`, `coverart_test.go` (create).
- `internal/store/enrich.go` (create) — `TracksNeedingEnrichment`, `ApplyEnrichment`, `AlbumCoverByTrack`.
- `internal/store/enrich_test.go` (create).
- `internal/enrich/enrich.go` (create) — the pass: orchestration, status, covers dir.
- `internal/enrich/enrich_test.go` (create).
- `internal/api/enrich.go` (create) — `POST`/`GET /api/enrich` handlers.
- `internal/api/server.go` (modify) — register routes; hold the enrich runner.
- `internal/api/cover.go` (modify) — `serveCover` cached-cover fallback.
- `main.go` (modify) — wire the enrich runner into the server.

---

## Task 1: MBID schema + migration

**Files:** modify `internal/store/schema.sql`, `internal/store/migrate.go`

- [ ] **Step 1:** In `schema.sql`, add `mbid TEXT NOT NULL DEFAULT ''` to the
  `artist`, `album`, and `track` CREATE statements.

- [ ] **Step 2:** In `migrate.go`, after the `added_at` block, add guarded
  column adds (mirrors the existing `columnExists` pattern):

  ```go
  for _, t := range []string{"artist", "album", "track"} {
      has, err := columnExists(db, t, "mbid")
      if err != nil {
          return err
      }
      if !has {
          if _, err := db.Exec("ALTER TABLE " + t + " ADD COLUMN mbid TEXT NOT NULL DEFAULT ''"); err != nil {
              return err
          }
      }
  }
  ```

  (Table names are a fixed local list, not user input — safe to concatenate;
  `ALTER TABLE` cannot use `?` placeholders for identifiers.)

- [ ] **Step 3:** Add `Mbid` to relevant structs in `internal/model/model.go`
  (`Artist`, `Album`, `Track`) as `Mbid string `json:"-"`` (internal only).

- [ ] **Step 4:** `go build ./...`; add a `migrate_test.go` case: open a DB with
  pre-existing tables lacking `mbid`, assert columns added and re-running `Open`
  is a no-op. `go test ./internal/store/...`.

- [ ] **Step 5:** Commit: `feat: add mbid columns + migration`.

---

## Task 2: Rate-limited HTTP client (`internal/external`)

**Files:** create `internal/external/client.go`, `client_test.go`

- [ ] **Step 1:** `Client` wraps `*http.Client` + a `limiter` + `userAgent`.
  Constructor `New(userAgent string, minInterval time.Duration) *Client`.

- [ ] **Step 2:** Limiter = mutex guarding a `nextAllowed time.Time`; `wait()`
  sleeps until `nextAllowed`, then sets `nextAllowed = now + minInterval`. Inject
  a `now func() time.Time` (default `time.Now`) so tests use a fake clock.

- [ ] **Step 3:** `getJSON(ctx, url, v any) error`: `wait()`, set `User-Agent`,
  do request; on `429`/`5xx` retry up to 3 times honoring `Retry-After`
  (fall back to exponential backoff); decode JSON into `v`. `do(ctx, url)
  (*http.Response, error)` for raw bytes (cover download).

- [ ] **Step 4:** Tests against `httptest.Server`: success decode; a `429` then
  `200` retries and succeeds; the limiter enforces spacing (fake clock asserts
  `wait()` advances by `minInterval`). `go test ./internal/external/...`.

- [ ] **Step 5:** Commit: `feat: rate-limited external HTTP client`.

---

## Task 3: MusicBrainz + CAA clients

**Files:** create `internal/external/musicbrainz.go`, `coverart.go`, + tests

- [ ] **Step 1:** `musicbrainz.go`: `MB struct { c *Client }`,
  `NewMusicBrainz(c *Client) *MB`. `SearchRecording(ctx, artist, title, album
  string) (RecordingMatch, bool, error)` builds the Lucene query (omit empty /
  placeholder terms), GETs
  `https://musicbrainz.org/ws/2/recording?query=<q>&fmt=json&limit=1`, returns
  the top recording with `Score`, recording MBID, artist-credit MBID + name,
  and first release MBID + title. `bool` = whether a hit existed (caller applies
  the score threshold).

- [ ] **Step 2:** Define response structs for the subset used (recordings[],
  score, id, title, artist-credit[].artist.{id,name}, releases[].{id,title}).

- [ ] **Step 3:** `coverart.go`: `FetchFrontCover(ctx, releaseMBID string)
  (data []byte, contentType string, ok bool, err error)` — GET
  `https://coverartarchive.org/release/{mbid}/front`; `404` → `ok=false` (no art,
  not an error); follow redirects (default client behavior); cap body size.

- [ ] **Step 4:** Tests with `httptest` + injected base URLs (make the base URL a
  field so tests point at the test server): parse a canned MB JSON response into
  a `RecordingMatch`; query-building omits placeholders; CAA `404` → `ok=false`,
  `200` → bytes + content-type.

- [ ] **Step 5:** Commit: `feat: musicbrainz + cover art archive clients`.

---

## Task 4: Store enrichment helpers

**Files:** create `internal/store/enrich.go`, `enrich_test.go`

- [ ] **Step 1:** `TracksNeedingEnrichment(db) ([]EnrichTarget, error)` —
  `SELECT t.id, t.title, t.genre, t.path, ar.name, ar.id, al.name, al.id
   FROM track t JOIN artist ar ON ar.id=t.artist_id JOIN album al ON al.id=t.album_id
   WHERE t.mbid = ''`. `EnrichTarget` carries ids + current names so the pass can
  decide placeholder replacement without re-querying.

- [ ] **Step 2:** `ApplyEnrichment(db, e Enrichment) error` in one transaction:
  set `track.mbid`; set `artist.mbid`/`album.mbid` when currently empty; replace
  `artist.name` only if it was `Unknown Artist` (and the new name is non-empty,
  guarding the `UNIQUE(name)` constraint — `INSERT OR IGNORE`-merge if the target
  name already exists is **out of scope**; on a unique collision, keep the MBID
  and skip the rename); same for `album.name` vs `Unknown Album`; replace blank
  `genre`/filename-`title`.

- [ ] **Step 3:** `SetAlbumCover(db, albumID int64, path string) error` and
  `AlbumCoverByTrack(db, trackID int64) (path string, ok bool)` (joins track→album,
  returns `album.cover`, `ok=false` if empty).

- [ ] **Step 4:** Tests: target query returns only `mbid=''` rows; `ApplyEnrichment`
  fills MBIDs, replaces placeholders but not real names, is idempotent; unique-name
  collision keeps MBID without erroring. `go test ./internal/store/...`.

- [ ] **Step 5:** Commit: `feat: store enrichment helpers`.

---

## Task 5: The enrichment pass (`internal/enrich`)

**Files:** create `internal/enrich/enrich.go`, `enrich_test.go`

- [ ] **Step 1:** `Runner` holds `db`, the MB + CAA clients, `coversDir`, and
  guarded status (`running bool`, `Processed/Matched/Covers/Failed` counts,
  `mu sync.Mutex`). `NewRunner(db, mb, ca, coversDir)`.

- [ ] **Step 2:** `Start(ctx) (Status, bool)` — if already running, return current
  status + `false`; else flip `running`, launch the pass in a goroutine, return
  `true`. `Status()` returns a snapshot.

- [ ] **Step 3:** The pass: `TracksNeedingEnrichment`; for each, `SearchRecording`;
  if hit and `score >= matchThreshold`, build `Enrichment` + `ApplyEnrichment`;
  then if the album has a release MBID and `AlbumCoverByTrack` is empty,
  `FetchFrontCover`, write `<coversDir>/<albumID><ext>`, `SetAlbumCover`. Update
  counts under `mu`. Respect `ctx` cancellation between tracks. Log a summary.

- [ ] **Step 4:** `matchThreshold = 90` constant. Define small interfaces for the
  MB/CAA clients so tests inject fakes (no network): assert a high-score hit
  applies MBIDs + replaces placeholders + writes a cover file; a low-score hit is
  skipped; a track with existing MBID is not re-queried; `Start` twice does not
  double-run.

- [ ] **Step 5:** Commit: `feat: enrichment pass runner`.

---

## Task 6: HTTP endpoint + cover fallback + wiring

**Files:** modify `internal/api/server.go`, `internal/api/cover.go`, `main.go`;
create `internal/api/enrich.go`

- [ ] **Step 1:** Add an `enrich *enrich.Runner` field to `Server` (nil-safe).
  A setter `SetEnrichRunner(r)` like `SetListenAddr`.

- [ ] **Step 2:** `internal/api/enrich.go`: `enrichStart` (POST) calls
  `runner.Start(r.Context())` and returns the status JSON (200 whether it started
  or was already running — the body's `running` field tells the caller);
  `enrichStatus` (GET) returns `runner.Status()`. If the runner is nil, 503.

- [ ] **Step 3:** Register `POST /api/enrich` and `GET /api/enrich` in `Handler()`.

- [ ] **Step 4:** In `cover.go` `serveCover`: after the embedded-art lookup fails
  (no picture), call `store.AlbumCoverByTrack`; if a path exists, open + serve it
  (sniff content-type via `http.DetectContentType`); else the existing 404.

- [ ] **Step 5:** In `main.go`: derive `coversDir = filepath.Join(filepath.Dir(cfg.DBPath), "covers")`,
  `os.MkdirAll` it; build `external.New("exit66jukebox/0.1 (+https://github.com/andybarilla/exit66jukebox)", time.Second)`,
  the MB + CAA clients, the `enrich.Runner`; `srv.SetEnrichRunner(runner)`.

- [ ] **Step 6:** Tests in `internal/api`: `POST /api/enrich` with a fake runner
  returns status JSON and flips running; `serveCover` falls back to a temp cover
  file when the track has no embedded art (reuse the existing cover test fixture
  approach). `go test ./internal/api/...`.

- [ ] **Step 7:** Commit: `feat: enrich endpoint + cached-cover fallback`.

---

## Task 7: Final verification

- [ ] `go build ./...` and `go vet ./...` clean.
- [ ] `go test ./...` passes (no live network — all external calls hit `httptest`
  or fakes).
- [ ] `gofmt -l .` reports nothing.
- [ ] Manual smoke (optional, documented in the PR, not CI): point at a small
  library with a deliberately mistagged file, `POST /api/enrich`, confirm the
  MBID is filled and a cover appears; re-run confirms the matched track is
  skipped.

---

## Out of scope (future children / issues)

- AcoustID fingerprint matching (needs the `fpcalc` binary).
- Scrobbling, auth, and recommendations (#39/#40/#41).
- Merging duplicate artist/album rows when a rename would collide with an
  existing name — the pass keeps the MBID and skips the rename instead.
- Re-enriching already-matched tracks (force/refresh mode).
