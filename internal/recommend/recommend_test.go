package recommend

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/external"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

// --- fakes ---

type fakeLB struct {
	mu    sync.Mutex
	calls int
	recs  []external.RecRecording
	err   error
}

func (f *fakeLB) Username(ctx context.Context) (string, error) { return "alice", nil }
func (f *fakeLB) Recommendations(ctx context.Context, user string, count int) ([]external.RecRecording, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	return f.recs, f.err
}
func (f *fakeLB) callCount() int { f.mu.Lock(); defer f.mu.Unlock(); return f.calls }

type fakeLF struct {
	byArtist map[string][]external.SimilarArtist
}

func (f *fakeLF) SimilarArtists(ctx context.Context, name, mbid string, limit int) ([]external.SimilarArtist, error) {
	return f.byArtist[name], nil
}

// seed inserts an artist/album/track, returns track id. mbid optional.
func seed(t *testing.T, db *sql.DB, artist, title, path, trackMBID string, plays int) (trackID int64) {
	t.Helper()
	var artistID int64
	db.QueryRow(`SELECT id FROM artist WHERE name = ?`, artist).Scan(&artistID)
	if artistID == 0 {
		res, _ := db.Exec(`INSERT INTO artist(name) VALUES(?)`, artist)
		artistID, _ = res.LastInsertId()
	}
	res, err := db.Exec(`INSERT INTO album(name, artist_id) VALUES(?, ?)`, title+" album", artistID)
	if err != nil {
		t.Fatalf("album: %v", err)
	}
	albumID, _ := res.LastInsertId()
	res, err = db.Exec(
		`INSERT INTO track(path, mod_time, size, title, artist_id, album_id, play_count, mbid)
		 VALUES(?,1,1,?,?,?,?,?)`, path, title, artistID, albumID, plays, trackMBID)
	if err != nil {
		t.Fatalf("track: %v", err)
	}
	trackID, _ = res.LastInsertId()
	return trackID
}

func TestRunnerRefreshMapsAndDedupes(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()

	// A track recommended by ListenBrainz (matched by recording mbid).
	lbTrack := seed(t, db, "Stereolab", "Ping Pong", "/m/ping.mp3", "rec-ping", 0)
	// Aphex Twin: a similar-artist seed target with one track. Also (overlap)
	// reachable via LB so dedupe is exercised.
	overlap := seed(t, db, "Aphex Twin", "Xtal", "/m/xtal.mp3", "rec-xtal", 0)
	// A seed artist with plays, so TopArtists yields it.
	seed(t, db, "Boards of Canada", "Roygbiv", "/m/roy.mp3", "", 5)

	lb := &fakeLB{recs: []external.RecRecording{
		{RecordingMBID: "rec-ping", Score: 0.9},
		{RecordingMBID: "rec-xtal", Score: 0.8},
		{RecordingMBID: "rec-unknown", Score: 0.7},
	}}
	lf := &fakeLF{byArtist: map[string][]external.SimilarArtist{
		"Boards of Canada": {{Name: "Aphex Twin", Match: 0.9}}, // maps to overlap by name
	}}

	r := NewRunner(db, lb, lf)
	r.Refresh(context.Background())

	got := r.Get()
	ids := map[int64]int{}
	for _, e := range got {
		ids[e.ID]++
	}
	if ids[lbTrack] != 1 {
		t.Errorf("lb track count = %d, want 1", ids[lbTrack])
	}
	if ids[overlap] != 1 {
		t.Errorf("overlap track count = %d, want 1 (deduped across sources)", ids[overlap])
	}
	if len(got) != 2 {
		t.Fatalf("got %d recommended tracks, want 2; %+v", len(got), got)
	}
	// Enriched fields are populated.
	if got[0].ArtistName == "" {
		t.Error("expected EnrichTracks to fill artist_name")
	}
}

func TestRunnerNoSourcesIsEmpty(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()

	r := NewRunner(db, nil, nil)
	r.Refresh(context.Background())
	if got := r.Get(); len(got) != 0 {
		t.Errorf("got %d, want 0 with no sources", len(got))
	}
}

func TestRunnerGetTriggersRefreshWhenStaleOnly(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	seed(t, db, "A", "T", "/m/a.mp3", "rec-a", 0)

	lb := &fakeLB{recs: []external.RecRecording{{RecordingMBID: "rec-a"}}}
	r := NewRunner(db, lb, nil)

	now := time.Unix(1000, 0)
	r.now = func() time.Time { return now }

	// First Get: cache is empty/never-refreshed → kicks an async refresh.
	r.Get()
	waitFor(t, func() bool { return lb.callCount() == 1 })

	// Second Get within TTL: no new refresh.
	r.Get()
	time.Sleep(20 * time.Millisecond)
	if c := lb.callCount(); c != 1 {
		t.Errorf("calls = %d, want 1 (fresh cache must not refetch)", c)
	}

	// Advance past the TTL: next Get refreshes again.
	now = now.Add(ttl + time.Second)
	r.Get()
	waitFor(t, func() bool { return lb.callCount() == 2 })
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}
