package scan

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/andybarilla/exit66jukebox/internal/store"
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
	Compilation bool
}

// AlbumArtistOrFallback returns the album's grouping artist, resolved in order:
// the AlbumArtist tag → "Various Artists" when the compilation flag is set →
// the track Artist. This is the key that collapses a compilation (or incidental
// single-artist duplicates) into one album card.
func (m Meta) AlbumArtistOrFallback() string {
	if m.AlbumArtist != "" {
		return m.AlbumArtist
	}
	if m.Compilation {
		return store.VariousArtists
	}
	return m.Artist
}

// compilationFlag reports whether raw tag data carries a set iTunes compilation
// flag. It checks each container's key: ID3 "TCMP" (string), MP4 "cpil" (int),
// and Vorbis "compilation" (string). A value is "set" when non-empty/non-zero.
func compilationFlag(raw map[string]interface{}) bool {
	for _, k := range []string{"TCMP", "cpil", "compilation"} {
		switch v := raw[k].(type) {
		case string:
			if s := strings.TrimSpace(v); s != "" && s != "0" {
				return true
			}
		case int:
			if v != 0 {
				return true
			}
		case bool:
			if v {
				return true
			}
		}
	}
	return false
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
		Compilation: compilationFlag(m.Raw()),
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
