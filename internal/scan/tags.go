package scan

import (
	"os"
	"path/filepath"
	"regexp"

	"github.com/dhowden/tag"
)

// Meta is the subset of tag data the index stores.
type Meta struct {
	Title       string
	Artist      string
	AlbumArtist string
	Album       string
	Genre       string
	TrackNo     int
	Links       []string
}

// urlRe matches http(s) URLs in free text. It stops at whitespace and at
// trailing punctuation that commonly wraps a link in prose (closing brackets,
// quotes, sentence enders), so "(https://x.com/a)." yields the bare URL.
var urlRe = regexp.MustCompile(`https?://[^\s<>"')\]}]+`)

// extractLinks pulls every distinct http(s) URL out of a comment string,
// preserving first-seen order. Returns nil when none are found.
func extractLinks(comment string) []string {
	matches := urlRe.FindAllString(comment, -1)
	if matches == nil {
		return nil
	}
	seen := make(map[string]bool, len(matches))
	var out []string
	for _, u := range matches {
		if seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return out
}

// AlbumArtistOrFallback returns the album's grouping artist: the AlbumArtist tag
// when present, else the track Artist. This is the key that collapses a
// compilation (or incidental single-artist duplicates) into one album card.
func (m Meta) AlbumArtistOrFallback() string {
	if m.AlbumArtist != "" {
		return m.AlbumArtist
	}
	return m.Artist
}

// ReadTags reads tags from a single audio file, filling blanks with placeholders
// so the index never stores empty artist/album/title.
func ReadTags(path string) (Meta, error) {
	f, err := os.Open(path)
	if err != nil {
		return Meta{}, err
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err != nil {
		return Meta{}, err
	}
	trackNo, _ := m.Track()
	meta := Meta{
		Title:       m.Title(),
		Artist:      m.Artist(),
		AlbumArtist: m.AlbumArtist(),
		Album:       m.Album(),
		Genre:       m.Genre(),
		TrackNo:     trackNo,
		Links:       extractLinks(m.Comment()),
	}
	return normalize(meta, path), nil
}

func normalize(m Meta, path string) Meta {
	if m.Title == "" {
		m.Title = filepath.Base(path)
	}
	if m.Artist == "" {
		m.Artist = "Unknown Artist"
	}
	if m.Album == "" {
		m.Album = "Unknown Album"
	}
	return m
}
