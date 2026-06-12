package enrich

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/external"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

// fakeMB returns a canned match and counts how many searches it received.
type fakeMB struct {
	match external.RecordingMatch
	ok    bool
	calls int32
	block chan struct{} // if non-nil, SearchRecording blocks until closed
}

func (f *fakeMB) SearchRecording(ctx context.Context, artist, title, album string) (external.RecordingMatch, bool, error) {
	atomic.AddInt32(&f.calls, 1)
	if f.block != nil {
		<-f.block
	}
	return f.match, f.ok, nil
}

// fakeCAA returns canned cover bytes.
type fakeCAA struct {
	data []byte
	ct   string
	ok   bool
}

func (f *fakeCAA) FetchFrontCover(ctx context.Context, releaseMBID string) ([]byte, string, bool, error) {
	return f.data, f.ct, f.ok, nil
}

func seedTrack(t *testing.T, db *sql.DB, artist, album, title, path string) (alID, trID int64) {
	t.Helper()
	res, _ := db.Exec(`INSERT INTO artist(name) VALUES(?)`, artist)
	arID, _ := res.LastInsertId()
	res, _ = db.Exec(`INSERT INTO album(name, artist_id) VALUES(?, ?)`, album, arID)
	alID, _ = res.LastInsertId()
	res, err := db.Exec(
		`INSERT INTO track(path, mod_time, size, title, artist_id, album_id) VALUES(?, 1, 1, ?, ?, ?)`,
		path, title, arID, alID)
	if err != nil {
		t.Fatalf("track: %v", err)
	}
	trID, _ = res.LastInsertId()
	return
}

func waitDone(t *testing.T, r *Runner) Status {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s := r.Status(); !s.Running {
			return s
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("pass did not finish within timeout")
	return Status{}
}

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestRunMatchesAndFetchesCover(t *testing.T) {
	db := openDB(t)
	alID, trID := seedTrack(t, db, "Unknown Artist", "Unknown Album", "a.mp3", "/m/a.mp3")
	coversDir := t.TempDir()

	mb := &fakeMB{ok: true, match: external.RecordingMatch{
		Score: 95, RecordingMBID: "rec1", RecordingTitle: "Karma Police",
		ArtistMBID: "art1", ArtistName: "Radiohead",
		ReleaseMBID: "rel1", ReleaseTitle: "OK Computer",
	}}
	ca := &fakeCAA{data: []byte("imgbytes"), ct: "image/jpeg", ok: true}

	r := NewRunner(db, mb, ca, coversDir)
	if _, started := r.Start(context.Background()); !started {
		t.Fatal("expected Start to begin a pass")
	}
	s := waitDone(t, r)
	if s.Processed != 1 || s.Matched != 1 || s.Covers != 1 || s.Failed != 0 {
		t.Fatalf("status = %+v, want processed=1 matched=1 covers=1 failed=0", s)
	}

	var mbid, title string
	db.QueryRow(`SELECT mbid, title FROM track WHERE id=?`, trID).Scan(&mbid, &title)
	if mbid != "rec1" || title != "Karma Police" {
		t.Errorf("track = %q/%q, want rec1/Karma Police", mbid, title)
	}
	var cover string
	db.QueryRow(`SELECT cover FROM album WHERE id=?`, alID).Scan(&cover)
	want := filepath.Join(coversDir, "1.jpg")
	if cover != want {
		t.Errorf("album cover = %q, want %q", cover, want)
	}
	if b, err := os.ReadFile(want); err != nil || string(b) != "imgbytes" {
		t.Errorf("cover file = %q/%v, want imgbytes", string(b), err)
	}
}

func TestRunSkipsLowScore(t *testing.T) {
	db := openDB(t)
	_, trID := seedTrack(t, db, "Unknown Artist", "Unknown Album", "a.mp3", "/m/a.mp3")

	mb := &fakeMB{ok: true, match: external.RecordingMatch{Score: 50, RecordingMBID: "rec1"}}
	r := NewRunner(db, mb, &fakeCAA{}, t.TempDir())
	r.Start(context.Background())
	s := waitDone(t, r)
	if s.Matched != 0 {
		t.Errorf("matched = %d, want 0 for sub-threshold score", s.Matched)
	}
	var mbid string
	db.QueryRow(`SELECT mbid FROM track WHERE id=?`, trID).Scan(&mbid)
	if mbid != "" {
		t.Errorf("track mbid = %q, want empty (not matched)", mbid)
	}
}

func TestRunSkipsAlreadyMatched(t *testing.T) {
	db := openDB(t)
	_, trID := seedTrack(t, db, "A", "X", "T", "/m/a.mp3")
	db.Exec(`UPDATE track SET mbid='existing' WHERE id=?`, trID)

	mb := &fakeMB{ok: true, match: external.RecordingMatch{Score: 99, RecordingMBID: "rec1"}}
	r := NewRunner(db, mb, &fakeCAA{}, t.TempDir())
	r.Start(context.Background())
	waitDone(t, r)
	if c := atomic.LoadInt32(&mb.calls); c != 0 {
		t.Errorf("SearchRecording called %d times, want 0 for already-matched track", c)
	}
}

func TestStartTwiceDoesNotDoubleRun(t *testing.T) {
	db := openDB(t)
	seedTrack(t, db, "Unknown Artist", "Unknown Album", "a.mp3", "/m/a.mp3")

	// Block the first pass mid-search so the second Start sees Running.
	block := make(chan struct{})
	mb := &fakeMB{ok: true, block: block, match: external.RecordingMatch{Score: 95, RecordingMBID: "rec1"}}
	r := NewRunner(db, mb, &fakeCAA{}, t.TempDir())

	if _, started := r.Start(context.Background()); !started {
		t.Fatal("first Start should begin a pass")
	}
	if _, started := r.Start(context.Background()); started {
		t.Error("second Start should not begin a concurrent pass")
	}
	close(block)
	waitDone(t, r)
	if c := atomic.LoadInt32(&mb.calls); c != 1 {
		t.Errorf("SearchRecording called %d times, want 1 (no double run)", c)
	}
}
