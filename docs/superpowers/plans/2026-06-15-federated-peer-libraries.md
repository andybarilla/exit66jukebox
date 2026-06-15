# Federated Peer Libraries Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let multiple exit66 instances share their libraries through a public hub, so a remote peer's tracks appear as ordinary library entries and play locally on whichever instance you are using.

**Architecture:** Members behind NAT dial out to a public hub over TLS and multiplex a [yamux](https://github.com/hashicorp/yamux) session over the single connection. Catalogs sync up to the hub and cache down to every member (tagged by owning peer). Audio for a remote track is reverse-proxied: the playing peer asks its hub, the hub relays to the owner's existing `/audio/{id}` endpoint, and bytes stream back — `Range`/seek preserved end to end.

**Tech Stack:** Go 1.26, `github.com/hashicorp/yamux`, `net/http` + `net/http/httputil`, modernc SQLite, Svelte UI.

**Spec:** `docs/superpowers/specs/2026-06-15-federated-peer-libraries-design.md`

**Dependency:** the #85 admin gate (branch `issue-85-admin-gate`) must be merged to `main` before Phase 6 (it provides `requireAdmin` and `config.AdminPassword`). Phases 1–5 do not depend on it.

---

## File Structure

New package `internal/fed/`:
- `internal/fed/resolver.go` — `Resolver` interface + a `peerSource` value type the store/audio layers use to decide local-vs-remote. No networking; the testable seam.
- `internal/fed/session.go` — yamux session lifecycle: member dialer (reconnect/backoff) and hub acceptor + peer registry (token auth, online/offline).
- `internal/fed/transport.go` — an `http.RoundTripper`/`http.Client` whose connections are yamux streams, plus a `net.Listener` adapter so each side can serve HTTP over the session.
- `internal/fed/relay.go` — the hub's `/fed/audio/{peer}/{id}` reverse proxy and the member's loopback `/fed/audio/{peer}/{id}` proxy.
- `internal/fed/sync.go` — catalog push (member→hub) + merge/fan-out (hub→members) over a control stream.
- `internal/fed/manager.go` — wires role (hub/member/off) to sessions, relay, and sync; the single object `main.go` and `api.Server` hold.

Changed:
- `internal/config/config.go` — federation config block.
- `internal/store/migrate.go`, `internal/store/schema.sql`, `internal/store/library.go` — `source_peer`/`remote_id` columns + unique index, remote upsert path, `GetTrack` returns source fields.
- `internal/api/audio.go`, `internal/broadcast/source.go`, `internal/api/server.go` — branch on source on audio resolution; ffmpeg URL source; route registration + manager wiring.
- `main.go` — build the fed manager from config.
- `web/src/lib/*` — owner badge + offline grey-out on remote tracks.

---

## Phase 1 — Config + schema + the resolution seam

Ships independently: schema and config in place, audio resolution that correctly *branches* on a remote source via an injectable resolver (verified with a fake). No networking yet.

### Task 1: Federation config block

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_fed_test.go` (create)

- [ ] **Step 1: Write the failing test**

```go
package config

import (
	"os"
	"testing"
)

func TestFederationFromEnv(t *testing.T) {
	t.Setenv("EXIT66_FED_ROLE", "member")
	t.Setenv("EXIT66_FED_HUB", "hub.example.com:8443")
	t.Setenv("EXIT66_FED_LISTEN", ":8443")
	t.Setenv("EXIT66_FED_TOKEN", "s3cret")
	t.Setenv("EXIT66_FED_PEER_ID", "home")

	f := federationFromEnv()
	if !f.Enabled() {
		t.Fatal("expected federation enabled")
	}
	if f.Role != "member" || f.HubAddr != "hub.example.com:8443" || f.Listen != ":8443" || f.Token != "s3cret" || f.PeerID != "home" {
		t.Fatalf("unexpected federation config: %+v", f)
	}
}

func TestFederationDisabledWhenRoleUnset(t *testing.T) {
	os.Unsetenv("EXIT66_FED_ROLE")
	if federationFromEnv().Enabled() {
		t.Fatal("federation should be disabled when role unset")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestFederation -v`
Expected: FAIL — `federationFromEnv` undefined.

- [ ] **Step 3: Add the config type and loader**

In `internal/config/config.go`, add to the `Config` struct a `Federation Federation` field, and add:

```go
// Federation holds peer-sharing config. Role is "hub", "member", or "" (off).
// Like Services, the token comes from the environment, never a flag, so it
// doesn't leak via the process list. HubAddr is the public host:port a member
// dials. PeerID is this instance's stable identifier within the federation.
type Federation struct {
	Role    string // "hub" | "member" | ""
	HubAddr string // members only: hub's public address to dial
	Listen  string // hub only: local address to listen on (e.g. ":8443")
	Token   string // shared secret presented at registration
	PeerID  string // this instance's id (e.g. "home", "vps")
}

// Enabled reports whether federation is configured.
func (f Federation) Enabled() bool { return f.Role == "hub" || f.Role == "member" }

func federationFromEnv() Federation {
	return Federation{
		Role:    os.Getenv("EXIT66_FED_ROLE"),
		HubAddr: os.Getenv("EXIT66_FED_HUB"),
		Listen:  os.Getenv("EXIT66_FED_LISTEN"),
		Token:   os.Getenv("EXIT66_FED_TOKEN"),
		PeerID:  os.Getenv("EXIT66_FED_PEER_ID"),
	}
}
```

Then, wherever `Config` is assembled (the function that already calls `servicesFromEnv()`), set `cfg.Federation = federationFromEnv()`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestFederation -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_fed_test.go
git commit -m "feat(config): federation config block from env"
```

### Task 2: Schema columns for remote rows

**Files:**
- Modify: `internal/store/schema.sql`, `internal/store/migrate.go`
- Test: `internal/store/fed_schema_test.go` (create)

- [ ] **Step 1: Write the failing test**

```go
package store

import "testing"

func TestRemoteColumnsExist(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, col := range []string{"source_peer", "remote_id"} {
		has, err := columnExists(db, "track", col)
		if err != nil {
			t.Fatal(err)
		}
		if !has {
			t.Fatalf("track.%s missing", col)
		}
	}
}

func TestRemoteUniqueIndexRejectsDuplicate(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO artist(name, sort_key) VALUES('A','a')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO album(name, artist_id, sort_key) VALUES('Al', 1, 'al')`); err != nil {
		t.Fatal(err)
	}
	// Distinct paths so the path UNIQUE constraint can't be what rejects the
	// second row — this isolates idx_track_remote on (source_peer, remote_id).
	// Remote rows use synthetic non-empty paths (never opened; audio resolution
	// branches on source_peer first). artist_id/album_id reference the rows above
	// so the foreign keys hold.
	ins := `INSERT INTO track(path, mod_time, size, source_peer, remote_id, title, artist_id, album_id, added_at)
	        VALUES(?, 0, 0, 'home', 5, 't', 1, 1, 0)`
	if _, err := db.Exec(ins, "fed://home/5#a"); err != nil {
		t.Fatalf("first remote insert failed: %v", err)
	}
	if _, err := db.Exec(ins, "fed://home/5#b"); err == nil {
		t.Fatal("expected unique-index violation on duplicate (source_peer, remote_id)")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestRemote -v`
Expected: FAIL — `source_peer` column missing.

- [ ] **Step 3: Add columns in migrate, index in schema**

In `internal/store/migrate.go`, inside `migrate`, after the existing `mbid` loop, add:

```go
	// Federation: remote rows carry their owning peer and the track's id on that
	// peer. Local rows leave both empty/0 (#86). path is "" for remote tracks, so
	// the path-unique index can't key them — a partial unique index on
	// (source_peer, remote_id) covers remote rows instead (added in schema.sql).
	if has, err := columnExists(db, "track", "source_peer"); err != nil {
		return err
	} else if !has {
		if _, err := db.Exec(`ALTER TABLE track ADD COLUMN source_peer TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	if has, err := columnExists(db, "track", "remote_id"); err != nil {
		return err
	} else if !has {
		if _, err := db.Exec(`ALTER TABLE track ADD COLUMN remote_id INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	if _, err := db.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_track_remote
		 ON track(source_peer, remote_id) WHERE source_peer <> ''`,
	); err != nil {
		return err
	}
```

In `internal/store/schema.sql`, add the same columns to the `CREATE TABLE track` definition (so fresh DBs have them without ALTER) — add `source_peer TEXT NOT NULL DEFAULT ''` and `remote_id INTEGER NOT NULL DEFAULT 0` to the column list, and add after the table:

```sql
CREATE UNIQUE INDEX IF NOT EXISTS idx_track_remote
  ON track(source_peer, remote_id) WHERE source_peer <> '';
```

> **Do not change anything else in the track table.** Specifically: keep `path TEXT NOT NULL UNIQUE` as-is (remote rows get a synthetic non-empty path in Task 3, so they don't collide and the UNIQUE constraint still protects local rows); keep `album_id INTEGER NOT NULL REFERENCES album(id)` (remote tracks get real album ids via `upsertAlbum` in Task 3); leave `mod_time`/`size` as they are. The only track-table changes are the two new columns and the new partial index.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestRemote -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/migrate.go internal/store/schema.sql internal/store/fed_schema_test.go
git commit -m "feat(store): source_peer/remote_id columns + partial unique index"
```

### Task 3: GetTrack returns source fields; UpsertRemoteTrack

**Files:**
- Modify: `internal/model/model.go`, `internal/store/library.go`
- Test: `internal/store/fed_library_test.go` (create)

- [ ] **Step 1: Write the failing test**

```go
package store

import "testing"

func TestUpsertRemoteTrackAndResolve(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id, err := UpsertRemoteTrack(db, RemoteTrack{
		SourcePeer: "home", RemoteID: 42,
		Title: "Song", ArtistName: "Artist", AlbumArtist: "Artist", AlbumName: "Album",
		TrackNo: 1, Duration: 180,
	})
	if err != nil {
		t.Fatal(err)
	}
	tr, path, ok := GetTrack(db, id)
	if !ok {
		t.Fatal("track not found")
	}
	// Remote rows get a synthetic non-empty path (never opened — audio resolution
	// branches on source_peer first); the source fields are what callers act on.
	if path != "fed://home/42" {
		t.Fatalf("expected synthetic remote path, got %q", path)
	}
	if tr.SourcePeer != "home" || tr.RemoteID != 42 {
		t.Fatalf("source fields not returned: %+v", tr)
	}

	// Re-upsert same (peer, remote_id) updates rather than duplicating.
	id2, err := UpsertRemoteTrack(db, RemoteTrack{
		SourcePeer: "home", RemoteID: 42, Title: "Song v2",
		ArtistName: "Artist", AlbumArtist: "Artist", AlbumName: "Album",
	})
	if err != nil {
		t.Fatal(err)
	}
	if id2 != id {
		t.Fatalf("expected same row id on re-upsert, got %d != %d", id2, id)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestUpsertRemoteTrack -v`
Expected: FAIL — `UpsertRemoteTrack` / `RemoteTrack` undefined; `tr.SourcePeer` undefined.

- [ ] **Step 3: Add model fields, RemoteTrack, and UpsertRemoteTrack; extend GetTrack**

In `internal/model/model.go`, add to `Track`:

```go
	SourcePeer string `json:"source_peer,omitempty"`
	RemoteID   int64  `json:"-"`
```

In `internal/store/library.go`, extend `GetTrack`'s SELECT and Scan to include the two columns:

```go
func GetTrack(db *sql.DB, id int64) (t model.Track, path string, ok bool) {
	err := db.QueryRow(
		`SELECT id, path, title, artist_id, album_id, track_no, genre, duration, play_count, source_peer, remote_id
		 FROM track WHERE id = ?`, id).Scan(
		&t.ID, &path, &t.Title, &t.ArtistID, &t.AlbumID,
		&t.TrackNo, &t.Genre, &t.Duration, &t.PlayCount, &t.SourcePeer, &t.RemoteID)
	if err != nil {
		return model.Track{}, "", false
	}
	return t, path, true
}
```

Add the remote upsert (reuses the existing `upsertArtist`/`upsertAlbum` so names coalesce into shared rows):

```go
// RemoteTrack is a track owned by another peer, received via catalog sync.
type RemoteTrack struct {
	SourcePeer  string
	RemoteID    int64
	Title       string
	ArtistName  string
	AlbumArtist string
	AlbumName   string
	TrackNo     int
	Genre       string
	Duration    int
	Links       []string
}

// UpsertRemoteTrack inserts or updates a track owned by another peer, keyed by
// (source_peer, remote_id). It reuses the local artist/album upsert path so a
// remote album with the same name as a local one collapses into one browse row.
// The row carries no path; audio resolution proxies it back to its owner.
func UpsertRemoteTrack(db *sql.DB, rt RemoteTrack) (int64, error) {
	if rt.AlbumArtist == "" {
		rt.AlbumArtist = rt.ArtistName
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	artistID, err := upsertArtist(tx, rt.ArtistName)
	if err != nil {
		return 0, err
	}
	albumArtistID, err := upsertArtist(tx, rt.AlbumArtist)
	if err != nil {
		return 0, err
	}
	albumID, err := upsertAlbum(tx, rt.AlbumName, albumArtistID)
	if err != nil {
		return 0, err
	}
	// Synthetic non-empty path keeps the track.path UNIQUE constraint satisfied
	// (remote rows never collide with local ones and are never opened as files).
	// The ON CONFLICT target repeats the partial index's WHERE predicate, which
	// SQLite requires to match a partial unique index.
	synthPath := fmt.Sprintf("fed://%s/%d", rt.SourcePeer, rt.RemoteID)
	_, err = tx.Exec(
		`INSERT INTO track(path, source_peer, remote_id, title, artist_id, album_id, track_no, genre, duration, links, added_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, strftime('%s','now'))
		 ON CONFLICT(source_peer, remote_id) WHERE source_peer <> '' DO UPDATE SET
		   title=excluded.title, artist_id=excluded.artist_id, album_id=excluded.album_id,
		   track_no=excluded.track_no, genre=excluded.genre, duration=excluded.duration, links=excluded.links`,
		synthPath, rt.SourcePeer, rt.RemoteID, rt.Title, artistID, albumID, rt.TrackNo, rt.Genre, rt.Duration,
		strings.Join(rt.Links, "\n"),
	)
	if err != nil {
		return 0, err
	}
	var id int64
	if err = tx.QueryRow(
		`SELECT id FROM track WHERE source_peer = ? AND remote_id = ?`, rt.SourcePeer, rt.RemoteID,
	).Scan(&id); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

// DeleteRemoteTracks removes all cached tracks owned by a peer, then prunes
// orphaned albums/artists. Called when a peer's catalog is replaced or it leaves.
func DeleteRemoteTracks(db *sql.DB, peer string) error {
	if _, err := db.Exec(`DELETE FROM track WHERE source_peer = ?`, peer); err != nil {
		return err
	}
	return PruneOrphans(db)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run "TestUpsertRemoteTrack|TestRemote" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/model/model.go internal/store/library.go internal/store/fed_library_test.go
git commit -m "feat(store): remote track upsert + source fields on GetTrack"
```

### Task 4: Resolver seam + audio handler branch

**Files:**
- Create: `internal/fed/resolver.go`
- Modify: `internal/api/audio.go`, `internal/api/server.go`
- Test: `internal/api/audio_fed_test.go` (create)

- [ ] **Step 1: Write the failing test**

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

func TestTrackAudioProxiesRemote(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	id, _ := store.UpsertRemoteTrack(db, store.RemoteTrack{
		SourcePeer: "home", RemoteID: 7, Title: "T", ArtistName: "A", AlbumName: "Al",
	})

	called := ""
	srv := NewServer(db, nil, nil)
	srv.SetFedResolver(fakeResolver(func(w http.ResponseWriter, r *http.Request, peer string, remoteID int64) {
		called = peer
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("remote-bytes"))
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/tracks/1/audio", nil)
	req.SetPathValue("id", itoa(id))
	srv.trackAudio(rec, req)

	if called != "home" {
		t.Fatalf("expected remote resolve for peer 'home', got %q", called)
	}
	if rec.Body.String() != "remote-bytes" {
		t.Fatalf("expected proxied body, got %q", rec.Body.String())
	}
}
```

Add small helpers at the bottom of the test file:

```go
import "strconv"

func itoa(i int64) string { return strconv.FormatInt(i, 10) }

type fakeResolver func(http.ResponseWriter, *http.Request, string, int64)

func (f fakeResolver) ServeRemoteAudio(w http.ResponseWriter, r *http.Request, peer string, remoteID int64) {
	f(w, r, peer, remoteID)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestTrackAudioProxiesRemote -v`
Expected: FAIL — `SetFedResolver` / `ServeRemoteAudio` undefined.

- [ ] **Step 3: Define the Resolver interface and wire the branch**

Create `internal/fed/resolver.go`:

```go
package fed

import "net/http"

// Resolver serves the audio for a track owned by another peer. The api layer
// calls it when a track row carries a non-empty source_peer, keeping all
// networking out of the handlers. Phase 3 supplies the real implementation; a
// nil resolver means "not federated" and remote tracks return 503.
type Resolver interface {
	ServeRemoteAudio(w http.ResponseWriter, r *http.Request, peer string, remoteID int64)
}
```

In `internal/api/server.go`, add a field and setter to `Server`:

```go
	fedResolver fed.Resolver // nil unless federation is configured
```

```go
// SetFedResolver attaches the federation resolver used to proxy audio for
// tracks owned by other peers. Left nil when federation is off.
func (s *Server) SetFedResolver(r fed.Resolver) { s.fedResolver = r }
```

(Add the `"github.com/andybarilla/exit66jukebox/internal/fed"` import.)

In `internal/api/audio.go`, branch after the `GetTrack` lookup:

```go
func (s *Server) trackAudio(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	t, path, ok := store.GetTrack(s.db, id)
	if !ok {
		writeErr(w, http.StatusNotFound, "track not found")
		return
	}
	if t.SourcePeer != "" {
		if s.fedResolver == nil {
			writeErr(w, http.StatusServiceUnavailable, "remote track unavailable")
			return
		}
		s.fedResolver.ServeRemoteAudio(w, r, t.SourcePeer, t.RemoteID)
		return
	}
	http.ServeFile(w, r, path) // sets type + supports Range for <audio> seeking
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/ -run TestTrackAudioProxiesRemote -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/fed/resolver.go internal/api/server.go internal/api/audio.go internal/api/audio_fed_test.go
git commit -m "feat(api): resolve remote-track audio through fed.Resolver seam"
```

---

## Phase 2 — yamux transport

Ships a connected hub↔member session with token auth, reachable both ways over HTTP. No catalog or audio wiring yet; verified by serving a trivial handler over the session in both directions.

### Task 5: Add the yamux dependency

**Files:** `go.mod`, `go.sum`

- [ ] **Step 1: Add the module**

Run: `go get github.com/hashicorp/yamux@latest`
Expected: `go.mod` gains `github.com/hashicorp/yamux`.

- [ ] **Step 2: Verify it builds**

Run: `go build ./...`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "build: add hashicorp/yamux"
```

### Task 6: HTTP-over-session transport adapters

A yamux `*Session` is both a `net.Listener` (via `Accept`) and a stream dialer (via `Open`). This task wraps those so each side can run an `http.Server` over the session and an `http.Client` whose connections are session streams.

**Files:**
- Create: `internal/fed/transport.go`
- Test: `internal/fed/transport_test.go` (create)

- [ ] **Step 1: Write the failing test**

```go
package fed

import (
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/hashicorp/yamux"
)

func TestHTTPOverSession(t *testing.T) {
	c1, c2 := net.Pipe()
	server, err := yamux.Server(c1, nil)
	if err != nil {
		t.Fatal(err)
	}
	client, err := yamux.Client(c2, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Serve a handler over the server session.
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "pong")
	})
	go http.Serve(server, mux)

	// Client makes a request over the client session.
	hc := SessionClient(client)
	resp, err := hc.Get("http://peer/ping")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "pong" {
		t.Fatalf("expected pong, got %q", body)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/fed/ -run TestHTTPOverSession -v`
Expected: FAIL — `SessionClient` undefined.

- [ ] **Step 3: Implement the transport**

Create `internal/fed/transport.go`:

```go
package fed

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/hashicorp/yamux"
)

// SessionClient returns an http.Client whose connections are streams opened on
// the given yamux session. The request URL's host is ignored — every request
// rides the one session — so callers use any placeholder host. Range headers
// and 206 responses pass through unchanged, which is what makes audio seeking
// work across the relay.
func SessionClient(sess *yamux.Session) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return sess.Open()
			},
			// One session multiplexes many streams; disable connection pooling
			// quirks that assume distinct TCP conns.
			DisableKeepAlives:   true,
			MaxIdleConns:        -1,
			IdleConnTimeout:     time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
		},
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/fed/ -run TestHTTPOverSession -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/fed/transport.go internal/fed/transport_test.go
git commit -m "feat(fed): http client over a yamux session"
```

### Task 7: Hub acceptor + member dialer + registry with token auth

The member dials the hub, sends the token as the first line, and the hub validates it before registering the session. After the handshake, both run `http.Serve` over the session and hold a `SessionClient` for outbound calls.

**Files:**
- Create: `internal/fed/session.go`
- Test: `internal/fed/session_test.go` (create)

- [ ] **Step 1: Write the failing test**

```go
package fed

import (
	"net"
	"testing"
	"time"
)

func TestHandshakeAcceptsValidToken(t *testing.T) {
	reg := NewRegistry()
	cConn, sConn := net.Pipe()

	go func() { _ = acceptPeer(sConn, "good-token", reg) }()

	if err := dialHandshake(cConn, "good-token", "home"); err != nil {
		t.Fatalf("handshake failed: %v", err)
	}
	// Registry sees the peer within a moment.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if reg.Get("home") != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("peer 'home' never registered")
}

func TestHandshakeRejectsBadToken(t *testing.T) {
	reg := NewRegistry()
	cConn, sConn := net.Pipe()
	go func() { _ = acceptPeer(sConn, "good-token", reg) }()
	if err := dialHandshake(cConn, "wrong", "home"); err == nil {
		t.Fatal("expected rejection on bad token")
	}
	if reg.Get("home") != nil {
		t.Fatal("bad-token peer must not register")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/fed/ -run TestHandshake -v`
Expected: FAIL — `NewRegistry`/`acceptPeer`/`dialHandshake` undefined.

- [ ] **Step 3: Implement registry + handshake**

Create `internal/fed/session.go`:

```go
package fed

import (
	"bufio"
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/hashicorp/yamux"
)

// Peer is a live federated connection: the multiplexed session plus an http
// client for outbound calls over it.
type Peer struct {
	ID      string
	Session *yamux.Session
	Client  *http.Client
}

// Registry tracks live peer sessions by id. A peer present here is online.
type Registry struct {
	mu    sync.RWMutex
	peers map[string]*Peer
}

func NewRegistry() *Registry { return &Registry{peers: make(map[string]*Peer)} }

func (r *Registry) put(p *Peer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if old := r.peers[p.ID]; old != nil {
		old.Session.Close()
	}
	r.peers[p.ID] = p
}

func (r *Registry) remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.peers, id)
}

// Get returns the live peer for id, or nil if offline.
func (r *Registry) Get(id string) *Peer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.peers[id]
}

// IDs returns the ids of all online peers.
func (r *Registry) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.peers))
	for id := range r.peers {
		out = append(out, id)
	}
	return out
}

// dialHandshake is the member side: send "token peerID\n" on the raw conn, then
// read the single-byte ack the hub sends on success. Returns an error if the hub
// closes without acking (bad token / rejection). The ack round-trip is what lets
// a member detect rejection; without it a write-only handshake can't. yamux.Client
// is created by the caller AFTER this returns, so the ack byte is consumed here
// and never collides with yamux framing.
func dialHandshake(conn net.Conn, token, peerID string) error {
	if _, err := fmt.Fprintf(conn, "%s %s\n", token, peerID); err != nil {
		return err
	}
	var ack [1]byte
	if _, err := conn.Read(ack[:]); err != nil {
		return fmt.Errorf("rejected: %w", err)
	}
	return nil
}

// acceptPeer is the hub side: read the first line, constant-time compare the
// token, then wrap the conn in a yamux server session and register it. Blocks
// until the session ends, then deregisters.
func acceptPeer(conn net.Conn, token string, reg *Registry) error {
	br := bufio.NewReader(conn)
	line, err := br.ReadString('\n')
	if err != nil {
		conn.Close()
		return err
	}
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) != 2 {
		conn.Close()
		return fmt.Errorf("malformed handshake")
	}
	gotToken, peerID := fields[0], fields[1]
	if subtle.ConstantTimeCompare([]byte(gotToken), []byte(token)) != 1 {
		conn.Close()
		return fmt.Errorf("bad token")
	}
	// Any bytes the member buffered after the line belong to the session. net.Pipe
	// in tests writes exactly the line, so br has no surplus; in production the
	// session's first frames may already be buffered, so hand br to yamux via a
	// small wrapper that drains the bufio buffer first.
	sess, err := yamux.Server(&bufferedConn{Reader: br, Conn: conn}, nil)
	if err != nil {
		conn.Close()
		return err
	}
	p := &Peer{ID: peerID, Session: sess, Client: SessionClient(sess)}
	reg.put(p)
	defer reg.remove(peerID)
	<-sess.CloseChan() // block until session dies
	return nil
}

// bufferedConn lets yamux read through a bufio.Reader that may already hold
// bytes from the handshake read, while writes go straight to the conn.
type bufferedConn struct {
	*bufio.Reader
	net.Conn
}

func (b *bufferedConn) Read(p []byte) (int, error) { return b.Reader.Read(p) }
```

> Note: `bufferedConn` embeds both `*bufio.Reader` and `net.Conn`, which both have `Read`; the explicit `Read` method resolves the ambiguity in favor of the buffered reader. All other `net.Conn` methods promote from the embedded `Conn`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/fed/ -run TestHandshake -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/fed/session.go internal/fed/session_test.go
git commit -m "feat(fed): peer registry + token handshake over yamux"
```

### Task 8: Manager — dial loop (member) and listener (hub)

**Files:**
- Create: `internal/fed/manager.go`
- Test: `internal/fed/manager_test.go` (create)

- [ ] **Step 1: Write the failing test**

```go
package fed

import (
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestManagerMemberServesHubRequests(t *testing.T) {
	reg := NewRegistry()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	// Hub: accept one peer.
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		_ = acceptPeer(conn, "tok", reg)
	}()

	// Member: dial hub, serve a handler over the session.
	mux := http.NewServeMux()
	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "from-member")
	})
	m := &Manager{Token: "tok", PeerID: "home", HubAddr: ln.Addr().String(), MemberHandler: mux}
	go m.runMember()

	// Wait for registration, then the hub calls the member.
	var peer *Peer
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if peer = reg.Get("home"); peer != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if peer == nil {
		t.Fatal("member never registered with hub")
	}
	resp, err := peer.Client.Get("http://home/hello")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "from-member" {
		t.Fatalf("expected from-member, got %q", body)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/fed/ -run TestManagerMember -v`
Expected: FAIL — `Manager` undefined.

- [ ] **Step 3: Implement the Manager**

Create `internal/fed/manager.go`:

```go
package fed

import (
	"log"
	"net"
	"net/http"
	"time"

	"github.com/hashicorp/yamux"
)

// Manager runs federation for one instance. Role "hub" listens for members;
// role "member" dials a hub and keeps the connection alive. MemberHandler is
// the http.Handler served back to the hub over the session (the member's audio
// endpoints); HubHandler is served to members (the relay endpoints).
type Manager struct {
	Role          string
	Token         string
	PeerID        string
	HubAddr       string       // member only
	HubListen     string       // hub only, e.g. ":8443"
	MemberHandler http.Handler // served over session, member side
	HubHandler    http.Handler // served over session, hub side
	Registry      *Registry

	hubSession *yamux.Session // member side: the live session to the hub
}

// Start launches the role's networking in background goroutines.
func (m *Manager) Start() {
	if m.Registry == nil {
		m.Registry = NewRegistry()
	}
	switch m.Role {
	case "hub":
		go m.runHub()
	case "member":
		go m.runMember()
	}
}

func (m *Manager) runHub() {
	ln, err := net.Listen("tcp", m.HubListen)
	if err != nil {
		log.Printf("fed hub listen %s: %v", m.HubListen, err)
		return
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go func() {
			// acceptPeer registers then blocks until the session dies. Serve the
			// hub handler over the same session for member-initiated requests.
			// We need the session before serving, so inline a variant:
			m.serveHubConn(conn)
		}()
	}
}

// serveHubConn performs the handshake, registers the peer, and serves HubHandler
// over the session until it closes.
func (m *Manager) serveHubConn(conn net.Conn) {
	// Reuse acceptPeer's handshake by running it and grabbing the session from the
	// registry once it appears would race; instead replicate the handshake inline.
	p, err := acceptAndRegister(conn, m.Token, m.Registry)
	if err != nil {
		return
	}
	defer m.Registry.remove(p.ID)
	if m.HubHandler != nil {
		_ = http.Serve(p.Session, m.HubHandler)
	} else {
		<-p.Session.CloseChan()
	}
}

func (m *Manager) runMember() {
	backoff := time.Second
	for {
		conn, err := net.Dial("tcp", m.HubAddr)
		if err != nil {
			time.Sleep(backoff)
			backoff = min(backoff*2, 30*time.Second)
			continue
		}
		if err := dialHandshake(conn, m.Token, m.PeerID); err != nil {
			conn.Close()
			time.Sleep(backoff)
			continue
		}
		sess, err := yamux.Client(conn, nil)
		if err != nil {
			conn.Close()
			time.Sleep(backoff)
			continue
		}
		m.hubSession = sess
		// register the hub as a pseudo-peer so the member can call it by client.
		m.Registry.put(&Peer{ID: "@hub", Session: sess, Client: SessionClient(sess)})
		backoff = time.Second
		if m.MemberHandler != nil {
			_ = http.Serve(sess, m.MemberHandler) // blocks until session closes
		} else {
			<-sess.CloseChan()
		}
		m.Registry.remove("@hub")
		sess.Close()
	}
}

// HubClient returns the http client to the hub (member side), or nil if not
// connected.
func (m *Manager) HubClient() *http.Client {
	if p := m.Registry.Get("@hub"); p != nil {
		return p.Client
	}
	return nil
}
```

Refactor `session.go`: split `acceptPeer` so the handshake+registration is reusable. Replace the body of `acceptPeer` with a thin wrapper and add `acceptAndRegister`:

```go
// acceptAndRegister performs the handshake, builds the session, and registers
// the peer. The caller is responsible for serving over and tearing down the
// session.
func acceptAndRegister(conn net.Conn, token string, reg *Registry) (*Peer, error) {
	br := bufio.NewReader(conn)
	line, err := br.ReadString('\n')
	if err != nil {
		conn.Close()
		return nil, err
	}
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) != 2 {
		conn.Close()
		return nil, fmt.Errorf("malformed handshake")
	}
	gotToken, peerID := fields[0], fields[1]
	if subtle.ConstantTimeCompare([]byte(gotToken), []byte(token)) != 1 {
		conn.Close()
		return nil, fmt.Errorf("bad token")
	}
	// Single-byte ack before yamux handoff so the member can detect acceptance
	// vs. rejection without stealing yamux framing bytes (see dialHandshake).
	if _, err := conn.Write([]byte{1}); err != nil {
		conn.Close()
		return nil, err
	}
	sess, err := yamux.Server(&bufferedConn{Reader: br, Conn: conn}, nil)
	if err != nil {
		conn.Close()
		return nil, err
	}
	p := &Peer{ID: peerID, Session: sess, Client: SessionClient(sess)}
	reg.put(p)
	return p, nil
}

// acceptPeer is the test-friendly form: register then block until the session
// dies. Production code uses acceptAndRegister + http.Serve.
func acceptPeer(conn net.Conn, token string, reg *Registry) error {
	p, err := acceptAndRegister(conn, token, reg)
	if err != nil {
		return err
	}
	defer reg.remove(p.ID)
	<-p.Session.CloseChan()
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/fed/ -v`
Expected: PASS (all fed tests).

- [ ] **Step 5: Commit**

```bash
git add internal/fed/manager.go internal/fed/session.go internal/fed/manager_test.go
git commit -m "feat(fed): manager dial loop (member) + listener (hub)"
```

> **TLS (spec requirement, apply before any non-loopback deployment):** the connection carries catalogs and audio over the public internet, so the hub's listener must be TLS. Wrap `runHub`'s `net.Listen` with `tls.NewListener(ln, tlsConfig)` and `runMember`'s `net.Dial` with `tls.Dial("tcp", m.HubAddr, tlsConfig)`. Source the hub cert/key from config (add `EXIT66_FED_TLS_CERT`/`EXIT66_FED_TLS_KEY`); members verify against the hub's hostname. The token still authenticates the *member to the hub* on top of TLS. Loopback tests in this plan use plain `net.Pipe`/`net.Dial` and are unaffected. If you instead terminate TLS at a reverse proxy in front of the VPS, the listener stays plain and this note is moot — decide per deployment.

---

## Phase 3 — Audio relay end-to-end

After this phase the original use case works: the hub plays a track whose file lives on a member, with `Range`/seek intact. Implemented as the real `Resolver`.

### Task 9: Hub relay handler

The hub serves `GET /fed/audio/{peer}/{id}` (over the session, to members) by reverse-proxying to the owning peer's existing `/api/tracks/{id}/audio`.

**Files:**
- Create: `internal/fed/relay.go`
- Test: `internal/fed/relay_test.go` (create)

- [ ] **Step 1: Write the failing test**

```go
package fed

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHubRelayForwardsToOwner(t *testing.T) {
	reg := NewRegistry()

	// The hub listens; the owner member "home" dials in and serves its audio
	// endpoint over the session. The relay runs on the hub side against reg.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		p, err := acceptAndRegister(conn, "tok", reg)
		if err != nil {
			return
		}
		http.Serve(p.Session, nil) // member-initiated requests unused in this test
	}()

	ownerMux := http.NewServeMux()
	ownerMux.HandleFunc("/api/tracks/{id}/audio", func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "x.mp3", time.Time{}, strings.NewReader("0123456789"))
	})
	home := &Manager{Role: "member", Token: "tok", PeerID: "home",
		HubAddr: ln.Addr().String(), MemberHandler: ownerMux, Registry: NewRegistry()}
	home.Start()

	// Wait for home to register on the hub, then relay a ranged request to it.
	for i := 0; i < 300 && reg.Get("home") == nil; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if reg.Get("home") == nil {
		t.Fatal("owner 'home' never registered")
	}

	relay := NewRelay(reg, nil) // nil db: this test exercises audio relay only
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/fed/audio/home/42", nil)
	req.SetPathValue("peer", "home")
	req.SetPathValue("id", "42")
	req.Header.Set("Range", "bytes=2-5")
	relay.ServeHTTP(rec, req)

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("expected 206, got %d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if string(body) != "2345" {
		t.Fatalf("expected range bytes 2345, got %q", body)
	}
}
```

Imports for this test file: `io`, `net`, `net/http`, `net/http/httptest`, `strings`, `testing`, `time`.

> `relay.ServeHTTP` reads its peer/id from `r.PathValue`, so the test sets them explicitly with `SetPathValue` rather than relying on a mux. `NewRelay`'s second arg is the hub's DB (Task 14); pass `nil` here since this test only covers audio relay.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/fed/ -run TestHubRelay -v`
Expected: FAIL — `NewRelay` undefined.

- [ ] **Step 3: Implement the relay**

Create `internal/fed/relay.go`:

```go
package fed

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
)

// Relay is the hub-side handler. It reverse-proxies GET /fed/audio/{peer}/{id}
// to the owning peer's /api/tracks/{id}/audio over that peer's session
// (forwarding Range, copying 206 + body back), and in Phase 4 also ingests and
// fans out catalogs. db is the hub's own database — the hub is a peer too, so
// received catalogs are applied to it (Task 14). db may be nil in tests that
// exercise only audio relay.
type Relay struct {
	reg *Registry
	db  *sql.DB
}

func NewRelay(reg *Registry, db *sql.DB) *Relay { return &Relay{reg: reg, db: db} }

func (h *Relay) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	peerID := r.PathValue("peer")
	remoteID := r.PathValue("id")
	if peerID == "" || remoteID == "" {
		http.Error(w, "bad fed audio path", http.StatusBadRequest)
		return
	}
	peer := h.reg.Get(peerID)
	if peer == nil {
		http.Error(w, "peer offline", http.StatusServiceUnavailable)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet,
		fmt.Sprintf("http://%s/api/tracks/%s/audio", peerID, remoteID), nil)
	if err != nil {
		http.Error(w, "build request", http.StatusInternalServerError)
		return
	}
	if rng := r.Header.Get("Range"); rng != "" {
		req.Header.Set("Range", rng)
	}
	resp, err := peer.Client.Do(req)
	if err != nil {
		http.Error(w, "peer fetch failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for _, k := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
		if v := resp.Header.Get(k); v != "" {
			w.Header().Set(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// Routes returns the hub's federation mux (served over the session to members).
func (h *Relay) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /fed/audio/{peer}/{id}", h)
	return mux
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/fed/ -run TestHubRelay -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/fed/relay.go internal/fed/relay_test.go
git commit -m "feat(fed): hub relay reverse-proxies remote audio with Range"
```

### Task 10: Member-side Resolver (loopback into hub session)

The member's `Resolver.ServeRemoteAudio` reverse-proxies to its hub: `GET http://@hub/fed/audio/{peer}/{id}`. This is the implementation the api layer's seam (Task 4) calls.

**Files:**
- Modify: `internal/fed/relay.go`
- Test: `internal/fed/resolver_impl_test.go` (create)

- [ ] **Step 1: Write the failing test**

```go
package fed

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMemberResolverReachesHubRelay(t *testing.T) {
	reg := NewRegistry()
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()

	// Hub: accept members, serve the relay over their sessions.
	relay := NewRelay(reg)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				p, err := acceptAndRegister(conn, "tok", reg)
				if err != nil {
					return
				}
				http.Serve(p.Session, relay.Routes())
			}()
		}
	}()

	// Owner member "home" serves audio.
	ownerMux := http.NewServeMux()
	ownerMux.HandleFunc("/api/tracks/{id}/audio", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "AUDIO")
	})
	home := &Manager{Role: "member", Token: "tok", PeerID: "home", HubAddr: ln.Addr().String(), MemberHandler: ownerMux, Registry: NewRegistry()}
	home.Start()

	// Playing member "vps" connects too; its resolver fetches home's track.
	vps := &Manager{Role: "member", Token: "tok", PeerID: "vps", HubAddr: ln.Addr().String(), Registry: NewRegistry()}
	vps.Start()

	for i := 0; i < 300 && (reg.Get("home") == nil || vps.HubClient() == nil); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	res := NewResolverFor(vps)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/tracks/1/audio", nil)
	res.ServeRemoteAudio(rec, req, "home", 99)

	if rec.Body.String() != "AUDIO" {
		t.Fatalf("expected AUDIO via relay, got %q (code %d)", rec.Body.String(), rec.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/fed/ -run TestMemberResolver -v`
Expected: FAIL — `NewResolverFor` undefined.

- [ ] **Step 3: Implement the member resolver**

Add to `internal/fed/relay.go`:

```go
// memberResolver implements api/fed.Resolver on the member side by proxying to
// the hub's relay over the hub session.
type memberResolver struct{ m *Manager }

// NewResolverFor returns a Resolver that routes remote audio through the manager's
// hub session.
func NewResolverFor(m *Manager) Resolver { return &memberResolver{m: m} }

func (mr *memberResolver) ServeRemoteAudio(w http.ResponseWriter, r *http.Request, peer string, remoteID int64) {
	hc := mr.m.HubClient()
	if hc == nil {
		http.Error(w, "hub offline", http.StatusServiceUnavailable)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet,
		fmt.Sprintf("http://@hub/fed/audio/%s/%d", peer, remoteID), nil)
	if err != nil {
		http.Error(w, "build request", http.StatusInternalServerError)
		return
	}
	if rng := r.Header.Get("Range"); rng != "" {
		req.Header.Set("Range", rng)
	}
	resp, err := hc.Do(req)
	if err != nil {
		http.Error(w, "hub fetch failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for _, k := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
		if v := resp.Header.Get(k); v != "" {
			w.Header().Set(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
```

> The hub also plays remote tracks (it's a peer too). For the hub role, `ServeRemoteAudio` calls `Relay.ServeHTTP` directly instead of going through a hub session. Add a `hubResolver{relay *Relay}` whose `ServeRemoteAudio` builds a request with `SetPathValue("peer", peer)` / `("id", ...)` and calls `relay.ServeHTTP`. `NewResolverFor` returns the right one based on `m.Role`.

Implement that branch:

```go
type hubResolver struct{ relay *Relay }

func (hr *hubResolver) ServeRemoteAudio(w http.ResponseWriter, r *http.Request, peer string, remoteID int64) {
	req := r.Clone(r.Context())
	req.SetPathValue("peer", peer)
	req.SetPathValue("id", fmt.Sprintf("%d", remoteID))
	hr.relay.ServeHTTP(w, req)
}
```

And update `NewResolverFor` to reuse the manager's single `Relay` (add a `Relay *Relay` field to `Manager`; `main.go` sets it once so the resolver and `HubHandler` share one instance — same registry *and* same catalog cache/DB):

```go
func NewResolverFor(m *Manager) Resolver {
	if m.Role == "hub" {
		return &hubResolver{relay: m.Relay}
	}
	return &memberResolver{m: m}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/fed/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/fed/relay.go internal/fed/resolver_impl_test.go
git commit -m "feat(fed): member + hub resolvers for remote audio"
```

### Task 11: ffmpeg source accepts a URL for shared streams

The shared-stream source currently opens a local path. For a remote track the source must read the loopback fed URL so ffmpeg pulls bytes through the relay.

**Files:**
- Modify: `internal/broadcast/source.go`, and the caller that picks the path (find with `grep -rn "FFmpegSource\|\.Open(" internal/`)
- Test: `internal/broadcast/source_test.go` (create or extend)

Both local and remote tracks resolve through the instance's own `/api/tracks/{id}/audio` (Task 4 already branches there), so the source never needs to know which is which — it always points ffmpeg at the loopback audio URL for the track's **local** id. This removes any peer/remote_id args from the source.

- [ ] **Step 1: Write the failing test**

```go
package broadcast

import "testing"

func TestSourceInputIsLoopbackAudioURL(t *testing.T) {
	got := sourceInput("http://127.0.0.1:8066", 42)
	want := "http://127.0.0.1:8066/api/tracks/42/audio"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/broadcast/ -run TestSourceInput -v`
Expected: FAIL — `sourceInput` undefined.

- [ ] **Step 3: Implement sourceInput and use it in Open**

```go
// sourceInput returns the ffmpeg -i argument for a track. Local and remote
// tracks both resolve through the instance's own audio endpoint, so the source
// never needs to know which is which — it always points ffmpeg at the loopback
// audio URL for the track's local id.
func sourceInput(selfBaseURL string, localTrackID int64) string {
	return fmt.Sprintf("%s/api/tracks/%d/audio", selfBaseURL, localTrackID)
}
```

In `FFmpegSource.Open`, change the input from the file path to this URL. Thread `selfBaseURL` (e.g. `http://127.0.0.1<listenAddr>`) and the track's local id from the hub/jukebox caller. Update `FFmpegSource.Open(path string)` to `Open(input string)` where `input` is already the resolved URL, and have the caller compute it via `sourceInput`. Verify ffmpeg's `-re -i <url>` works against a local HTTP server (it does; ffmpeg supports HTTP input).

> This is a behavioral change for *local* shared-stream playback too (now via HTTP loopback rather than file path). Confirm the shared stream still plays a local track after this change with a manual run (Phase 7 verification) — the trade-off buys one uniform source path.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/broadcast/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/broadcast/source.go internal/broadcast/source_test.go
git commit -m "feat(broadcast): shared-stream source reads via loopback audio URL"
```

### Task 12: Wire the manager + resolver into the server and main

**Files:**
- Modify: `internal/api/server.go` (route registration), `main.go`
- Test: manual (Phase 7) + `go build`

- [ ] **Step 1: Register the hub relay route on the public API mux**

In `internal/api/server.go` `Handler()`, when federation role is hub, also expose the relay over the **public** listener is NOT wanted (members reach it over the session). The relay is served over the session by the manager (Task 10 wiring). So in `Handler()` add nothing public for the relay. Confirm `Handler()` is unchanged except it already serves `/api/tracks/{id}/audio` which both local clients and the session use.

> Key point: the member serves its *whole* API handler over the session (so the hub's relay can hit `/api/tracks/{id}/audio`). Set `Manager.MemberHandler = server.Handler()` and `Manager.HubHandler = relay.Routes()` in `main.go`.

- [ ] **Step 2: Build the manager in main.go**

In `main.go`, after the server is constructed and before `ListenAndServe`, add:

```go
	if cfg.Federation.Enabled() {
		reg := fed.NewRegistry()
		fm := &fed.Manager{
			Role:          cfg.Federation.Role,
			Token:         cfg.Federation.Token,
			PeerID:        cfg.Federation.PeerID,
			HubAddr:       cfg.Federation.HubAddr, // member: hub to dial
			HubListen:     cfg.Federation.Listen,  // hub: local listen addr
			MemberHandler: srv.Handler(),
			Registry:      reg,
			DB:            db,
		}
		if cfg.Federation.Role == "hub" {
			// One relay instance: it holds the registry, the catalog cache, AND the
			// hub's DB, and is shared by both the session handler and the resolver
			// so received catalogs land in the hub's own browse.
			relay := fed.NewRelay(reg, db)
			fm.Relay = relay
			fm.HubHandler = relay.Routes()
		}
		fm.Start()
		srv.SetFedResolver(fed.NewResolverFor(fm))
		srv.SetFedPeers(fm.OnlinePeers)
		log.Printf("federation: role=%s peer=%s", cfg.Federation.Role, cfg.Federation.PeerID)
	}
```

> This uses a dedicated `Federation.Listen` field (hub's listen address) distinct from `HubAddr` (the address a member dials). Add it in Task 1: `Listen string` on the `Federation` struct, sourced from `EXIT66_FED_LISTEN`. Update Task 1's test to set and assert it.

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: success.

- [ ] **Step 4: Run the full test suite**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/api/server.go main.go
git commit -m "feat: wire federation manager + resolver into server/main"
```

---

## Phase 4 — Catalog sync

Members push their catalog up; the hub merges and fans out; members cache remote rows. After this phase, remote tracks appear in browse.

### Task 13: Catalog snapshot serialization

**Files:**
- Create: `internal/store/fed_export.go`
- Test: `internal/store/fed_export_test.go` (create)

- [ ] **Step 1: Write the failing test**

```go
package store

import "testing"

func TestExportCatalogReturnsLocalTracks(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	_, err := UpsertTrack(db, model.Track{Path: "/m/a.mp3", Title: "A", Duration: 100, TrackNo: 1},
		"Artist", "Artist", "Album")
	if err != nil {
		t.Fatal(err)
	}
	// A remote row must NOT be re-exported (we only share our own files).
	_, _ = UpsertRemoteTrack(db, RemoteTrack{SourcePeer: "x", RemoteID: 1, Title: "R", ArtistName: "Q", AlbumName: "Z"})

	rows, err := ExportCatalog(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 local track exported, got %d", len(rows))
	}
	if rows[0].Title != "A" || rows[0].RemoteID != 1 || rows[0].ArtistName != "Artist" {
		t.Fatalf("unexpected export row: %+v", rows[0])
	}
}
```

(`RemoteID` in an export row is the *local* track id on this peer — what other peers will store as `remote_id`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestExportCatalog -v`
Expected: FAIL — `ExportCatalog` undefined.

- [ ] **Step 3: Implement ExportCatalog**

Create `internal/store/fed_export.go`:

```go
package store

import (
	"database/sql"
	"strings"
)

// CatalogRow is one local track flattened with the artist/album names a peer
// needs to upsert it. RemoteID is this peer's local track id — the receiving
// peer stores it as remote_id.
type CatalogRow struct {
	RemoteID    int64    `json:"remote_id"`
	Title       string   `json:"title"`
	ArtistName  string   `json:"artist"`
	AlbumArtist string   `json:"album_artist"`
	AlbumName   string   `json:"album"`
	TrackNo     int      `json:"track_no"`
	Genre       string   `json:"genre"`
	Duration    int      `json:"duration"`
	Links       []string `json:"links,omitempty"`
}

// ExportCatalog returns all local (non-remote) tracks as flattened rows for
// catalog sync. Remote rows are excluded — a peer only shares its own files.
func ExportCatalog(db *sql.DB) ([]CatalogRow, error) {
	rows, err := db.Query(
		`SELECT t.id, t.title, ta.name, aa.name, al.name, t.track_no, t.genre, t.duration, t.links
		 FROM track t
		 JOIN artist ta ON ta.id = t.artist_id
		 JOIN album  al ON al.id = t.album_id
		 JOIN artist aa ON aa.id = al.artist_id
		 WHERE t.source_peer = ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CatalogRow
	for rows.Next() {
		var c CatalogRow
		var links string
		if err := rows.Scan(&c.RemoteID, &c.Title, &c.ArtistName, &c.AlbumArtist,
			&c.AlbumName, &c.TrackNo, &c.Genre, &c.Duration, &links); err != nil {
			return nil, err
		}
		if links != "" {
			c.Links = strings.Split(links, "\n")
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestExportCatalog -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/fed_export.go internal/store/fed_export_test.go
git commit -m "feat(store): export local catalog rows for sync"
```

### Task 14: Sync protocol — member push + hub apply

The member POSTs its catalog to the hub (`POST /fed/catalog/{peer}`); the hub stores it under that peer and re-fans the union to other members. Members apply received catalogs via `UpsertRemoteTrack` + `DeleteRemoteTracks` for the replaced peer.

**Files:**
- Create: `internal/fed/sync.go`
- Test: `internal/fed/sync_test.go` (create)

- [ ] **Step 1: Write the failing test**

```go
package fed

import (
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

func TestApplyCatalogReplacesPeerRows(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()

	rows := []store.CatalogRow{
		{RemoteID: 1, Title: "One", ArtistName: "A", AlbumName: "Al", TrackNo: 1, Duration: 10},
		{RemoteID: 2, Title: "Two", ArtistName: "A", AlbumName: "Al", TrackNo: 2, Duration: 20},
	}
	if err := ApplyCatalog(db, "home", rows); err != nil {
		t.Fatal(err)
	}
	got, _ := store.ListTracks(db, "", 0, 0)
	if len(got) != 2 {
		t.Fatalf("expected 2 remote tracks, got %d", len(got))
	}

	// A second, smaller catalog for the same peer replaces the first.
	if err := ApplyCatalog(db, "home", rows[:1]); err != nil {
		t.Fatal(err)
	}
	got, _ = store.ListTracks(db, "", 0, 0)
	if len(got) != 1 {
		t.Fatalf("expected 1 track after replace, got %d", len(got))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/fed/ -run TestApplyCatalog -v`
Expected: FAIL — `ApplyCatalog` undefined.

- [ ] **Step 3: Implement ApplyCatalog and the sync handlers**

Create `internal/fed/sync.go`:

```go
package fed

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

// ApplyCatalog replaces all cached rows for peer with the given catalog rows,
// in one pass: delete the peer's existing rows, then upsert the new set.
func ApplyCatalog(db *sql.DB, peer string, rows []store.CatalogRow) error {
	if err := store.DeleteRemoteTracks(db, peer); err != nil {
		return err
	}
	for _, c := range rows {
		if _, err := store.UpsertRemoteTrack(db, store.RemoteTrack{
			SourcePeer: peer, RemoteID: c.RemoteID,
			Title: c.Title, ArtistName: c.ArtistName, AlbumArtist: c.AlbumArtist,
			AlbumName: c.AlbumName, TrackNo: c.TrackNo, Genre: c.Genre,
			Duration: c.Duration, Links: c.Links,
		}); err != nil {
			return err
		}
	}
	return nil
}

// PushCatalog (member side) exports the local catalog and POSTs it to the hub.
func PushCatalog(db *sql.DB, hubClient *http.Client, peerID string) error {
	rows, err := store.ExportCatalog(db)
	if err != nil {
		return err
	}
	body, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	req, _ := http.NewRequest(http.MethodPost, "http://@hub/fed/catalog/"+peerID, bytesReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := hubClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
```

Add `bytesReader` (or just use `bytes.NewReader`; import `bytes`).

Add hub-side handlers to `Relay.Routes()` in `relay.go`. Two things happen on receive: the rows are cached for fan-out to *other* members, **and** applied to the hub's own DB — the hub is a peer too, so its browse must show remote tracks (this is the headline VPS-shows-home-library scenario). Add the catalog cache + a mutex to the existing `Relay` struct (which already holds `reg` and `db` from Task 9):

```go
// Extend the Relay struct from Task 9 with a catalog cache:
//   reg      *Registry
//   db       *sql.DB
//   mu       sync.Mutex
//   catalogs map[string][]store.CatalogRow  // peer -> its rows
// and initialize catalogs in NewRelay:
//   return &Relay{reg: reg, db: db, catalogs: make(map[string][]store.CatalogRow)}

func (h *Relay) receiveCatalog(w http.ResponseWriter, r *http.Request) {
	peer := r.PathValue("peer")
	var rows []store.CatalogRow
	if err := json.NewDecoder(r.Body).Decode(&rows); err != nil {
		http.Error(w, "bad catalog", http.StatusBadRequest)
		return
	}
	h.mu.Lock()
	h.catalogs[peer] = rows
	h.mu.Unlock()
	// The hub is a peer too: apply to its own DB so its browse shows remote tracks.
	if h.db != nil {
		if err := ApplyCatalog(h.db, peer, rows); err != nil {
			http.Error(w, "apply catalog", http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// MergedCatalog is the fan-out payload: every peer's rows except the requester's
// own, plus the ids of all currently-online peers so members can grey out the
// offline ones (a member's local registry only knows the hub, not its siblings).
type MergedCatalog struct {
	Catalogs map[string][]store.CatalogRow `json:"catalogs"`
	Online   []string                      `json:"online"`
}

func (h *Relay) serveMerged(w http.ResponseWriter, r *http.Request) {
	exclude := r.PathValue("peer")
	h.mu.Lock()
	cats := make(map[string][]store.CatalogRow)
	for peer, rows := range h.catalogs {
		if peer != exclude {
			cats[peer] = rows
		}
	}
	h.mu.Unlock()
	json.NewEncoder(w).Encode(MergedCatalog{Catalogs: cats, Online: h.reg.IDs()})
}
```

Register in `Routes()` (add `"sync"`, `"encoding/json"`, and the store import to `relay.go`):

```go
	mux.HandleFunc("POST /fed/catalog/{peer}", h.receiveCatalog)
	mux.HandleFunc("GET /fed/catalog/{peer}/merged", h.serveMerged)
```

> Add a test asserting the **hub's own DB** gains a member's track after a push — not just a downstream member's DB:
>
> ```go
> func TestReceiveCatalogAppliesToHubDB(t *testing.T) {
> 	hubDB, _ := store.Open(":memory:")
> 	defer hubDB.Close()
> 	relay := NewRelay(NewRegistry(), hubDB)
> 	body, _ := json.Marshal([]store.CatalogRow{{RemoteID: 1, Title: "Hit", ArtistName: "A", AlbumName: "Al"}})
> 	rec := httptest.NewRecorder()
> 	req := httptest.NewRequest("POST", "/fed/catalog/home", bytes.NewReader(body))
> 	req.SetPathValue("peer", "home")
> 	relay.receiveCatalog(rec, req)
> 	got, _ := store.ListTracks(hubDB, "", 0, 0)
> 	if len(got) != 1 || got[0].Title != "Hit" {
> 		t.Fatalf("hub DB should hold the member's track, got %+v", got)
> 	}
> }
> ```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/fed/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/fed/sync.go internal/fed/relay.go internal/fed/sync_test.go
git commit -m "feat(fed): catalog push, hub store/merge, member apply"
```

### Task 15: Sync loop — push on scan, pull merged periodically

**Files:**
- Modify: `internal/fed/manager.go`, `main.go`
- Test: `internal/fed/sync_loop_test.go` (create) — integration over loopback

- [ ] **Step 1: Write the failing integration test**

```go
package fed

import (
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

func TestEndToEndCatalogSync(t *testing.T) {
	hubDB, _ := store.Open(":memory:")
	defer hubDB.Close()
	memDB, _ := store.Open(":memory:")
	defer memDB.Close()
	store.UpsertTrack(memDB, modelTrack("/m/x.mp3", "Hit", 1), "Band", "Band", "Rec")

	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	reg := NewRegistry()
	relay := NewRelay(reg)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				p, err := acceptAndRegister(conn, "tok", reg)
				if err != nil {
					return
				}
				http.Serve(p.Session, relay.Routes())
			}()
		}
	}()

	mem := &Manager{Role: "member", Token: "tok", PeerID: "home", HubAddr: ln.Addr().String(), Registry: NewRegistry()}
	mem.Start()
	for i := 0; i < 300 && mem.HubClient() == nil; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if err := PushCatalog(memDB, mem.HubClient(), "home"); err != nil {
		t.Fatal(err)
	}

	// A second member pulls the merged catalog and applies it.
	other := &Manager{Role: "member", Token: "tok", PeerID: "vps", HubAddr: ln.Addr().String(), Registry: NewRegistry()}
	other.Start()
	for i := 0; i < 300 && other.HubClient() == nil; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if err := PullAndApply(hubDB, other.HubClient(), "vps"); err != nil {
		t.Fatal(err)
	}
	got, _ := store.ListTracks(hubDB, "", 0, 0)
	if len(got) != 1 || got[0].Title != "Hit" {
		t.Fatalf("expected home's track synced to vps db, got %+v", got)
	}
}
```

Add a `modelTrack` helper to the test file building a `model.Track`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/fed/ -run TestEndToEndCatalogSync -v`
Expected: FAIL — `PullAndApply` undefined.

- [ ] **Step 3: Implement PullAndApply and the manager sync loop**

Add to `internal/fed/sync.go`:

```go
// PullAndApply (member side) fetches the merged catalog from the hub, applies
// each peer's rows into the local DB, and returns the hub-reported list of
// online peers so the caller can update liveness.
func PullAndApply(db *sql.DB, hubClient *http.Client, peerID string) ([]string, error) {
	resp, err := hubClient.Get("http://@hub/fed/catalog/" + peerID + "/merged")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var merged MergedCatalog
	if err := json.NewDecoder(resp.Body).Decode(&merged); err != nil {
		return nil, err
	}
	for peer, rows := range merged.Catalogs {
		if err := ApplyCatalog(db, peer, rows); err != nil {
			return nil, err
		}
	}
	return merged.Online, nil
}
```

The end-to-end test (Step 1) must capture the new return value: change
`if err := PullAndApply(hubDB, other.HubClient(), "vps"); err != nil {` to
`if _, err := PullAndApply(hubDB, other.HubClient(), "vps"); err != nil {`.

In `manager.go`, add `DB *sql.DB`, `mu sync.Mutex`, and `online []string` fields to `Manager`, plus a member sync loop started from `runMember` after a successful connect:

```go
// startSyncLoop (member) pushes the local catalog once, then pulls the merged
// catalog on a ticker, recording the hub-reported online peers each pull. Runs
// until the session closes.
func (m *Manager) startSyncLoop(done <-chan struct{}) {
	if m.DB == nil {
		return
	}
	if hc := m.HubClient(); hc != nil {
		_ = PushCatalog(m.DB, hc, m.PeerID)
		if online, err := PullAndApply(m.DB, hc, m.PeerID); err == nil {
			m.setOnline(online)
		}
	}
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			if hc := m.HubClient(); hc != nil {
				if online, err := PullAndApply(m.DB, hc, m.PeerID); err == nil {
					m.setOnline(online)
				}
			}
		}
	}
}

func (m *Manager) setOnline(ids []string) {
	m.mu.Lock()
	m.online = ids
	m.mu.Unlock()
}

// OnlinePeers returns the ids of peers currently online. The hub knows this
// directly from its registry; a member uses the list the hub last reported,
// since a member's own registry only contains the hub.
func (m *Manager) OnlinePeers() []string {
	if m.Role == "hub" {
		return m.Registry.IDs()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.online...)
}
```

Call `go m.startSyncLoop(sess.CloseChan())` in `runMember` right after `m.Registry.put(... "@hub" ...)`. Wire `DB` in `main.go`'s manager construction (`DB: db`). Also push immediately after a scan completes: in the scan-completion path (find via `grep -rn "PruneOrphans\|scan complete" internal/`), if a fed manager exists and is a member, call `PushCatalog`. Expose a `Manager.PushNow()` helper that the scan path can call:

```go
// PushNow re-exports and pushes the local catalog if connected as a member.
func (m *Manager) PushNow() {
	if m.Role != "member" || m.DB == nil {
		return
	}
	if hc := m.HubClient(); hc != nil {
		_ = PushCatalog(m.DB, hc, m.PeerID)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/fed/ -v && go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/fed/sync.go internal/fed/manager.go main.go internal/fed/sync_loop_test.go
git commit -m "feat(fed): member sync loop — push on connect/scan, pull merged on ticker"
```

---

## Phase 5 — UI: owner + availability

Remote tracks show their owning peer; offline peers' tracks grey out. Minimal, since the merged library already renders through existing components.

### Task 16: Expose source_peer + online peers to the client

**Files:**
- Modify: `internal/api/browse.go` (or wherever `EnrichedTrack` is built — find with `grep -rn "EnrichedTrack" internal/api/`), `internal/api/config.go`
- Test: `internal/api/browse_test.go` (extend)

- [ ] **Step 1: Write the failing test**

```go
func TestBrowseExposesSourcePeer(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	store.UpsertRemoteTrack(db, store.RemoteTrack{SourcePeer: "home", RemoteID: 1, Title: "R", ArtistName: "A", AlbumName: "Al"})
	srv := NewServer(db, nil, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/tracks", nil)
	srv.Handler().ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), `"source_peer":"home"`) {
		t.Fatalf("expected source_peer in track JSON, got %s", rec.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestBrowseExposesSourcePeer -v`
Expected: FAIL — `source_peer` absent (the browse query/SELECT doesn't load it).

- [ ] **Step 3: Load source_peer in the track-listing query and struct**

`model.Track` already has the `source_peer` JSON tag (Task 3). Extend the `ListTracks` SELECT/Scan (and any enriched-track query the browse endpoint uses) to include `source_peer`, mirroring the `GetTrack` change. Confirm the browse handler serializes the model `Track`/`EnrichedTrack` without stripping it.

Also extend `GET /api/config` (`internal/api/config.go`) to report online peers so the UI can grey out offline ones. Add to the config response a `fed_peers` array sourced from a setter the manager populates:

```go
// In server.go
	fedPeers func() []string // returns online peer ids; nil when federation off
```
```go
func (s *Server) SetFedPeers(fn func() []string) { s.fedPeers = fn }
```

In `getConfig`, include `"fed_peers"` (empty array when `s.fedPeers == nil`). The `main.go` wiring (`srv.SetFedPeers(fm.OnlinePeers)`) was added in Task 12 — `fm.OnlinePeers` returns the registry's ids on a hub and the hub-reported list on a member, so a member greys out offline siblings correctly.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/ -run TestBrowseExposesSourcePeer -v && go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/api/ internal/store/library.go main.go
git commit -m "feat(api): expose source_peer on tracks + online peers in /api/config"
```

### Task 17: Owner badge + offline grey-out in Svelte

**Files:**
- Modify: `web/src/lib/store.svelte.js` (load `fed_peers` from `/api/config`), `web/src/lib/components/TrackRow.svelte` (find the actual row component with `ls web/src/lib/components`)
- Test: manual (Phase 7)

- [ ] **Step 1: Load fed peers into the store**

In `web/src/lib/store.svelte.js`, where `/api/config` is consumed, store `fedPeers = config.fed_peers ?? []`.

- [ ] **Step 2: Show owner + offline state on remote rows**

In the track row component, when `track.source_peer` is set:
- render a small badge with the peer id (e.g. `<span class="peer-badge">{track.source_peer}</span>`),
- if `!fedPeers.includes(track.source_peer)`, add a `disabled`/dimmed class and disable the play/request action with a title like "owner offline".

Keep styling consistent with existing badges. Dark-friendly colors per the environment (light text on dark chip).

- [ ] **Step 3: Build the UI**

Run: `cd web && npm run build` (or the project's documented build — check `web/package.json`).
Expected: build succeeds; embedded dist updates.

- [ ] **Step 4: Commit**

```bash
git add web/
git commit -m "feat(web): owner badge + offline grey-out for remote tracks"
```

---

## Phase 6 — Admin gate integration (depends on #85 merged)

> Do this phase only after `issue-85-admin-gate` lands on `main`. If it hasn't, stop and surface that.

### Task 18: Gate federated control + require token on hub listener

**Files:**
- Modify: `internal/api/server.go`, `internal/fed/manager.go`
- Test: `internal/api/admin_fed_test.go` (create)

- [ ] **Step 1: Write the failing test**

```go
func TestRemoteControlRequiresAdmin(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	srv := NewServer(db, nil, nil)
	srv.SetAdminPassword("secret") // existing setter from #85

	rec := httptest.NewRecorder()
	// skip on a stream is admin-gated; a remote-sourced stream is no different.
	req := httptest.NewRequest("GET", "/api/streams/house/next", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without admin token, got %d", rec.Code)
	}
}
```

> Adjust the setter/method names to match what #85 actually shipped (`SetAdminPassword` is the expected name from the spec; verify against merged code).

- [ ] **Step 2: Run test to verify it fails or confirms existing gate**

Run: `go test ./internal/api/ -run TestRemoteControlRequiresAdmin -v`
Expected: FAIL if the gate isn't wired, PASS if #85 already covers these routes (then this task just confirms remote streams inherit it — no new code).

- [ ] **Step 3: Confirm/extend gating**

Federated control actions reuse existing `requireAdmin` wrappers from #85 — remote-sourced streams flow through the same handlers, so no new gating code is needed beyond confirming the routes are wrapped. If any federation-specific control route was added, wrap it with `requireAdmin`.

- [ ] **Step 4: Run tests**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/api/
git commit -m "test(api): confirm remote-stream control inherits admin gate"
```

---

## Phase 7 — Manual end-to-end verification

Not a code task — run the real binaries and confirm the original use case. Use superpowers:verification-before-completion.

- [ ] **Step 1: Build**

Run: `go build -o /tmp/exit66 . && cd web && npm run build && cd ..`

- [ ] **Step 2: Start a hub (no library) and a member (with music)**

```bash
# Hub on a local port acting as the public VPS
EXIT66_FED_ROLE=hub EXIT66_FED_TOKEN=tok EXIT66_FED_PEER_ID=vps \
  EXIT66_FED_LISTEN=:8443 /tmp/exit66 -addr :8066 -db /tmp/hub.db &

# Member with a music root, dialing the hub
EXIT66_FED_ROLE=member EXIT66_FED_TOKEN=tok EXIT66_FED_PEER_ID=home \
  EXIT66_FED_HUB=127.0.0.1:8443 /tmp/exit66 -addr :8077 -db /tmp/home.db -root /path/to/music &
```

- [ ] **Step 2b:** Wait for the member to scan, then confirm the hub logs `federation: role=hub` and the member registered.

- [ ] **Step 3: Confirm catalog synced to the hub**

Run: `curl -s localhost:8066/api/tracks | head` — expect the member's tracks with `"source_peer":"home"`.

- [ ] **Step 4: Play a remote track from the hub UI**

Open `http://localhost:8066` in a dark-mode browser, find a `home`-owned track, play it. Confirm audio plays and seeking works (exercises `Range`/206 through the relay).

- [ ] **Step 5: Offline behavior**

Kill the member. Confirm its tracks grey out in the hub UI within ~one config refresh and that play returns a clear error rather than hanging.

- [ ] **Step 6: Record results**

Note pass/fail per step. If anything fails, debug with superpowers:systematic-debugging before claiming completion.

---

## Self-Review notes (already applied)

- **Spec coverage:** topology/connectivity → Phase 2; token auth → Task 7; track identity/storage → Tasks 2–3; audio resolution branch → Task 4; relay flow → Tasks 9–11; ffmpeg URL source → Task 11; catalog sync (push/merge/cache, offline) → Phase 4 + Tasks 16–17; security/admin gate → Phase 6; testing → unit tests per task + integration Tasks 9/15 + manual Phase 7; out-of-scope items (#87 direct P2P, #88 listening groups) intentionally excluded.
- **Type consistency:** `RemoteTrack`/`CatalogRow`/`Peer`/`Manager`/`Relay`/`Resolver` names used consistently across tasks; `ServeRemoteAudio` signature identical in seam (Task 4) and impl (Task 10); `RemoteID` means "owner's local id" in both export (Task 13) and apply (Task 14); `NewRelay(reg, db)` and `PullAndApply(...) ([]string, error)` signatures consistent across Tasks 9/14/15.
- **Hub-is-a-peer (headline scenario):** the hub applies received catalogs to its **own** DB in `receiveCatalog` (Task 14), so the VPS browse shows the home library — verified by `TestReceiveCatalogAppliesToHubDB`. A single `Relay` instance (built in `main.go`, Task 12) is shared by the session handler and the resolver so the registry, catalog cache, and DB are all one.
- **Member liveness:** members learn online siblings from the hub's `MergedCatalog.Online` (Task 14/15), not their local registry; `Manager.OnlinePeers()` returns the right source per role. This makes the office↔home offline grey-out correct.
- **Resolved overloads:** dedicated `Federation.Listen` (`EXIT66_FED_LISTEN`) separates the hub's listen address from a member's dial target (Tasks 1/12); `sourceInput` uses the final loopback-URL form (Task 11).
