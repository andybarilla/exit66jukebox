package scan

import (
	"os"
	"path/filepath"
	"regexp"
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
	Links       []string
	Compilation bool
}

type scannedTrack struct {
	path string
	meta Meta
}

type albumFolderKey struct {
	dir   string
	album string
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
		// Trim sentence punctuation the regex swallows when a URL ends a clause
		// ("Visit https://x/a." / "https://x/a, https://y/b").
		u = strings.TrimRight(u, ".,;:!?")
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return out
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

func resolveScanAlbumArtists(records []scannedTrack, assumeSameTitleFolderCompilations bool) []string {
	albumArtists := make([]string, len(records))
	if !assumeSameTitleFolderCompilations {
		for i, record := range records {
			albumArtists[i] = record.meta.AlbumArtistOrFallback()
		}
		return albumArtists
	}

	artistsByGroup := make(map[albumFolderKey]map[string]bool)
	for _, record := range records {
		if record.meta.AlbumArtist != "" || record.meta.Compilation {
			continue
		}
		key := albumFolderKey{dir: filepath.Dir(record.path), album: record.meta.Album}
		if artistsByGroup[key] == nil {
			artistsByGroup[key] = map[string]bool{}
		}
		artistsByGroup[key][record.meta.Artist] = true
	}

	for i, record := range records {
		albumArtists[i] = record.meta.AlbumArtistOrFallback()
		if record.meta.AlbumArtist != "" || record.meta.Compilation {
			continue
		}
		key := albumFolderKey{dir: filepath.Dir(record.path), album: record.meta.Album}
		if len(artistsByGroup[key]) > 1 {
			albumArtists[i] = store.VariousArtists
		}
	}
	return albumArtists
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
		Links:       extractLinks(m.Comment()),
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
