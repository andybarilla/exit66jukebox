package scrobble

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/external"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

type fakeSub struct {
	mu      sync.Mutex
	batches [][]external.Listen
	fail    bool
}

func (f *fakeSub) Submit(ctx context.Context, listens []external.Listen) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return errors.New("boom")
	}
	f.batches = append(f.batches, listens)
	return nil
}

func newScrobbleDB(t *testing.T) (*sql.DB, int64) {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO artist(id, name) VALUES(1, 'A')`); err != nil {
		t.Fatalf("artist: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO album(id, name, artist_id) VALUES(1, 'Rel', 1)`); err != nil {
		t.Fatalf("album: %v", err)
	}
	res, err := db.Exec(
		`INSERT INTO track(path, mod_time, size, title, artist_id, album_id, duration)
		 VALUES('/m/s.mp3', 1, 1, 'Trk', 1, 1, 100)`)
	if err != nil {
		t.Fatalf("track: %v", err)
	}
	id, _ := res.LastInsertId()
	return db, id
}

func TestDrainOnSuccessDeletes(t *testing.T) {
	db, trackID := newScrobbleDB(t)
	defer db.Close()
	store.EnqueueScrobble(db, []string{"listenbrainz"}, trackID, 1000)
	store.EnqueueScrobble(db, []string{"listenbrainz"}, trackID, 2000)

	sub := &fakeSub{}
	d := NewDrainer(db, map[string]Submitter{"listenbrainz": sub}, 50)
	if failed := d.DrainOnce(context.Background()); failed {
		t.Fatal("DrainOnce reported failure on a healthy submitter")
	}

	if len(sub.batches) != 1 || len(sub.batches[0]) != 2 {
		t.Fatalf("expected one batch of 2 listens, got %v", sub.batches)
	}
	if sub.batches[0][0].Meta.TrackName != "Trk" || sub.batches[0][0].Meta.ArtistName != "A" || sub.batches[0][0].Meta.ReleaseName != "Rel" {
		t.Fatalf("metadata not resolved: %+v", sub.batches[0][0])
	}
	rows, _ := store.ScrobbleBatch(db, "listenbrainz", 50)
	if len(rows) != 0 {
		t.Fatalf("expected queue empty after success, got %d rows", len(rows))
	}
}

func TestDrainBackoffOnFailureKeepsRows(t *testing.T) {
	db, trackID := newScrobbleDB(t)
	defer db.Close()
	store.EnqueueScrobble(db, []string{"listenbrainz"}, trackID, 1000)

	sub := &fakeSub{fail: true}
	d := NewDrainer(db, map[string]Submitter{"listenbrainz": sub}, 50)
	if failed := d.DrainOnce(context.Background()); !failed {
		t.Fatal("DrainOnce should report failure when submit errors")
	}

	rows, _ := store.ScrobbleBatch(db, "listenbrainz", 50)
	if len(rows) != 1 {
		t.Fatalf("failed submit must retain rows, got %d", len(rows))
	}
	if rows[0].Attempts != 1 {
		t.Fatalf("attempts = %d, want 1 after one failed drain", rows[0].Attempts)
	}
}

// A second drainer over the same (already-populated) DB delivers the pending
// rows — the queue state, not in-memory state, drives delivery, so a restart
// resumes cleanly.
func TestDrainResumesPendingRows(t *testing.T) {
	db, trackID := newScrobbleDB(t)
	defer db.Close()
	store.EnqueueScrobble(db, []string{"listenbrainz"}, trackID, 1000)

	// First drainer fails; rows survive.
	failing := &fakeSub{fail: true}
	NewDrainer(db, map[string]Submitter{"listenbrainz": failing}, 50).DrainOnce(context.Background())

	// "Restart": a fresh drainer over the same DB succeeds.
	ok := &fakeSub{}
	if failed := NewDrainer(db, map[string]Submitter{"listenbrainz": ok}, 50).DrainOnce(context.Background()); failed {
		t.Fatal("resumed drain reported failure")
	}
	if len(ok.batches) != 1 || len(ok.batches[0]) != 1 {
		t.Fatalf("expected resumed delivery of 1 listen, got %v", ok.batches)
	}
	rows, _ := store.ScrobbleBatch(db, "listenbrainz", 50)
	if len(rows) != 0 {
		t.Fatalf("expected queue drained after resume, got %d", len(rows))
	}
}

// A service with no submitter wired (e.g. lastfm not yet enabled) leaves its
// rows untouched rather than erroring.
func TestDrainIgnoresUnwiredService(t *testing.T) {
	db, trackID := newScrobbleDB(t)
	defer db.Close()
	store.EnqueueScrobble(db, []string{"listenbrainz", "lastfm"}, trackID, 1000)

	sub := &fakeSub{}
	d := NewDrainer(db, map[string]Submitter{"listenbrainz": sub}, 50)
	d.DrainOnce(context.Background())

	lf, _ := store.ScrobbleBatch(db, "lastfm", 50)
	if len(lf) != 1 {
		t.Fatalf("lastfm rows should be untouched, got %d", len(lf))
	}
}

type disabledSub struct{ calls int }

func (d *disabledSub) Submit(ctx context.Context, listens []external.Listen) error {
	d.calls++
	return external.ErrServiceDisabled
}

// A submitter that reports ErrServiceDisabled must not have its rows bumped or
// deleted, and the cycle must not count as a failure (so the shared backoff
// doesn't penalize other services). Rows stay queued for a later re-auth.
func TestDrainServiceDisabledLeavesRowsNoFailure(t *testing.T) {
	db, trackID := newScrobbleDB(t)
	defer db.Close()
	store.EnqueueScrobble(db, []string{"lastfm"}, trackID, 1000)

	sub := &disabledSub{}
	d := NewDrainer(db, map[string]Submitter{"lastfm": sub}, 50)
	if failed := d.DrainOnce(context.Background()); failed {
		t.Fatal("disabled service must not count as a drain failure")
	}
	rows, _ := store.ScrobbleBatch(db, "lastfm", 50)
	if len(rows) != 1 {
		t.Fatalf("disabled service must retain rows, got %d", len(rows))
	}
	if rows[0].Attempts != 0 {
		t.Fatalf("disabled service must not bump attempts, got %d", rows[0].Attempts)
	}
}

// A disabled Last.fm in the same cycle as a healthy ListenBrainz must not drag
// the healthy one: ListenBrainz still drains and the cycle reports no failure.
func TestDrainDisabledDoesNotBlockHealthyService(t *testing.T) {
	db, trackID := newScrobbleDB(t)
	defer db.Close()
	store.EnqueueScrobble(db, []string{"listenbrainz", "lastfm"}, trackID, 1000)

	lb := &fakeSub{}
	d := NewDrainer(db, map[string]Submitter{"listenbrainz": lb, "lastfm": &disabledSub{}}, 50)
	if failed := d.DrainOnce(context.Background()); failed {
		t.Fatal("healthy + disabled cycle should report no failure")
	}
	if len(lb.batches) != 1 {
		t.Fatalf("listenbrainz should have drained, got %d batches", len(lb.batches))
	}
	lbRows, _ := store.ScrobbleBatch(db, "listenbrainz", 50)
	if len(lbRows) != 0 {
		t.Fatalf("listenbrainz rows should be drained, got %d", len(lbRows))
	}
	lfRows, _ := store.ScrobbleBatch(db, "lastfm", 50)
	if len(lfRows) != 1 {
		t.Fatalf("lastfm rows should be retained, got %d", len(lfRows))
	}
}

func TestNextInterval(t *testing.T) {
	base := nextInterval(baseInterval, 0)
	if base != baseInterval {
		t.Fatalf("no failures should use base interval, got %v", base)
	}
	if nextInterval(baseInterval, 1) != baseInterval*2 {
		t.Fatalf("one failure should double the interval")
	}
	if nextInterval(baseInterval, 100) != maxInterval {
		t.Fatalf("backoff must cap at maxInterval, got %v", nextInterval(baseInterval, 100))
	}
}
