package scan

import (
	"os"
	"path/filepath"

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
