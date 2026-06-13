package scan

import "testing"

func TestReadTagsReturnsArtistAndTitle(t *testing.T) {
	meta, err := ReadTags("testdata/sample.mp3")
	if err != nil {
		t.Fatalf("ReadTags: %v", err)
	}
	if meta.Artist == "" {
		t.Errorf("expected a non-empty artist")
	}
	if meta.Title == "" {
		t.Errorf("expected a non-empty title")
	}
}

func TestAlbumArtistOrFallback(t *testing.T) {
	if got := (Meta{Artist: "Track A", AlbumArtist: "Various Artists"}).AlbumArtistOrFallback(); got != "Various Artists" {
		t.Errorf("with AlbumArtist tag: expected %q, got %q", "Various Artists", got)
	}
	if got := (Meta{Artist: "Track A"}).AlbumArtistOrFallback(); got != "Track A" {
		t.Errorf("without AlbumArtist tag: expected fallback to track artist %q, got %q", "Track A", got)
	}
}

func TestReadTagsUnknownFallback(t *testing.T) {
	meta, err := ReadTags("testdata/sample.mp3")
	if err != nil {
		t.Fatalf("ReadTags: %v", err)
	}
	if meta.Album == "" {
		t.Errorf("album should fall back to a placeholder, got empty")
	}
}
