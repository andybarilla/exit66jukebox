package external

import "context"

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
