# Exit 66 Jukebox — Modern Revival (v1 Design)

## Background

The original Exit 66 Jukebox (2001–2011, Java + Jetty + HSQLDB) was a self-hosted
LAN music server: scan local folders, index tracks, browse from any device's
browser, request songs into a queue, stream over the network. Its distinctive
idea — the part nothing off-the-shelf replicates — was the **request-queue with
fairness rules**: no duplicate requests, no recently-played repeats, optional
shuffle. The 2011 code keyed that queue per browser session.

This revival keeps that soul and modernizes everything around it. It is the
**core hub** of a larger personal music system; later phases add Sonos casting,
music discovery, and external-service integration, each as its own spec.

## Goal

A single self-hosted binary that the household runs in the background, plays the
local MP3/OGG/FLAC library, and serves the jukebox to any device on the LAN — with
two playback modes that share one data model.

## The unifying abstraction: Stream

Everything orbits one first-class entity:

```
Stream { id, name, kind: private | shared, queue[], playhead, listeners[], fairness{} }
```

A **queue belongs to a stream.** The two playback modes are two renderings of the
same model:

- **Private stream** — created per browser session, ephemeral. The browser plays
  the next track file directly and asks for the next when one ends. No server-side
  transcoding. This is the original 2011 per-session model, modernized. One
  listener (you).

- **Shared stream** — named, persistent, joinable. The server runs an FFmpeg
  pipeline that decodes each queued track into **one continuous MP3 feed** at a
  stable URL (`/stream/{id}.mp3`). Any client — browser `<audio>`, VLC, later a
  Sonos — tunes in and hears the same playhead. MP3/CBR for maximum compatibility.

The queue and fairness logic are identical for both kinds; they differ only in how
audio is rendered. This keeps the "soul" (fairness) in one place.

## Fairness logic (per stream)

Revived from the original `RequestQueue`, scoped to a stream:

- Reject a track already queued in this stream.
- Reject a track in this stream's recent history (configurable window, N tracks).
- `requestAlbum` / `requestArtist` = bulk requests, reporting partial success
  (all / some / none queued).
- `next` pops the head (or shuffles from the top-N when shuffle is on), records the
  play in history, and increments the track's playcount.

## Stack

- **Backend:** Go, compiled to a single static binary that runs as a background
  daemon. Embeds the built web UI via `go:embed`.
- **Audio rendering (shared streams only):** FFmpeg subprocess.
- **Storage:** SQLite (single file; replaces HSQLDB).
- **Tag reading:** `dhowden/tag` (pure Go, no CGo — preserves the single-binary).
- **Frontend:** Svelte SPA, served from the binary.

Rationale: scanning and streaming are the v1 core and are exactly what Go does
well; the single binary matches the app's always-on daemon nature. Future limbs
(Sonos = UPnP, MusicBrainz/ListenBrainz/Last.fm = HTTP) are all reachable from Go;
if the discovery limb ever needs heavy Python ML libraries, it can run as a
separate sidecar the Go core calls.

## Data model (SQLite)

- `library_root` — scanned folder paths.
- `track` — path, mtime, size, title, artist_id, album_id, track number, genre,
  duration, playcount, cover reference.
- `artist`, `album` — normalized lookups.
- `stream` — id, name, kind, owner/session, fairness config.
- `queue_item` — stream_id, track_id, play_order, added_by.
- `history` — stream_id, track_id, played_at (drives anti-repeat fairness).

## Scanner (fast by design)

The original (and an earlier Python POC) scanned slowly because of per-file DB
commits and single-threaded synchronous tag reads. This design avoids all three
known culprits:

- **Incremental:** walk roots, compare mtime + size against the DB, only (re)read
  new or changed files.
- **Concurrent:** worker pool for tag reads (I/O-bound work).
- **Batched:** SQLite writes grouped into transactions, never per-file.
- Runs in a background goroutine; progress is pushed to the UI over SSE.

## API & real-time

REST/JSON:

- **Browse:** `GET /api/artists`, `/api/albums`, `/api/tracks` — paged, with search.
- **Streams:** `GET /api/streams`, `POST /api/streams` (create shared),
  `GET /api/streams/{id}` (now-playing + queue), join semantics for shared.
- **Requests:** `POST /api/streams/{id}/requests` (track/album/artist),
  `DELETE /api/streams/{id}/requests/{trackId}` (remove), clear-all.
- **Private next track:** `GET /api/streams/{id}/next`.
- **Audio:** `GET /api/tracks/{id}/audio` (private, original file),
  `GET /stream/{id}.mp3` (shared continuous feed).
- **Art:** cover-art endpoints for tracks and albums.

Now-playing and queue changes are pushed via **SSE** (replaces the original's
1-second polling). Clients still POST requests over REST.

## Highest-risk component

The **shared-stream broadcaster**: a long-lived FFmpeg feed that advances through
the queue and fans bytes out to multiple listeners, including late-joiners who
must pick up the live playhead. Build it behind a clean interface so the private
path is fully functional even while the shared path iterates. Everything else in
v1 is well-trodden.

## Out of scope for v1 (later phases, each its own spec)

- Sonos casting (UPnP `SetAVTransportURI` pointing a Sonos at a shared-stream URL).
- Music discovery.
- External-service integration (MusicBrainz / ListenBrainz / Last.fm, streaming
  services).
- Rule-based dynamic playlists (revived from the original `PlaylistRule`) — a
  fast-follow after v1.
- Bandwidth transcoding, user accounts/auth (home LAN trust assumed for v1),
  native mobile apps.

## Alexa (explicitly not pursued)

Echo devices expose no open local API to push arbitrary audio; the only paths are
a fragile cloud custom-skill or Bluetooth-pairing the Echo to the server box.
Out of scope. Sonos, by contrast, fits the "everything is a stream URL" backbone
and is a realistic later phase.
