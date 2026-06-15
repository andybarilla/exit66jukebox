package api

import (
	"net/http"
	"net/http/httptest"
	"strconv"
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
	req.SetPathValue("id", strconv.FormatInt(id, 10))
	srv.trackAudio(rec, req)

	if called != "home" {
		t.Fatalf("expected remote resolve for peer 'home', got %q", called)
	}
	if rec.Body.String() != "remote-bytes" {
		t.Fatalf("expected proxied body, got %q", rec.Body.String())
	}
}

type fakeResolver func(http.ResponseWriter, *http.Request, string, int64)

func (f fakeResolver) ServeRemoteAudio(w http.ResponseWriter, r *http.Request, peer string, remoteID int64) {
	f(w, r, peer, remoteID)
}
