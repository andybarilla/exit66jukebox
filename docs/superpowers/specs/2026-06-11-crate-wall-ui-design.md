# Exit 66 Jukebox — Crate-wall UI design

Implement the "Crate wall" (Layout 03) jukebox interface from the Claude Design
handoff bundle (`Exit 66 Jukebox.dc.html`) as the real front end, wired to the
existing Go backend, with the small backend additions needed to make the UI
honest rather than faked.

The **visual design is fixed**: the bundle contains pixel-exact HTML/CSS for the
neon-noir aesthetic. This spec covers integration — component structure, data
mapping, the stream model, bounded backend changes, and build wiring — not
layout exploration.

## Goals

- Replace the placeholder `web/src/App.svelte` with the full Crate-wall UI:
  desktop three-region layout (top bar, library, lineup rail, player) collapsing
  to a phone build (compact top bar, 2-up grid, mobile player, lineup sheet).
- Library browser with **Artists / Albums / Tracks** tabs and live search.
- One-tap **request** of a track, a whole album, or a whole artist, with toasts.
- **The Lineup** panel: now-playing highlight + upcoming tracks with requester
  attribution and remove control; shuffle toggle.
- **House / Personal** stream switcher in the player bar, each with its own
  now-playing, queue, and listener count.
- Self-hosted fonts so the UI works on a home network with no internet.

## Non-goals

- No login/auth. The requester name is a local display name (default "You").
- No real-time playback-position sync for the house stream (approximated).
- No JS test harness (none exists; not added here).
- No changes to scanning, Sonos, or audio-serving code beyond what the UI needs.

## Visual fidelity reference

Reproduce these source files pixel-for-pixel (from the bundle):

- `Exit 66 Jukebox.dc.html` — main app: top bars, tab group, library views,
  lineup rail, desktop/mobile players, album dialog, toasts, phone sheet/FAB.
- `JukeAlbumCard.dc.html` — album card (cover tile, "Art" label, big initial,
  `+` request button, title/artist/meta).
- `JukeLineup.dc.html` — lineup panel (header + shuffle switch, now-playing card
  with equalizer + progress, "Up next" list, empty state).
- Design tokens: `_ds/.../tokens/{colors,typography,spacing,effects,fonts}.css`
  and `styles.css`. Port the custom properties verbatim into `tokens.css`.

Notable design-system rules to honor: UPPERCASE tracked display type (Chakra
Petch), Space Grotesk body, Space Mono micro-labels; neon used as light (edges,
1.5px rings, zero-blur glows) never flat fields; scanline texture on cards/bars;
sharp machined radii (0/2/3/5/8px); the equalizer is the one ambient animation.

## Frontend architecture (`web/src/`)

State lives in a small module so SSE updates and HMR both behave. Components are
thin and prop-driven, mirroring the bundle's component split.

- `lib/tokens.css` — ported CSS custom properties + `@font-face` rules pointing
  at self-hosted `.woff2` files in `assets/fonts/`. Imported once from
  `app.css` / `main.js`.
- `lib/api.js` — extend the existing client: `listArtists`, `listAlbums`,
  per-item cover URLs, `removeRequest`, `setShuffle`, and a `by` (requester)
  param on requests. Keep existing `subscribeEvents`, `getQueue`, Sonos calls.
- `lib/store.js` — app state: active tab, query, current stream (`me`/`house`),
  loaded library (artists/albums/tracks), derived slot codes, per-stream
  now-playing + queue + listeners, shuffle, toasts, responsive `isPhone`,
  lineup-sheet open, album-detail id, local display name.
- `App.svelte` — shell: responsive switch, mounts top bar, library, rail/sheet,
  player, dialog, toast host; owns the 1s now-playing tick.
- Components: `TopBar.svelte`, `Tabs.svelte`, `AlbumCard.svelte`,
  `AlbumGrid.svelte`, `ArtistList.svelte`, `TrackRow.svelte`, `TrackList.svelte`,
  `Lineup.svelte` (rail + sheet via an `isPhone` prop), `NowPlayingBar.svelte`
  (desktop) with the stream switcher, `MobilePlayer.svelte`, `AlbumDialog.svelte`,
  `Toast.svelte`.

Breakpoint: phone `< 760px` (matches the bundle's `onResize`).

## Data mapping

| Mock element | Real source / derivation |
| --- | --- |
| Albums / Artists / Tracks | `/api/albums`, `/api/artists`, `/api/tracks`, loaded once with a raised `limit`; **filtered client-side** for instant live search across title / artist / album / slot code (matches the bundle, and is required because slot codes are client-derived) |
| Gradient cover tiles | Real art via `/api/albums/{id}/cover` and `/api/tracks/{id}/cover`; on image error, fall back to a deterministic gradient derived from the id (so the grid still reads as art) |
| 2-char slot codes (`A1`) | Derived client-side: stable album index → base-26 letter(s) (A, B, … Z, AA, …) + `track_no`; lets the library exceed 26 albums |
| "Personal" stream | private `me` stream (client `<audio>`, FIFO via `/api/streams/me/next`) |
| "House" stream | shared `house` stream (server MP3 at `/stream/house.mp3` + SSE now-playing) |
| Requester avatars | Real `added_by` from the backend; avatar shows the name's initial |
| Listener count | Real: house = Hub listener count; personal = "Just you" |

### Now-playing per stream

- **Personal (`me`):** client-driven. The `<audio>` element plays
  `/api/tracks/{id}/audio`; on `ended`, pop `/api/streams/me/next` and advance.
  Progress comes from `audio.currentTime` (exact). Seek/volume act on the element.
- **House:** SSE `now-playing` events set the track. There is **no playback
  offset** in the protocol, so progress is approximated by ticking client-side
  from event-arrival time; it drifts slightly and starts at 0 even if you tune
  in mid-track. Controls that imply server transport (prev/seek) are limited to
  what the shared stream supports; play/pause and volume act on the local audio
  element tuned into the feed.

## Backend deltas (Go)

All three are small and make the UI honest. Each ships with a Go test; existing
tests stay green (`go test ./...`, run by CI).

### 1. Requester attribution

`queue_item.added_by` already exists and `store.Enqueue` writes it — only the
read path drops it. Thread it through:

- `store`: a queue read that returns `(trackID, addedBy)` pairs in play order
  (new function or extend `QueueTrackIDs`); keep the existing id-only helper if
  other callers need it.
- `jukebox.Queue`: return tracks paired with their requester. Introduce a small
  `QueuedTrack { model.Track; RequestedBy string }` (JSON `requested_by`) rather
  than overloading `model.Track`.
- `jukebox.Request` / `requestMany` / `RequestAlbum` / `RequestArtist`: accept a
  `requestedBy` string and pass it to `Enqueue` (currently hard-coded `""`).
- API `POST /api/streams/{id}/requests`: read an optional `by` form field
  (default `""`); `GET /api/streams/{id}` returns queue items including
  `requested_by`.
- Test: enqueue with a requester, assert the queue read returns it.

### 2. Listener count

- `broadcast.Hub.ListenerCount() int` — `len(h.listeners)` under the mutex.
- Surface it: `GET /api/streams/{id}` includes `listeners` (house = hub count
  via the server's registered hub; private streams = omitted/0, UI shows
  "Just you").
- Test: register listeners, assert the count.

### 3. Shuffle (affects what plays next only)

- In-memory per-stream shuffle flag on `Jukebox` (`map[string]bool` + mutex);
  not persisted (resets on restart — acceptable).
- `store.PopRandom` (or a `shuffle bool` arg to `PopNext`) selects a random
  queued row instead of the lowest `play_order`, keeping the same atomic
  delete + history + play-count semantics.
- `jukebox.Next` consults the flag for the stream.
- API `POST /api/streams/{id}/shuffle` with a `value` form field; the house
  stream and personal stream both honor it. The visible lineup is **not**
  reordered — only the pop choice changes.
- Test: with shuffle on, popping from a multi-item queue still removes exactly
  one item, records history, and empties correctly.

## Build / dev wiring

- `vite build` already outputs to `../internal/web/dist` (the committed
  `//go:embed all:dist` dir) — unchanged; the Go binary keeps building without
  npm because `dist` is committed.
- Add `/stream` to the vite dev proxy (currently only `/api`) so the house MP3
  works under `npm run dev`. SSE (`/api/streams/{id}/events`) already rides the
  `/api` proxy.
- Self-hosted font `.woff2` files live under `web/src/assets/fonts/` and are
  bundled by Vite, so they land in the embedded `dist` automatically.
- `make build` (runs `make ui` then `go build`) remains the release path.

## Risks / open notes

- **House progress drift** — stated above; approximation only.
- **Slot-code stability** — codes are derived from a stable client-side album
  ordering; they are display sugar, never sent to the backend (requests use real
  numeric ids), so churn is cosmetic.
- **Cover art availability** — many tracks may lack embedded art; the gradient
  fallback keeps the grid looking intentional.
