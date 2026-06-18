package fed

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

func TestPeerRoutesExportsLocalCatalog(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	if _, err := store.UpsertTrack(db, model.Track{Path: "/music/song.mp3", Title: "Song"}, "Artist", "Artist", "Album"); err != nil {
		t.Fatalf("upsert track: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/fed/catalog", nil)

	PeerRoutes(db, nil).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("catalog status = %d (%s)", rec.Code, rec.Body)
	}
	var rows []store.CatalogRow
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	if len(rows) != 1 || rows[0].Title != "Song" {
		t.Fatalf("catalog rows = %#v", rows)
	}
}
