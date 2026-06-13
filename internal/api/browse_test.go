package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

func seedAPILibrary(t *testing.T, srv *Server) {
	t.Helper()
	store.UpsertTrack(srv.db, model.Track{Path: "/m/ct.mp3", Title: "Come Together", TrackNo: 1}, "The Beatles", "Abbey Road")
	store.UpsertTrack(srv.db, model.Track{Path: "/m/sm.mp3", Title: "Something", TrackNo: 2}, "The Beatles", "Abbey Road")
	store.UpsertTrack(srv.db, model.Track{Path: "/m/mn.mp3", Title: "Money", TrackNo: 1}, "ABBA", "Arrival")
}

func get(t *testing.T, srv *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status %d", path, rec.Code)
	}
	return rec
}

func TestTracksEndpointEnriched(t *testing.T) {
	srv := newTestServer(t)
	seedAPILibrary(t, srv)
	rec := get(t, srv, "/api/tracks")

	var tracks []model.EnrichedTrack
	if err := json.Unmarshal(rec.Body.Bytes(), &tracks); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, rec.Body.String())
	}
	if len(tracks) != 3 {
		t.Fatalf("expected 3 tracks, got %d", len(tracks))
	}
	// album-grouped order: A1, A2, B1
	if tracks[0].Code != "A1" || tracks[1].Code != "A2" || tracks[2].Code != "B1" {
		t.Errorf("codes = %q %q %q", tracks[0].Code, tracks[1].Code, tracks[2].Code)
	}
	if tracks[0].AlbumName != "Abbey Road" || tracks[0].ArtistName != "The Beatles" || tracks[0].Tone != "cyan" {
		t.Errorf("first track enrichment wrong: %+v", tracks[0])
	}
	if got := rec.Header().Get("X-Total-Count"); got != "3" {
		t.Errorf("X-Total-Count = %q, want 3", got)
	}
}

func TestTracksEndpointCodeSearch(t *testing.T) {
	srv := newTestServer(t)
	seedAPILibrary(t, srv)
	rec := get(t, srv, "/api/tracks?search=B1")
	var tracks []model.EnrichedTrack
	json.Unmarshal(rec.Body.Bytes(), &tracks)
	if len(tracks) != 1 || tracks[0].Title != "Money" {
		t.Fatalf("code search: got %+v", tracks)
	}
}

func TestAlbumsEndpointEnriched(t *testing.T) {
	srv := newTestServer(t)
	seedAPILibrary(t, srv)
	rec := get(t, srv, "/api/albums")
	var albums []model.EnrichedAlbum
	json.Unmarshal(rec.Body.Bytes(), &albums)
	if len(albums) != 2 {
		t.Fatalf("expected 2 albums, got %d", len(albums))
	}
	if albums[0].Letter != "A" || albums[0].Tone != "cyan" || albums[0].TrackCount != 2 ||
		albums[0].ArtistName != "The Beatles" {
		t.Errorf("first album wrong: %+v", albums[0])
	}
	if got := rec.Header().Get("X-Total-Count"); got != "2" {
		t.Errorf("X-Total-Count = %q, want 2", got)
	}
}

func TestAlbumTracksEndpoint(t *testing.T) {
	srv := newTestServer(t)
	seedAPILibrary(t, srv)
	var albumID int64
	srv.db.QueryRow(`SELECT id FROM album WHERE name = 'Abbey Road'`).Scan(&albumID)
	rec := get(t, srv, "/api/albums/"+strconv.FormatInt(albumID, 10)+"/tracks")
	var tracks []model.EnrichedTrack
	json.Unmarshal(rec.Body.Bytes(), &tracks)
	if len(tracks) != 2 || tracks[0].Code != "A1" || tracks[1].Code != "A2" {
		t.Fatalf("album tracks: got %+v", tracks)
	}
}

func TestQueueItemsEnriched(t *testing.T) {
	srv := newTestServer(t)
	seedAPILibrary(t, srv)
	var moneyID int64
	srv.db.QueryRow(`SELECT id FROM track WHERE title = 'Money'`).Scan(&moneyID)

	form := url.Values{"kind": {"track"}, "id": {strconv.FormatInt(moneyID, 10)}}
	req := httptest.NewRequest(http.MethodPost, "/api/streams/sess/requests", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.Handler().ServeHTTP(httptest.NewRecorder(), req)

	rec := get(t, srv, "/api/streams/sess")
	body := rec.Body.String()
	if !strings.Contains(body, `"code":"B1"`) || !strings.Contains(body, `"album_name":"Arrival"`) {
		t.Fatalf("queue item not enriched: %s", body)
	}
}

func TestNextTrackEnriched(t *testing.T) {
	srv := newTestServer(t)
	seedAPILibrary(t, srv)
	var moneyID int64
	srv.db.QueryRow(`SELECT id FROM track WHERE title = 'Money'`).Scan(&moneyID)

	form := url.Values{"kind": {"track"}, "id": {strconv.FormatInt(moneyID, 10)}}
	req := httptest.NewRequest(http.MethodPost, "/api/streams/sess/requests", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.Handler().ServeHTTP(httptest.NewRecorder(), req)

	// The /next payload backs now-playing; it must carry the slot code/tone/names.
	rec := get(t, srv, "/api/streams/sess/next")
	body := rec.Body.String()
	if !strings.Contains(body, `"code":"B1"`) || !strings.Contains(body, `"tone":"magenta"`) {
		t.Fatalf("next track not enriched: %s", body)
	}
}
