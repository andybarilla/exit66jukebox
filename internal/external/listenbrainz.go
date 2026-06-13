package external

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

const listenBrainzBaseURL = "https://api.listenbrainz.org"

// ListenBrainz submits listens to the ListenBrainz API on behalf of one user.
type ListenBrainz struct {
	c       *Client
	token   string
	baseURL string // overridable in tests
}

// NewListenBrainz wraps a rate-limited client with a user token.
func NewListenBrainz(c *Client, token string) *ListenBrainz {
	return &ListenBrainz{c: c, token: token, baseURL: listenBrainzBaseURL}
}

// ListenMeta is the track identity carried in a listen submission.
type ListenMeta struct {
	ArtistName  string
	TrackName   string
	ReleaseName string
}

// Listen is one completed listen: when it was played plus what was played.
type Listen struct {
	ListenedAt int64
	Meta       ListenMeta
}

type lbTrackMeta struct {
	ArtistName  string `json:"artist_name"`
	TrackName   string `json:"track_name"`
	ReleaseName string `json:"release_name,omitempty"`
}

type lbPayload struct {
	ListenedAt int64       `json:"listened_at,omitempty"`
	TrackMeta  lbTrackMeta `json:"track_metadata"`
}

type lbSubmit struct {
	ListenType string      `json:"listen_type"`
	Payload    []lbPayload `json:"payload"`
}

func trackMeta(m ListenMeta) lbTrackMeta {
	return lbTrackMeta{ArtistName: m.ArtistName, TrackName: m.TrackName, ReleaseName: m.ReleaseName}
}

// NowPlaying sends a playing_now notification. It is fire-and-forget at the call
// site; listened_at is omitted as the spec requires.
func (l *ListenBrainz) NowPlaying(ctx context.Context, meta ListenMeta) error {
	body := lbSubmit{
		ListenType: "playing_now",
		Payload:    []lbPayload{{TrackMeta: trackMeta(meta)}},
	}
	return l.post(ctx, body)
}

// Submit delivers completed listens as one batched request. ListenBrainz uses
// listen_type "import" for multi-listen payloads ("single" accepts exactly one),
// so the durable-queue drainer always submits as "import". A nil/empty batch is
// a no-op.
func (l *ListenBrainz) Submit(ctx context.Context, listens []Listen) error {
	if len(listens) == 0 {
		return nil
	}
	payload := make([]lbPayload, len(listens))
	for i, ln := range listens {
		payload[i] = lbPayload{ListenedAt: ln.ListenedAt, TrackMeta: trackMeta(ln.Meta)}
	}
	return l.post(ctx, lbSubmit{ListenType: "import", Payload: payload})
}

func (l *ListenBrainz) post(ctx context.Context, body lbSubmit) error {
	return l.c.postJSON(ctx, l.baseURL+"/1/submit-listens",
		map[string]string{"Authorization": "Token " + l.token}, body, nil)
}

// Username resolves the token's owner via GET /1/validate-token. The
// recommendation endpoint is keyed by username, so this derives it from the
// existing token rather than requiring separate config. It errors when the
// token is invalid.
func (l *ListenBrainz) Username(ctx context.Context) (string, error) {
	var out struct {
		Valid    bool   `json:"valid"`
		UserName string `json:"user_name"`
	}
	if err := l.c.getJSONAuth(ctx, l.baseURL+"/1/validate-token",
		map[string]string{"Authorization": "Token " + l.token}, &out); err != nil {
		return "", err
	}
	if !out.Valid || out.UserName == "" {
		return "", fmt.Errorf("listenbrainz: token invalid")
	}
	return out.UserName, nil
}

// RecRecording is one collaborative-filtered recommendation: a recording MBID
// and its confidence score. ListenBrainz returns MBIDs only (no names), so the
// caller maps these to local tracks by track.mbid.
type RecRecording struct {
	RecordingMBID string
	Score         float64
}

// Recommendations fetches up to count collaborative-filtered recording
// recommendations for user, in descending score order.
func (l *ListenBrainz) Recommendations(ctx context.Context, user string, count int) ([]RecRecording, error) {
	q := url.Values{}
	q.Set("count", strconv.Itoa(count))
	u := l.baseURL + "/1/cf/recommendation/user/" + url.PathEscape(user) + "/recording?" + q.Encode()

	var resp struct {
		Payload struct {
			MBIDs []struct {
				RecordingMBID string  `json:"recording_mbid"`
				Score         float64 `json:"score"`
			} `json:"mbids"`
		} `json:"payload"`
	}
	if err := l.c.getJSON(ctx, u, &resp); err != nil {
		return nil, err
	}
	out := make([]RecRecording, len(resp.Payload.MBIDs))
	for i, m := range resp.Payload.MBIDs {
		out[i] = RecRecording{RecordingMBID: m.RecordingMBID, Score: m.Score}
	}
	return out, nil
}
