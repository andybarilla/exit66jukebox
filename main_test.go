package main

import (
	"testing"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/config"
	"github.com/andybarilla/exit66jukebox/internal/external"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

func testClient() *external.Client { return external.New("test", time.Second) }

// No credentials -> no client (fully disabled).
func TestNewLastfmNilWhenUnconfigured(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	if lfm := newLastfm(testClient(), db, config.Services{}); lfm != nil {
		t.Fatal("expected nil client with no creds")
	}
}

// Credentials present but no persisted session -> pending auth, still nil so
// nothing is enqueued or sent until `exit66 lastfm-auth` runs.
func TestNewLastfmNilWhenPendingAuth(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	svc := config.Services{LastfmAPIKey: "k", LastfmAPISecret: "s"}
	if lfm := newLastfm(testClient(), db, svc); lfm != nil {
		t.Fatal("expected nil client when configured but not authorized")
	}
}

// Credentials + a persisted session row -> an authorized client.
func TestNewLastfmAuthorizedWithSession(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	if err := store.PutServiceAuth(db, "lastfm", "sk", "alice"); err != nil {
		t.Fatalf("PutServiceAuth: %v", err)
	}
	svc := config.Services{LastfmAPIKey: "k", LastfmAPISecret: "s"}
	lfm := newLastfm(testClient(), db, svc)
	if lfm == nil || !lfm.Authorized() {
		t.Fatalf("expected authorized client, got %v", lfm)
	}
}

// The enqueue gate is computed live: both services when ListenBrainz is on and
// Last.fm is authorized; Last.fm drops out when its client is nil.
func TestActiveScrobbleServices(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	store.PutServiceAuth(db, "lastfm", "sk", "alice")
	lfm := newLastfm(testClient(), db, config.Services{LastfmAPIKey: "k", LastfmAPISecret: "s"})

	if got := activeScrobbleServices(true, lfm); !eq(got, []string{"listenbrainz", "lastfm"}) {
		t.Errorf("both on = %v, want [listenbrainz lastfm]", got)
	}
	if got := activeScrobbleServices(false, lfm); !eq(got, []string{"lastfm"}) {
		t.Errorf("only lastfm = %v, want [lastfm]", got)
	}
	if got := activeScrobbleServices(true, nil); !eq(got, []string{"listenbrainz"}) {
		t.Errorf("lastfm nil = %v, want [listenbrainz]", got)
	}
	if got := activeScrobbleServices(false, nil); len(got) != 0 {
		t.Errorf("none = %v, want empty", got)
	}
}

// waitForClose returns true once the hub goroutine signals done, false if the
// bounded timeout expires first (so shutdown can't hang on a stuck goroutine).
func TestWaitForClose(t *testing.T) {
	closed := make(chan struct{})
	close(closed)
	if !waitForClose(closed, 50*time.Millisecond) {
		t.Error("already-closed channel should return true")
	}

	done := make(chan struct{})
	go func() {
		time.Sleep(10 * time.Millisecond)
		close(done)
	}()
	if !waitForClose(done, time.Second) {
		t.Error("channel closing before timeout should return true")
	}

	start := time.Now()
	if waitForClose(make(chan struct{}), 20*time.Millisecond) {
		t.Error("never-closing channel should return false at timeout")
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Errorf("timeout wait took %v, expected ~20ms", elapsed)
	}
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
