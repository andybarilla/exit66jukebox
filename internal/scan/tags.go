package scan

import (
	"os"

	"github.com/dhowden/tag"
)

// Meta is the subset of tag data the index stores.
type Meta struct {
	Title   string
	Artist  string
	Album   string
	Genre   string
	TrackNo int
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
		Title:   m.Title(),
		Artist:  m.Artist(),
		Album:   m.Album(),
		Genre:   m.Genre(),
		TrackNo: trackNo,
	}
	return normalize(meta, path), nil
}

func normalize(m Meta, path string) Meta {
	if m.Title == "" {
		m.Title = baseName(path)
	}
	if m.Artist == "" {
		m.Artist = "Unknown Artist"
	}
	if m.Album == "" {
		m.Album = "Unknown Album"
	}
	return m
}

func baseName(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[i+1:]
		}
	}
	return path
}
