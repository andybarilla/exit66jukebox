package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/enrich"
	"github.com/andybarilla/exit66jukebox/internal/external"
	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

// fakeSearcher returns one confident match and signals when it is called, so a
// test can tell whether the pass actually reached a track.
type fakeSearcher struct {
	match  external.RecordingMatch
	called chan struct{}
}

func (f *fakeSearcher) SearchRecording(_ context.Context, _, _, _ string) (external.RecordingMatch, bool, error) {
	select {
	case f.called <- struct{}{}:
	default:
	}
	return f.match, true, nil
}

// noCover never finds a cover, keeping the pass to the MusicBrainz step.
type noCover struct{}

func (noCover) FetchFrontCover(context.Context, string) ([]byte, string, bool, error) {
	return nil, "", false, nil
}

// TestEnrichPassOutlivesRequest guards against binding the background pass to
// the HTTP request context. The pass runs after the POST handler returns, so
// using r.Context() cancels it immediately and it processes nothing. A real
// httptest.Server is required: it cancels the request context once the handler
// returns, exactly as net/http does in production (httptest.NewRecorder does
// not, which is why the original endpoint test missed this).
func TestEnrichPassOutlivesRequest(t *testing.T) {
	srv := newTestServer(t)
	id, _ := store.UpsertTrack(srv.db, model.Track{Path: "/m/a.mp3", Title: "Song"}, "Artist", "Album")

	called := make(chan struct{}, 1)
	mb := &fakeSearcher{
		match:  external.RecordingMatch{Score: 95, RecordingMBID: "rec-1"},
		called: called,
	}
	srv.SetEnrichRunner(enrich.NewRunner(srv.db, mb, noCover{}, t.TempDir()))

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/enrich", "", nil)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()

	// The pass must reach the track even though the request has completed.
	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("pass did not process the track after the request returned (bound to request context?)")
	}

	// ...and apply the match.
	deadline := time.Now().Add(2 * time.Second)
	for {
		var mbid string
		if err := srv.db.QueryRow(`SELECT mbid FROM track WHERE id = ?`, id).Scan(&mbid); err != nil {
			t.Fatalf("query mbid: %v", err)
		}
		if mbid == "rec-1" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("track mbid not applied, got %q", mbid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
