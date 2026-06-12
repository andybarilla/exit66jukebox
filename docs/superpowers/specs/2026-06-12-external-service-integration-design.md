# External-Service Integration — Architecture (Issue #20)

## Summary

Connect Exit 66 to three external HTTP services, layered on top of the local
library and play stats:

1. **MusicBrainz + Cover Art Archive** — canonical IDs (MBIDs) and better
   tags/cover art for poorly-tagged files. Read-only, no auth.
2. **ListenBrainz** — scrobble plays outward; later pull recommendations in.
   Single pasted user token.
3. **Last.fm** — scrobble plays outward; pull similar-artist data. Full
   API-key + browser auth flow (heavier than ListenBrainz).

All are HTTP APIs that fit the Go core. This is an **epic**: this document is
the shared architecture; each capability ships as its own child issue with its
own `plan-issue` pass. #20 itself is the umbrella and stays open until the
children land.

## Foundation (already present)

- `track.play_count` (cumulative) and `history(stream_id, track_id, played_at)`;
  `PopNext` records history + bumps playcount atomically (`internal/store/queue.go`).
- `track.added_at` plus the `migrate(db)` step in `store.Open` (guarded
  `ALTER TABLE` via `pragma_table_info`) — established by the discovery work
  (#19). New columns follow the same idempotent pattern.
- `scan.ReadTags` (`internal/scan/tags.go`) reads tags via `dhowden/tag` and
  normalizes blanks to `Unknown Artist` / `Unknown Album`. This is the seam
  where MusicBrainz enrichment can fill or correct fields.
- Cover art today: `internal/api/cover.go` (+ `album.cover` path). CAA can be a
  fallback source when no embedded/sidecar art exists.
- `DiscoverTracks` selection seam (`internal/store/discover.go`) backs all
  Discovery surfaces. Recommendations from ListenBrainz/Last.fm feed a **new**
  surface alongside it, not into its local-only ordering.
- **Config is flag-only** (`internal/config/config.go`) — no secret storage
  exists yet. See "Secrets & config" below; this is a real cross-cutting
  sub-task, not a footnote.
- Playback is **pull-driven**: no background goroutines in the queue path.
  Scrobble delivery introduces the first background worker (the scrobble queue
  drainer); enrichment runs as an explicit pass, not a ticker.

## Cross-cutting concerns

### Secrets & config

Tokens must not be passed as `-flag` values (they leak via the process list).
Add a secret-bearing config source the whole epic shares:

- Read service credentials from **environment variables** (12-factor; never
  logged), with an optional config file for local dev. Proposed names:
  `EXIT66_LISTENBRAINZ_TOKEN`, `EXIT66_LASTFM_API_KEY`,
  `EXIT66_LASTFM_API_SECRET`. The Last.fm **session key** is obtained at runtime
  (not a static secret) and is persisted in the DB (see Last.fm child).
- Extend `config.Config` with a `Services` sub-struct populated from env, kept
  out of the flag set. A service with no credentials is simply disabled — the
  app runs identically to today.

This config work is a prerequisite and is owned by the **first scrobble child**
(ListenBrainz), not duplicated across children.

### Shared HTTP client

A small `internal/external` package with one rate-limited, retrying HTTP client
helper:

- Mandatory descriptive `User-Agent` (`exit66jukebox/<ver> (+url)`) — required
  by MusicBrainz and good manners for the rest.
- **MusicBrainz: ≤1 req/sec** (hard requirement) — a per-host token-bucket
  limiter lives here.
- Timeouts, a couple of retries with backoff on 5xx/429 (honor `Retry-After`).
- Each service gets a thin typed client (`musicbrainz`, `listenbrainz`,
  `lastfm`) built on this helper. Network calls are isolated here so the rest of
  the app stays testable against an interface + `httptest`.

### Schema additions

Follow the `#19` idempotent-migration pattern (guarded `ALTER TABLE` +
`pragma_table_info`). MBIDs are nullable text, empty = not yet enriched:

```sql
ALTER TABLE artist ADD COLUMN mbid TEXT NOT NULL DEFAULT '';
ALTER TABLE album  ADD COLUMN mbid TEXT NOT NULL DEFAULT '';
ALTER TABLE track  ADD COLUMN mbid TEXT NOT NULL DEFAULT '';
```

Scrobble durability needs a queue table (one row per pending listen, drained by
the background worker, deleted on success):

```sql
CREATE TABLE IF NOT EXISTS scrobble_queue (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    service    TEXT NOT NULL,            -- 'listenbrainz' | 'lastfm'
    track_id   INTEGER NOT NULL,
    played_at  INTEGER NOT NULL,         -- unix seconds (the listen timestamp)
    attempts   INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL
);
```

Last.fm's runtime session key is persisted (small `service_auth` row or a
dedicated column) so the auth flow runs once.

### Scrobble delivery (durable queue)

Decided, not put to a vote — offline resilience is the obvious default:

- When a track passes the scrobble threshold (**>30s long AND half its duration
  or 4 min elapsed, whichever is earlier** — identical for ListenBrainz and
  Last.fm), enqueue one `scrobble_queue` row per enabled service.
- A single background worker drains the queue: batches (ListenBrainz `single`
  payloads / Last.fm `track.scrobble`, ≤50 per request), deletes rows on 2xx,
  bumps `attempts` + backs off on failure. Survives restarts because the queue
  is in SQLite.
- **"Now playing"** (`track.updateNowPlaying` / ListenBrainz `playing_now`) is
  fire-and-forget on track start — never queued, never retried.
- The threshold trigger is the open design question for the scrobble children:
  playback advancement is pull-driven, so "elapsed listening time" must be
  derived (track start timestamp + the pull that advances past it). Each
  scrobble child's plan nails this down against the then-current jukebox/stream
  playback signal.

## Child issues

Each is thin and `plan-issue`-able on its own. Suggested order = dependency order.

1. **MusicBrainz + Cover Art Archive enrichment** *(no auth — start here)*
   - `internal/external` HTTP helper + rate limiter + `musicbrainz` client.
   - MBID columns + migration.
   - Enrichment pass: for tracks with placeholder/blank tags or empty MBID, look
     up MBIDs and backfill artist/album/track + genre; CAA fallback for missing
     covers. Runs as an explicit, resumable pass (CLI subcommand or API
     trigger), not on every scan and not a background ticker.
   - Idempotent and rate-limit-respecting; safe to re-run.

2. **ListenBrainz scrobbling** *(introduces secrets/config + scrobble queue)*
   - Env-based secrets/config plumbing (shared by later children).
   - `scrobble_queue` table + background drainer + threshold trigger.
   - `listenbrainz` client: `POST /1/submit-listens`, `Token <token>` header,
     `playing_now` for now-playing, `single` for completed listens
     (`listened_at` + `track_metadata{artist_name, track_name, release_name}`).

3. **Last.fm scrobbling** *(full auth flow — heavier)*
   - API-key/secret config + `auth.getToken` → browser-authorize →
     `auth.getSession`, persist the session key; md5-signed POSTs to
     `ws.audioscrobbler.com/2.0/`.
   - Reuse the `scrobble_queue` + drainer (`service='lastfm'`):
     `track.updateNowPlaying` + batched `track.scrobble` (≤50).

4. **Recommendations → Discovery** *(depends on 2/3 for identity/MBIDs)*
   - Pull ListenBrainz recommendations and/or Last.fm similar-artist data, map
     to local tracks via MBIDs (falls back to fuzzy artist/title match), expose
     as a new Discover surface + API endpoint alongside the local-only
     `DiscoverTracks` surfaces. Frontend: a recommendations section in the
     Discover view.

## Testing strategy (applies to every child)

- Service clients tested against `httptest` servers — no live network in CI.
- Rate limiter and retry/backoff have unit tests (fake clock).
- Migration tests: column/table added on a pre-existing DB, idempotent on
  re-run (mirrors `#19`).
- Scrobble queue: threshold gating, enqueue-per-service, drain-on-success,
  backoff-on-failure, survives "restart" (reopen DB), now-playing never queued.
- Enrichment: blank/placeholder tags get filled; existing good tags/MBIDs are
  not clobbered; re-run is a no-op.

## Out of scope (for the epic)

- Writing user loves/bans back to services (read/scrobble only for v1).
- Spotify / Apple Music / other services beyond the three named.
- Replacing local Discovery ordering — recommendations are an additional
  surface, not a rewrite of `DiscoverTracks`.
