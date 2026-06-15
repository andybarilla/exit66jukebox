package fed

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/model"
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

func TestServeMergedIncludesHubOwnLibrary(t *testing.T) {
	hubDB, _ := store.Open(":memory:")
	defer hubDB.Close()
	if _, err := store.UpsertTrack(hubDB, model.Track{Path: "/m/h.mp3", Title: "HubSong", TrackNo: 1}, "HubBand", "HubBand", "HubRec"); err != nil {
		t.Fatal(err)
	}
	relay := NewRelay(NewRegistry(), hubDB)
	relay.SetSelf("vps")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/fed/catalog/home/merged", nil)
	req.SetPathValue("peer", "home")
	relay.serveMerged(rec, req)

	var merged MergedCatalog
	if err := json.Unmarshal(rec.Body.Bytes(), &merged); err != nil {
		t.Fatal(err)
	}
	if len(merged.Catalogs["vps"]) != 1 || merged.Catalogs["vps"][0].Title != "HubSong" {
		t.Fatalf("expected hub's own catalog under 'vps', got %+v", merged.Catalogs)
	}
	foundSelf := false
	for _, p := range merged.Online {
		if p == "vps" {
			foundSelf = true
		}
	}
	if !foundSelf {
		t.Fatalf("hub self 'vps' should be in Online, got %v", merged.Online)
	}
}

func TestRelayServesHubOwnAudio(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "h.mp3")
	if err := os.WriteFile(fp, []byte("HUBAUDIO"), 0o644); err != nil {
		t.Fatal(err)
	}
	hubDB, _ := store.Open(":memory:")
	defer hubDB.Close()
	id, err := store.UpsertTrack(hubDB, model.Track{Path: fp, Title: "H", TrackNo: 1}, "B", "B", "R")
	if err != nil {
		t.Fatal(err)
	}
	relay := NewRelay(NewRegistry(), hubDB)
	relay.SetSelf("vps")

	rec := httptest.NewRecorder()
	idStr := strconv.FormatInt(id, 10)
	req := httptest.NewRequest("GET", "/fed/audio/vps/"+idStr, nil)
	req.SetPathValue("peer", "vps")
	req.SetPathValue("id", idStr)
	relay.ServeHTTP(rec, req)

	if rec.Body.String() != "HUBAUDIO" {
		t.Fatalf("expected hub's own file bytes, got %q (code %d)", rec.Body.String(), rec.Code)
	}
}

func TestReceiveCatalogAppliesToHubDB(t *testing.T) {
	hubDB, _ := store.Open(":memory:")
	defer hubDB.Close()
	relay := NewRelay(NewRegistry(), hubDB)
	body, _ := json.Marshal([]store.CatalogRow{{RemoteID: 1, Title: "Hit", ArtistName: "A", AlbumName: "Al"}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/fed/catalog/home", bytes.NewReader(body))
	req.SetPathValue("peer", "home")
	relay.receiveCatalog(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	got, _ := store.ListTracks(hubDB, "", 0, 0)
	if len(got) != 1 || got[0].Title != "Hit" {
		t.Fatalf("hub DB should hold the member's track, got %+v", got)
	}
}

// Re-applying an unchanged catalog (every sync tick does) must keep each track's
// local id stable — otherwise a browsed id 404s on a later play and queue rows
// dangle. Regression for the delete+reinsert churn caught in manual e2e.
func TestApplyCatalogKeepsStableIDs(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	rows := []store.CatalogRow{
		{RemoteID: 1, Title: "One", ArtistName: "A", AlbumName: "Al"},
		{RemoteID: 2, Title: "Two", ArtistName: "A", AlbumName: "Al"},
	}
	if err := ApplyCatalog(db, "home", rows); err != nil {
		t.Fatal(err)
	}
	first, _ := store.ListTracks(db, "", 0, 0)
	if len(first) != 2 {
		t.Fatalf("want 2 tracks, got %d", len(first))
	}
	ids := map[string]int64{first[0].Title: first[0].ID, first[1].Title: first[1].ID}

	if err := ApplyCatalog(db, "home", rows); err != nil { // identical re-apply
		t.Fatal(err)
	}
	again, _ := store.ListTracks(db, "", 0, 0)
	if len(again) != 2 {
		t.Fatalf("want 2 tracks after re-apply, got %d", len(again))
	}
	for _, tr := range again {
		if tr.ID != ids[tr.Title] {
			t.Fatalf("id for %q changed across sync: was %d now %d", tr.Title, ids[tr.Title], tr.ID)
		}
	}
}
