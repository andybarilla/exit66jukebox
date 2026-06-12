package external

import (
	"context"
	"net/url"
	"strings"
)

const musicBrainzBaseURL = "https://musicbrainz.org/ws/2"

// MB queries the MusicBrainz recording search API.
type MB struct {
	c       *Client
	baseURL string // overridable in tests
}

// NewMusicBrainz wraps a rate-limited client.
func NewMusicBrainz(c *Client) *MB {
	return &MB{c: c, baseURL: musicBrainzBaseURL}
}

// RecordingMatch is the subset of a recording search hit the enrichment pass
// uses. Score is MusicBrainz's 0–100 confidence in the match.
type RecordingMatch struct {
	Score          int
	RecordingMBID  string
	RecordingTitle string
	ArtistMBID     string
	ArtistName     string
	ReleaseMBID    string
	ReleaseTitle   string
}

// recordingSearchResponse mirrors the JSON fields we read.
type recordingSearchResponse struct {
	Recordings []struct {
		ID           string `json:"id"`
		Score        int    `json:"score"`
		Title        string `json:"title"`
		ArtistCredit []struct {
			Artist struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"artist"`
		} `json:"artist-credit"`
		Releases []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"releases"`
	} `json:"recordings"`
}

// SearchRecording finds the single best recording matching the supplied tags.
// Empty or placeholder fields are omitted from the Lucene query. ok is false
// when MusicBrainz returns no recordings; the caller applies the score
// threshold.
func (m *MB) SearchRecording(ctx context.Context, artist, title, album string) (RecordingMatch, bool, error) {
	query := buildLuceneQuery(artist, title, album)
	if query == "" {
		return RecordingMatch{}, false, nil
	}
	q := url.Values{}
	q.Set("query", query)
	q.Set("fmt", "json")
	q.Set("limit", "1")
	u := m.baseURL + "/recording?" + q.Encode()

	var resp recordingSearchResponse
	if err := m.c.getJSON(ctx, u, &resp); err != nil {
		return RecordingMatch{}, false, err
	}
	if len(resp.Recordings) == 0 {
		return RecordingMatch{}, false, nil
	}
	r := resp.Recordings[0]
	match := RecordingMatch{
		Score:          r.Score,
		RecordingMBID:  r.ID,
		RecordingTitle: r.Title,
	}
	if len(r.ArtistCredit) > 0 {
		match.ArtistMBID = r.ArtistCredit[0].Artist.ID
		match.ArtistName = r.ArtistCredit[0].Artist.Name
	}
	if len(r.Releases) > 0 {
		match.ReleaseMBID = r.Releases[0].ID
		match.ReleaseTitle = r.Releases[0].Title
	}
	return match, true, nil
}

// buildLuceneQuery joins the non-placeholder fields into a quoted AND query.
func buildLuceneQuery(artist, title, album string) string {
	var terms []string
	if t := luceneTerm("recording", title); t != "" {
		terms = append(terms, t)
	}
	if t := luceneTerm("artist", artist); t != "" {
		terms = append(terms, t)
	}
	if t := luceneTerm("release", album); t != "" {
		terms = append(terms, t)
	}
	return strings.Join(terms, " AND ")
}

// placeholders are the synthetic tag values the scanner writes for blank tags;
// they carry no real information so they are dropped from queries.
var placeholders = map[string]bool{
	"":               true,
	"Unknown Artist": true,
	"Unknown Album":  true,
}

func luceneTerm(field, value string) string {
	value = strings.TrimSpace(value)
	if placeholders[value] {
		return ""
	}
	// Escape embedded quotes/backslashes so the quoted phrase stays well-formed.
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return field + `:"` + value + `"`
}
