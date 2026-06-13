package store

import (
	"database/sql"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/model"
)

// seedLibrary builds a small fixed library whose global album order by sort_key
// is: "Abbey Road" (rank 0, A/cyan), "Arrival" (rank 1, B/magenta),
// "The Zoo" (rank 2, C/amber).
func seedLibrary(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	UpsertTrack(db, model.Track{Path: "/m/ct.mp3", Title: "Come Together", TrackNo: 1}, "The Beatles", "Abbey Road")
	UpsertTrack(db, model.Track{Path: "/m/sm.mp3", Title: "Something", TrackNo: 2}, "The Beatles", "Abbey Road")
	UpsertTrack(db, model.Track{Path: "/m/mn.mp3", Title: "Money Money", TrackNo: 1}, "ABBA", "Arrival")
	UpsertTrack(db, model.Track{Path: "/m/zd.mp3", Title: "Zed", TrackNo: 1}, "Ziggy", "The Zoo")
	return db
}

func TestListAlbumsEnriched(t *testing.T) {
	db := seedLibrary(t)
	defer db.Close()

	albums, err := ListAlbumsEnriched(db, "", 50, 0)
	if err != nil {
		t.Fatalf("ListAlbumsEnriched: %v", err)
	}
	if len(albums) != 3 {
		t.Fatalf("expected 3 albums, got %d", len(albums))
	}
	want := []struct {
		name, letter, tone, artist string
		count                      int
	}{
		{"Abbey Road", "A", "cyan", "The Beatles", 2},
		{"Arrival", "B", "magenta", "ABBA", 1},
		{"The Zoo", "C", "amber", "Ziggy", 1},
	}
	for i, w := range want {
		a := albums[i]
		if a.Name != w.name || a.Letter != w.letter || a.Tone != w.tone ||
			a.ArtistName != w.artist || a.TrackCount != w.count {
			t.Errorf("album[%d] = {%q %q %q %q %d}, want {%q %q %q %q %d}",
				i, a.Name, a.Letter, a.Tone, a.ArtistName, a.TrackCount,
				w.name, w.letter, w.tone, w.artist, w.count)
		}
	}
}

func TestListAlbumsEnrichedSearchMatchesArtist(t *testing.T) {
	db := seedLibrary(t)
	defer db.Close()
	got, _ := ListAlbumsEnriched(db, "Beatles", 50, 0)
	if len(got) != 1 || got[0].Name != "Abbey Road" {
		t.Fatalf("search by artist: got %+v", got)
	}
	// Letter stays the GLOBAL rank-derived value even under a filter.
	if got[0].Letter != "A" {
		t.Errorf("filtered album letter = %q, want global %q", got[0].Letter, "A")
	}
}

func TestListTracksEnrichedAlbumGrouped(t *testing.T) {
	db := seedLibrary(t)
	defer db.Close()
	tracks, err := ListTracksEnriched(db, "", 50, 0)
	if err != nil {
		t.Fatalf("ListTracksEnriched: %v", err)
	}
	wantCodes := []string{"A1", "A2", "B1", "C1"}
	if len(tracks) != len(wantCodes) {
		t.Fatalf("expected %d tracks, got %d", len(wantCodes), len(tracks))
	}
	for i, code := range wantCodes {
		if tracks[i].Code != code {
			t.Errorf("track[%d].Code = %q, want %q", i, tracks[i].Code, code)
		}
	}
	ct := tracks[0]
	if ct.AlbumName != "Abbey Road" || ct.ArtistName != "The Beatles" || ct.Tone != "cyan" {
		t.Errorf("first track enrichment = {%q %q %q}", ct.AlbumName, ct.ArtistName, ct.Tone)
	}
}

func TestListTracksEnrichedSearchBroadened(t *testing.T) {
	db := seedLibrary(t)
	defer db.Close()
	byArtist, _ := ListTracksEnriched(db, "ABBA", 50, 0)
	if len(byArtist) != 1 || byArtist[0].Title != "Money Money" {
		t.Fatalf("search by artist name: got %+v", byArtist)
	}
	byAlbum, _ := ListTracksEnriched(db, "Abbey", 50, 0)
	if len(byAlbum) != 2 {
		t.Fatalf("search by album name: expected 2, got %d", len(byAlbum))
	}
}

func TestListTracksEnrichedCodeSearch(t *testing.T) {
	db := seedLibrary(t)
	defer db.Close()
	got, _ := ListTracksEnriched(db, "B1", 50, 0)
	if len(got) != 1 || got[0].Title != "Money Money" || got[0].Code != "B1" {
		t.Fatalf("code search B1: got %+v", got)
	}
}

func TestEnrichTracksMatchesBrowse(t *testing.T) {
	db := seedLibrary(t)
	defer db.Close()
	// The code/tone a track gets via the by-id enricher (queue/now-playing/
	// discover path) must equal what the browse query produces (same global rank).
	browse, _ := ListTracksEnriched(db, "", 50, 0)
	raw := make([]model.Track, len(browse))
	for i, b := range browse {
		raw[i] = b.Track
	}
	enriched, err := EnrichTracks(db, raw)
	if err != nil {
		t.Fatalf("EnrichTracks: %v", err)
	}
	for i := range browse {
		if enriched[i].Code != browse[i].Code || enriched[i].Tone != browse[i].Tone ||
			enriched[i].AlbumName != browse[i].AlbumName || enriched[i].ArtistName != browse[i].ArtistName {
			t.Errorf("track %d: by-id {%q %q} != browse {%q %q}",
				i, enriched[i].Code, enriched[i].Tone, browse[i].Code, browse[i].Tone)
		}
	}
}

func TestTracksByAlbumEnriched(t *testing.T) {
	db := seedLibrary(t)
	defer db.Close()
	var albumID int64
	db.QueryRow(`SELECT id FROM album WHERE name = 'Abbey Road'`).Scan(&albumID)
	tracks, err := TracksByAlbumEnriched(db, albumID)
	if err != nil {
		t.Fatalf("TracksByAlbumEnriched: %v", err)
	}
	if len(tracks) != 2 || tracks[0].Code != "A1" || tracks[1].Code != "A2" {
		t.Fatalf("album tracks: got %+v", tracks)
	}
}

func TestCounts(t *testing.T) {
	db := seedLibrary(t)
	defer db.Close()
	if n, _ := CountAlbums(db, ""); n != 3 {
		t.Errorf("CountAlbums = %d, want 3", n)
	}
	if n, _ := CountTracks(db, ""); n != 4 {
		t.Errorf("CountTracks = %d, want 4", n)
	}
	if n, _ := CountArtists(db, ""); n != 3 {
		t.Errorf("CountArtists = %d, want 3", n)
	}
	if n, _ := CountTracks(db, "Abbey"); n != 2 {
		t.Errorf("CountTracks(Abbey) = %d, want 2", n)
	}
}

func TestListArtistsEnriched(t *testing.T) {
	db := seedLibrary(t)
	defer db.Close()
	artists, err := ListArtistsEnriched(db, "", 50, 0)
	if err != nil {
		t.Fatalf("ListArtistsEnriched: %v", err)
	}
	// Ordered by sort_key: ABBA, The Beatles ("beatles"), Ziggy.
	if len(artists) != 3 || artists[0].Name != "ABBA" || artists[1].Name != "The Beatles" {
		t.Fatalf("artist order: got %+v", artists)
	}
	if artists[1].AlbumCount != 1 || artists[1].TrackCount != 2 {
		t.Errorf("Beatles counts = %d albums %d tracks, want 1/2",
			artists[1].AlbumCount, artists[1].TrackCount)
	}
}

func TestPagingRanksStable(t *testing.T) {
	db := seedLibrary(t)
	defer db.Close()
	// Letters/codes must not shift when fetched a page at a time.
	full, _ := ListTracksEnriched(db, "", 50, 0)
	var paged []model.EnrichedTrack
	for off := 0; off < 4; off++ {
		page, _ := ListTracksEnriched(db, "", 1, off)
		paged = append(paged, page...)
	}
	if len(paged) != len(full) {
		t.Fatalf("paged %d != full %d", len(paged), len(full))
	}
	for i := range full {
		if paged[i].Code != full[i].Code {
			t.Errorf("track %d code drift: paged %q vs full %q", i, paged[i].Code, full[i].Code)
		}
	}
}
