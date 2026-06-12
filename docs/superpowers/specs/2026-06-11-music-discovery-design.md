# Music Discovery — Design (Issue #19)

## Summary

Surface music the household hasn't played, built **purely on the local library + play stats**. No dependency on external services (issue #20); #20 can layer richer recommendations on top later.

Three surfaces:

1. **Rediscover** — tracks with low playcount and stale recency.
2. **Recently added** — newest tracks in the library.
3. **Genre stations** — a continuous "radio" attached to a stream that auto-refills its queue as it drains, seeded by a genre.

All three are filterable by genre. Rediscover and Recently-added are browse lists (request into a queue like the existing browse views). Genre stations are an auto-feed mechanic on a stream.

## Foundation (already present)

- `track.play_count` (cumulative) and a `history(stream_id, track_id, played_at)` table; `PopNext` records history + bumps playcount atomically.
- `track.genre` is indexed. No year/decade is parsed (decade stations are **out of scope**).
- Queue/playback is **pull-driven**: a track only advances when `jukebox.Next()` is called. There are no background goroutines in the queue path.
- Events are published at the **API layer** (`publishQueueChanged` in `streams.go`), not in jukebox/store.
- No migration framework: `store.Open` applies `schema.sql` with `CREATE TABLE IF NOT EXISTS`, which cannot alter existing tables.

## Schema + migration

Add a guarded `migrate(db)` step in `store.Open`, run after the schema apply.

- **`track.added_at INTEGER NOT NULL DEFAULT 0`** — added via `ALTER TABLE track ADD COLUMN added_at ...`, guarded by a `pragma_table_info('track')` check so it is idempotent and safe on existing databases. Backfill once: `UPDATE track SET added_at = mod_time WHERE added_at = 0`. `UpsertTrack` sets `added_at` to the current time on new inserts.
- **`station` table** (new, in `schema.sql`):

  ```sql
  CREATE TABLE IF NOT EXISTS station (
      stream_id TEXT PRIMARY KEY REFERENCES stream(id),
      genre     TEXT NOT NULL,
      threshold INTEGER NOT NULL,
      batch     INTEGER NOT NULL
  );
  ```

  One station per stream (matches coexist + one-genre-per-stream). Stopping a station deletes the row.

## Selection seam

A single store function backs all three surfaces:

```
DiscoverTracks(db, opts) ([]model.Track, error)
  opts:
    Genre         string  // optional filter ("" = all genres)
    OrderBy       string  // "rediscover" | "recent" | "random"
    ExcludeStream string  // optional: skip tracks within this stream's recent history window
    Window        int     // history window for ExcludeStream
    Limit, Offset int
```

`last_played` is computed as `MAX(history.played_at)` per track (0 / NULL = never played), via a `LEFT JOIN` or correlated subquery.

- **rediscover** → `ORDER BY play_count ASC, last_played ASC` (low playcount *and* stale recency — the explicit definition of "rediscover").
- **recent** → `ORDER BY added_at DESC`.
- **random** (radio refill) → genre filter + `ORDER BY RANDOM()`, with `ExcludeStream`/`Window` set so the station does not re-add recently-played tracks.

## HTTP API

Browse lists (genre-filterable via `?genre=`, paginated via `?limit`/`?offset` like existing browse):

- `GET /api/discover/rediscover` → `[]Track`
- `GET /api/discover/recent` → `[]Track`
- `GET /api/discover/genres` → distinct genres with track counts (for the station picker and filter dropdowns)

Station control (one station per stream):

- `POST /api/streams/{id}/station` body `{ "genre": "Rock" }` → upsert station row (defaults `threshold=3`, `batch=10`), perform an **immediate fill**, then publish `queue-changed`. Returns the station.
- `DELETE /api/streams/{id}/station` → delete the station row (halts future refills; queued tracks keep playing).
- `GET /api/streams/{id}/station` → current station or empty, so the UI can show active state.

## Radio refill (pull-on-drain + cold-start fill)

Two triggers:

1. **Cold-start fill** — on `POST .../station`, immediately enqueue `batch` tracks. An empty queue never drains (issue #28), so without this nothing would ever play.
2. **Refill on drain** — in `jukebox.Next()`, after `PopNext` commits: if a station exists for the stream and the queue length `< threshold`, enqueue `batch` tracks via `DiscoverTracks(OrderBy=random, Genre=station.genre, ExcludeStream=streamID, Window=HistoryWindow)`.

Refill reuses the existing `jukebox.Request` path so the existing fairness rules (`InQueue` dedupe, `HistoryWindow`) apply uniformly. Enqueued tracks carry `added_by = "station"`.

- **Events:** the `nextTrack` handler publishes `queue-changed` after `Next` returns, covering both the pop and any refill. The station POST handler publishes after its immediate fill.
- **Genre exhausted:** if no eligible tracks remain, refill is a no-op and the stream drains normally. Acceptable for v1.
- **Coexist:** the station never clears or blocks manual requests; it only tops up when the queue is low. Manually requested tracks play in order alongside station picks.

## Frontend (Svelte, `web/src/`)

- `web/src/lib/api.js`: add `discoverRediscover(genre)`, `discoverRecent(genre)`, `discoverGenres()`, `getStation(streamID)`, `startStation(streamID, genre)`, `stopStation(streamID)`.
- `web/src/App.svelte`: a **Discover** view with three sections —
  - Rediscover and Recently Added: track lists with a genre-filter dropdown, each row requestable into the current queue (reuse existing request UI).
  - Genre stations: pick a genre → **Start**; when a station is active, show it with a **Stop** button.
- Rebuild the SPA into `internal/web/dist` and commit both `web/src/` and the built assets (per the existing v2/v3 plan convention).

## Testing

- **Migration:** `Open` on a DB with a pre-existing `track` table (no `added_at`) adds the column and backfills from `mod_time`; running twice is a no-op.
- **Selection seam:** seed tracks with varied playcount/history/added_at/genre; assert ordering for each `OrderBy` and that `Genre`/`ExcludeStream` filter correctly.
- **Station:** start on an empty queue fills immediately; `Next` below threshold refills; at/above threshold does not; stop halts refills but leaves the queue intact; refill respects `HistoryWindow` and `InQueue`.
- **API:** discover endpoints return JSON lists and honor `?genre=`; station POST/GET/DELETE round-trip.

## Out of scope

- Decade stations (no year metadata parsed).
- Similar-artist suggestions — deferred to issue #20 (requires external metadata).
- Any external service (MusicBrainz / ListenBrainz / Last.fm) — issue #20.
- Background ticker / scheduled refill — playback is pull-driven; not needed.
