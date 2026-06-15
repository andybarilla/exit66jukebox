package fed

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
