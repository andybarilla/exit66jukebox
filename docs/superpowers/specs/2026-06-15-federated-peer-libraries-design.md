# Federated Peer Libraries

## Problem

Today an exit66 instance only knows its own scanned files. The goal: run several
instances that share their libraries, so a remote instance's tracks appear as
ordinary entries in the local library. A VPS instance can then play music whose
files live on a home LAN; a home or office instance can play another's tracks
too.

Playback always happens **local to whichever instance you are using** — the VPS
streams to your browser, an office instance plays to its own Sonos. The instance
that *owns* the file never plays; it only serves the bytes.

## Concepts

- **Peer / instance** — one running daemon with its own SQLite library and local
  outputs (browser stream, local Sonos).
- **Hub** — the publicly reachable instance (the VPS). Other peers dial out to
  it. The hub is also a normal peer with its own library.
- **Member** — a peer behind NAT that dials out to the hub.
- **Federation** — the set of peers sharing catalogs through one hub. v1 has a
  single implicit federation. A future "listening group" concept (named,
  membership-controlled) is out of scope.

## Topology & connectivity

Members are behind NAT, so they dial **out** to the hub's public TLS address and
hold one persistent connection. A [yamux](https://github.com/hashicorp/yamux)
session is multiplexed over that single connection. yamux is symmetric: once the
session is up, either side opens streams on demand.

- **Member-initiated streams** carry catalog sync (member → hub) and audio-fetch
  requests (member asks the hub to relay a track owned by another peer).
- **Hub-initiated streams** let the hub fetch audio *from* a member to satisfy
  another peer's request.

All inter-peer traffic transits the hub (any-to-any relay). Direct peer-to-peer
audio that bypasses the hub when NAT permits is implemented as an optional
transport on top of this relay (see "Direct P2P audio transport" below; #124).

### Auth

A member presents a shared token when registering. The hub compares it with
`crypto/subtle.ConstantTimeCompare` against its configured federation secret; a
bad token refuses the connection. One shared token for v1 (this is a personal
deployment). A future listening-group model would replace this with per-group
membership.

## Track identity & storage

Keep `model.Track.ID int64` and the existing API surface unchanged. Distinguish
remote rows with two new columns on the catalog tables:

- `source_peer TEXT` — empty/NULL for local rows; the owning peer's id otherwise.
- `remote_id INTEGER` — the track's id on its owner; NULL for local.

Local autoincrement ids stay unique within each peer's DB, so the UI and every
existing endpoint keep using int64 ids as they do now. Only **audio resolution**
branches on `source_peer`:

- local row → `http.ServeFile(path)` as today.
- remote row → proxy to the federated audio path for `(source_peer, remote_id)`.

Remote tracks have no local `path`, so `UpsertTrack`'s `path`-unique key cannot
apply to them. Remote rows are keyed by a unique index on
`(source_peer, remote_id)`. Browse stays unified: catalog sync upserts remote
artists/albums through the existing upsert path, so identical names coalesce
into the same artist/album rows (the same mechanism as compilation re-keying)
while each track stays attributed to its owning peer. Cross-peer dedupe of
identical files is out of scope.

## Catalog sync

"Sync & cache via hub" — browse stays fully local and fast; remote rows may be
briefly stale until the next sync.

1. On connect, a member pushes a full catalog snapshot over a dedicated control
   stream: the browse/enrichment fields plus `remote_id` per track (title,
   artist name, album name, album-artist, track_no, genre, duration, links,
   mbid, remote_id).
2. On local scan changes, the member pushes incremental upserts/deletes.
3. The hub aggregates every peer's catalog and pushes the merged set down to each
   member (excluding that member's own local rows). Members upsert the remote
   rows into their DB tagged with `source_peer`/`remote_id`.
4. The hub tracks each peer online/offline. Cached remote rows persist while a
   peer is offline; the UI greys them out and a play attempt returns a clear
   error.

Sync messages are newline-delimited JSON over the control stream.

## Audio relay

Each peer runs a local handler `GET /fed/audio/{peer}/{id}` that proxies over its
hub session. Both the browser-playback replacement for `ServeFile` and the
shared-stream ffmpeg source use this one loopback handler, so the "remote source"
seam is a single uniform path.

Flow for member A playing track T (owner = peer B, remote id = R):

1. A's audio handler sees `source_peer = B` and issues
   `GET /fed/audio/B/R` to its **own** local fed handler (loopback), forwarding
   any `Range` header.
2. That handler opens a yamux stream to the hub and reverse-proxies the request.
3. The hub's `/fed/audio/{peer}/{id}` handler looks up B's live session
   (503 if B is offline) and reverse-proxies `GET /audio/R` over a yamux stream
   to B, forwarding `Range`.
4. B serves it through the existing `trackAudio` → `ServeFile`, honoring `Range`
   (206 partial content) for seeking.
5. Bytes stream back B → hub → A → output. A treats the response exactly as a
   local file today.

The shared stream's `FFmpegSource.Open` takes a URL instead of a path for remote
tracks: `ffmpeg -i http://127.0.0.1:<port>/fed/audio/B/R ...`, hitting the same
loopback handler. ffmpeg accepts an HTTP URL as input directly.

Because the hub only ever dials registered peer sessions — never arbitrary URLs
— the relay adds no SSRF surface. The existing Sonos IP allowlist is orthogonal
and unchanged.

## Components

New package `internal/fed/`:

- `session.go` — yamux session lifecycle. Member dialer with reconnect/backoff;
  hub acceptor + peer registry with token auth and online/offline tracking.
- `sync.go` — catalog push/merge protocol over the control stream.
- `relay.go` — hub `/fed/audio/{peer}/{id}` reverse proxy over peer sessions, and
  the member-local `/fed/audio` proxy into the hub session.

Changed:

- `internal/config/config.go` — federation role (`hub` | `member` | off), hub
  address (members), shared token, this peer's id/name. Token from env, matching
  the existing secret-handling convention.
- `internal/store/` — migration adding `source_peer`/`remote_id` + the unique
  index; upsert path for remote rows; `GetTrack` returns `source_peer`/
  `remote_id` for audio resolution.
- `internal/api/audio.go` and the shared-stream source — branch on `source_peer`
  to proxy vs. `ServeFile`; ffmpeg source takes a URL for remote tracks.
- `internal/api/server.go` — register fed routes, wire the fed manager, reuse
  `requireAdmin` on federated control actions.
- `web/` (Svelte) — show owner/availability on remote tracks, grey out offline
  ones. Minimal; the merged library already renders through existing components.

## Security

- **Peer ↔ hub:** shared token, constant-time compare. The hub's public listener
  is TLS, since the connection carries catalog and audio over the internet.
  `/fed/*` and registration require the token; they are never open.
- **User ↔ instance:** reuse the issue #85 admin gate (shared-password →
  bearer-token, `requireAdmin` wrapper). Federated control actions (skip/cast on
  a remote-sourced stream) inherit the same guard.

**Dependency:** the admin gate (#85) is implemented on branch
`issue-85-admin-gate` but not yet merged to `main`. This work assumes it lands
first.

## Testing

- **Unit:** catalog merge/upsert with source tagging; audio-resolution branching;
  token compare.
- **Integration:** two in-process peers plus a hub over loopback yamux —
  register, sync catalog, fetch audio with `Range` and assert 206 semantics and
  byte-for-byte content; disconnect a peer and assert 503 on play while the
  cached catalog persists.
- Reconnect/backoff behavior on dropped hub connections.

## Out of scope (future issues)

- **Listening groups** — named federations with membership control, replacing the
  single shared token.
- Cross-peer dedupe of identical files.

## Direct P2P audio transport (#124)

The v1 hub relay is the default and fallback path. On top of it there are now
two direct transports that bypass the hub when connectivity allows, selected by
the `directResolver` cascade in `internal/fed/relay.go` (peer role only):

1. **WebRTC data channel** (`internal/fed/webrtc.go`) — NAT-traversing. ICE
   gathers host, server-reflexive (STUN), and relay (TURN) candidates so two
   peers behind NAT can stream audio directly with no inbound firewall openings.
   This is the tier that reaches peers the yamux path cannot.
2. **yamux TCP direct path** (issue #87, `directResolver` body) — works when the
   peer is reachable (same LAN via mDNS discovery, or a routable/port-forwarded
   address).

Each tier fails closed to the next; if all direct tiers fail, playback continues
through the hub relay with no user-visible breakage.

**Signaling** rides the authenticated hub relay (`internal/fed/signaling.go`):
SDP offer/answer and trickle ICE candidates are relayed only between registered,
token-authenticated peers (a `POST /fed/signal/{to}` endpoint on the hub
session), preserving the federation's SSRF-safety property. Correlation across
an exchange is by a per-negotiation SID. Peers advertise direct-transport
support via capabilities exchanged over the authenticated session
(`internal/fed/caps.go`).

**Required settings:** at least one STUN server (`EXIT66_FED_STUN`, default
`stun:stun.l.google.com:19302`). A TURN server (`EXIT66_FED_TURN` as
`turn://user:pass@host:port`) is required for symmetric NAT or restrictive
firewalls where a host/srflx candidate pair cannot connect. Direct P2P can be
disabled with `EXIT66_FED_DIRECT_P2P=0`; the hub relay then behaves exactly as
before.

**Why WebRTC over other transports:** pion/webrtc provides a full ICE/STUN/TURN
implementation in pure Go (no cgo), and data channels give reliable ordered
byte streams with no inbound firewall openings — the property the existing
yamux-direct path lacks for NAT'd peers. Range/seek semantics pass through a
tiny length-prefixed request/response frame (`internal/fed/webrtc_audio.go`)
mirroring the HTTP headers the relay path preserves.

**Diagnostics:** each resolved stream logs its transport
(`transport=webrtc|direct|relay`) so the chosen path is observable without a UI.

