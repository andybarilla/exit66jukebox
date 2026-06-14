package scan

import (
	"reflect"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

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
	// A set compilation flag forces "Various Artists" when AlbumArtist is blank.
	if got := (Meta{Artist: "Track A", Compilation: true}).AlbumArtistOrFallback(); got != store.VariousArtists {
		t.Errorf("compilation without AlbumArtist tag: expected %q, got %q", store.VariousArtists, got)
	}
	// An explicit AlbumArtist tag still wins over the compilation flag.
	if got := (Meta{Artist: "Track A", AlbumArtist: "Real Band", Compilation: true}).AlbumArtistOrFallback(); got != "Real Band" {
		t.Errorf("compilation with AlbumArtist tag: expected %q, got %q", "Real Band", got)
	}
}

func TestCompilationFlag(t *testing.T) {
	cases := []struct {
		name string
		raw  map[string]interface{}
		want bool
	}{
		{"id3 TCMP set", map[string]interface{}{"TCMP": "1"}, true},
		{"id3 TCMP unset", map[string]interface{}{"TCMP": "0"}, false},
		{"vorbis compilation set", map[string]interface{}{"compilation": "1"}, true},
		{"vorbis compilation unset", map[string]interface{}{"compilation": "0"}, false},
		{"mp4 cpil set", map[string]interface{}{"cpil": 1}, true},
		{"mp4 cpil unset", map[string]interface{}{"cpil": 0}, false},
		{"bool true", map[string]interface{}{"compilation": true}, true},
		{"absent", map[string]interface{}{}, false},
		{"blank string", map[string]interface{}{"TCMP": ""}, false},
	}
	for _, c := range cases {
		if got := compilationFlag(c.raw); got != c.want {
			t.Errorf("%s: compilationFlag = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestExtractLinks(t *testing.T) {
	cases := []struct {
		name    string
		comment string
		want    []string
	}{
		{"empty", "", nil},
		{"no url", "Visit our merch table", nil},
		{"single http", "Buy at http://example.com/album now", []string{"http://example.com/album"}},
		{"single https", "https://artist.bandcamp.com/album/foo", []string{"https://artist.bandcamp.com/album/foo"}},
		{
			"multiple distinct",
			"https://a.bandcamp.com/track/x and http://b.com/y",
			[]string{"https://a.bandcamp.com/track/x", "http://b.com/y"},
		},
		{
			"dedupe preserves first order",
			"https://a.com/x then https://a.com/x again, https://b.com/y",
			[]string{"https://a.com/x", "https://b.com/y"},
		},
		{
			"trailing punctuation trimmed",
			"see (https://a.com/x). thanks",
			[]string{"https://a.com/x"},
		},
		{
			"bare trailing period",
			"Visit https://a.com/x. Thanks",
			[]string{"https://a.com/x"},
		},
		{
			"trailing comma in list",
			"https://a.com/x, https://b.com/y",
			[]string{"https://a.com/x", "https://b.com/y"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractLinks(c.comment)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("extractLinks(%q) = %#v, want %#v", c.comment, got, c.want)
			}
		})
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
