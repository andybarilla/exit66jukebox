package store

import (
	"database/sql"
	"testing"
)

func seedScrobbleTrack(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO artist(id, name) VALUES(1, 'The Band')`); err != nil {
		t.Fatalf("artist: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO album(id, name, artist_id) VALUES(1, 'The Album', 1)`); err != nil {
		t.Fatalf("album: %v", err)
	}
	res, err := db.Exec(
		`INSERT INTO track(path, mod_time, size, title, artist_id, album_id, duration)
		 VALUES('/m/song.mp3', 1, 1, 'The Song', 1, 1, 200)`)
	if err != nil {
		t.Fatalf("track: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestEnqueueScrobblePerService(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	trackID := seedScrobbleTrack(t, db)

	if err := EnqueueScrobble(db, []string{"listenbrainz", "lastfm"}, trackID, 1500); err != nil {
		t.Fatalf("EnqueueScrobble: %v", err)
	}

	lb, err := ScrobbleBatch(db, "listenbrainz", 10)
	if err != nil {
		t.Fatalf("ScrobbleBatch: %v", err)
	}
	if len(lb) != 1 {
		t.Fatalf("listenbrainz rows = %d, want 1", len(lb))
	}
	if lb[0].TrackID != trackID || lb[0].PlayedAt != 1500 || lb[0].Attempts != 0 {
		t.Fatalf("unexpected row %+v", lb[0])
	}
	lf, _ := ScrobbleBatch(db, "lastfm", 10)
	if len(lf) != 1 {
		t.Fatalf("lastfm rows = %d, want 1", len(lf))
	}
}

func TestScrobbleBatchLimitAndOrder(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	trackID := seedScrobbleTrack(t, db)
	for i := 0; i < 3; i++ {
		if err := EnqueueScrobble(db, []string{"listenbrainz"}, trackID, int64(1000+i)); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
	}
	batch, err := ScrobbleBatch(db, "listenbrainz", 2)
	if err != nil {
		t.Fatalf("ScrobbleBatch: %v", err)
	}
	if len(batch) != 2 {
		t.Fatalf("batch len = %d, want 2 (limit)", len(batch))
	}
	if batch[0].PlayedAt != 1000 || batch[1].PlayedAt != 1001 {
		t.Fatalf("expected oldest-first order, got %d,%d", batch[0].PlayedAt, batch[1].PlayedAt)
	}
}

func TestDeleteScrobble(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	trackID := seedScrobbleTrack(t, db)
	EnqueueScrobble(db, []string{"listenbrainz"}, trackID, 1000)
	batch, _ := ScrobbleBatch(db, "listenbrainz", 10)
	if err := DeleteScrobble(db, batch[0].ID); err != nil {
		t.Fatalf("DeleteScrobble: %v", err)
	}
	after, _ := ScrobbleBatch(db, "listenbrainz", 10)
	if len(after) != 0 {
		t.Fatalf("expected 0 rows after delete, got %d", len(after))
	}
}

func TestBumpScrobbleAttempts(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	trackID := seedScrobbleTrack(t, db)
	EnqueueScrobble(db, []string{"listenbrainz"}, trackID, 1000)
	batch, _ := ScrobbleBatch(db, "listenbrainz", 10)
	if err := BumpScrobbleAttempts(db, []int64{batch[0].ID}); err != nil {
		t.Fatalf("BumpScrobbleAttempts: %v", err)
	}
	again, _ := ScrobbleBatch(db, "listenbrainz", 10)
	if again[0].Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", again[0].Attempts)
	}
}

func TestScrobbleMetadata(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	trackID := seedScrobbleTrack(t, db)
	meta, ok, err := ScrobbleMetadata(db, trackID)
	if err != nil {
		t.Fatalf("ScrobbleMetadata: %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if meta.ArtistName != "The Band" || meta.TrackName != "The Song" || meta.ReleaseName != "The Album" {
		t.Fatalf("unexpected meta %+v", meta)
	}
	if meta.Duration != 200 {
		t.Fatalf("duration = %d, want 200", meta.Duration)
	}

	if _, ok, _ := ScrobbleMetadata(db, 9999); ok {
		t.Fatal("expected ok=false for missing track")
	}
}
