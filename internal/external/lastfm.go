package external

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
)

// lastFMEndpoint is the single Last.fm API 2.0 root; every method POSTs here.
const lastFMEndpoint = "https://ws.audioscrobbler.com/2.0/"

// scrobbleBatchMax is Last.fm's cap on listens per track.scrobble request.
const scrobbleBatchMax = 50

// ErrServiceDisabled signals that a scrobble service has self-disabled (Last.fm
// after an invalid session key) and its queued rows should be left intact rather
// than retried or deleted. The drainer treats it as a skip, not a failure. It
// lives here, not in the scrobble package, because the Last.fm client returns it
// and external cannot import scrobble.
var ErrServiceDisabled = errors.New("scrobble service disabled (re-auth required)")

// Lastfm scrobbles to Last.fm: md5-signed, form-encoded POSTs through the shared
// rate-limited client. A client with no session key (or one invalidated by a
// Last.fm error 9) is unauthorized: NowPlaying/Submit no-op with ErrServiceDisabled.
type Lastfm struct {
	c          *Client
	apiKey     string
	apiSecret  string
	sessionKey string
	baseURL    string // overridable in tests
	authorized atomic.Bool
	onDisabled func() // invoked once when an error 9 disables the service
}

// NewLastfm wraps the shared client with Last.fm credentials. The client is
// authorized when a session key is present; pass "" for the one-time auth flow.
func NewLastfm(c *Client, apiKey, apiSecret, sessionKey string) *Lastfm {
	l := &Lastfm{c: c, apiKey: apiKey, apiSecret: apiSecret, sessionKey: sessionKey, baseURL: lastFMEndpoint}
	l.authorized.Store(sessionKey != "")
	return l
}

// SetOnDisabled registers a callback fired once when an invalid session key
// disables the service (main wires it to clear the persisted service_auth row).
func (l *Lastfm) SetOnDisabled(fn func()) { l.onDisabled = fn }

// Authorized reports whether the client currently holds a usable session key.
// It is the live gate for both now-playing and enqueue.
func (l *Lastfm) Authorized() bool { return l.authorized.Load() }

// NowPlaying sends track.updateNowPlaying, fire-and-forget at the call site. It
// no-ops with ErrServiceDisabled when unauthorized.
func (l *Lastfm) NowPlaying(ctx context.Context, meta ListenMeta) error {
	if !l.authorized.Load() {
		return ErrServiceDisabled
	}
	params := map[string]string{
		"method":  "track.updateNowPlaying",
		"api_key": l.apiKey,
		"sk":      l.sessionKey,
		"artist":  meta.ArtistName,
		"track":   meta.TrackName,
	}
	if meta.ReleaseName != "" {
		params["album"] = meta.ReleaseName
	}
	return l.post(ctx, params, nil)
}

// Submit delivers completed listens via track.scrobble with indexed array
// params, chunked at scrobbleBatchMax. It satisfies scrobble.Submitter. An
// unauthorized client returns ErrServiceDisabled without a request.
func (l *Lastfm) Submit(ctx context.Context, listens []Listen) error {
	if !l.authorized.Load() {
		return ErrServiceDisabled
	}
	for start := 0; start < len(listens); start += scrobbleBatchMax {
		end := min(start+scrobbleBatchMax, len(listens))
		params := map[string]string{
			"method":  "track.scrobble",
			"api_key": l.apiKey,
			"sk":      l.sessionKey,
		}
		for i, ln := range listens[start:end] {
			params[fmt.Sprintf("artist[%d]", i)] = ln.Meta.ArtistName
			params[fmt.Sprintf("track[%d]", i)] = ln.Meta.TrackName
			params[fmt.Sprintf("timestamp[%d]", i)] = strconv.FormatInt(ln.ListenedAt, 10)
			if ln.Meta.ReleaseName != "" {
				params[fmt.Sprintf("album[%d]", i)] = ln.Meta.ReleaseName
			}
		}
		if err := l.post(ctx, params, nil); err != nil {
			return err
		}
	}
	return nil
}

// SimilarArtist is one artist.getSimilar hit: a name, optional MBID, and a
// 0–1 similarity score.
type SimilarArtist struct {
	Name  string
	MBID  string
	Match float64
}

// SimilarArtists fetches artists similar to the given name (or MBID, preferred
// when set). It is a read method: GET with api_key only — no session key and no
// signature — so it works whenever Last.fm is configured, independent of the
// scrobble auth flow.
func (l *Lastfm) SimilarArtists(ctx context.Context, name, mbid string, limit int) ([]SimilarArtist, error) {
	q := url.Values{}
	q.Set("method", "artist.getSimilar")
	q.Set("api_key", l.apiKey)
	q.Set("format", "json")
	q.Set("limit", strconv.Itoa(limit))
	if mbid != "" {
		q.Set("mbid", mbid)
	} else {
		q.Set("artist", name)
	}
	var resp struct {
		SimilarArtists struct {
			Artist []struct {
				Name  string `json:"name"`
				MBID  string `json:"mbid"`
				Match string `json:"match"`
			} `json:"artist"`
		} `json:"similarartists"`
	}
	if err := l.c.getJSON(ctx, l.baseURL+"?"+q.Encode(), &resp); err != nil {
		return nil, err
	}
	out := make([]SimilarArtist, len(resp.SimilarArtists.Artist))
	for i, a := range resp.SimilarArtists.Artist {
		match, _ := strconv.ParseFloat(a.Match, 64)
		out[i] = SimilarArtist{Name: a.Name, MBID: a.MBID, Match: match}
	}
	return out, nil
}

// GetToken runs auth.getToken, the first step of the desktop auth flow.
func (l *Lastfm) GetToken(ctx context.Context) (string, error) {
	var out struct {
		Token string `json:"token"`
	}
	if err := l.post(ctx, map[string]string{"method": "auth.getToken", "api_key": l.apiKey}, &out); err != nil {
		return "", err
	}
	return out.Token, nil
}

// GetSession runs auth.getSession once the user has approved the token, returning
// the durable session key + username.
func (l *Lastfm) GetSession(ctx context.Context, token string) (sessionKey, username string, err error) {
	var out struct {
		Session struct {
			Name string `json:"name"`
			Key  string `json:"key"`
		} `json:"session"`
	}
	if err := l.post(ctx, map[string]string{
		"method": "auth.getSession", "api_key": l.apiKey, "token": token}, &out); err != nil {
		return "", "", err
	}
	return out.Session.Key, out.Session.Name, nil
}

// AuthorizeURL is the last.fm page the user visits to approve a token.
func (l *Lastfm) AuthorizeURL(token string) string {
	return "https://www.last.fm/api/auth/?api_key=" + url.QueryEscape(l.apiKey) +
		"&token=" + url.QueryEscape(token)
}

// post signs params, form-encodes them, and POSTs through the shared limiter +
// retry/backoff. It decodes Last.fm's JSON error envelope first: a non-zero
// error code becomes a Go error (error 9 also disables the client), otherwise
// the body is decoded into out when non-nil.
func (l *Lastfm) post(ctx context.Context, params map[string]string, out any) error {
	params["format"] = "json"
	params["api_sig"] = signParams(params, l.apiSecret)
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	body := form.Encode()
	resp, err := l.c.doRequest(ctx, http.MethodPost, l.baseURL,
		func() io.Reader { return strings.NewReader(body) },
		func(h http.Header) { h.Set("Content-Type", "application/x-www-form-urlencoded") })
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return err
	}
	var envelope struct {
		Error   int    `json:"error"`
		Message string `json:"message"`
	}
	json.Unmarshal(raw, &envelope)
	if envelope.Error != 0 {
		return l.apiError(envelope.Error, envelope.Message)
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("lastfm: %s", resp.Status)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// apiError maps a Last.fm error code to a Go error. Code 9 (invalid session key)
// disables the client and wraps ErrServiceDisabled so the drainer leaves rows
// queued instead of retrying.
func (l *Lastfm) apiError(code int, message string) error {
	if code == 9 {
		l.disable()
		return fmt.Errorf("lastfm error 9 (invalid session key): %s: %w", message, ErrServiceDisabled)
	}
	return fmt.Errorf("lastfm error %d: %s", code, message)
}

// disable flips the client unauthorized and fires onDisabled exactly once.
func (l *Lastfm) disable() {
	if l.authorized.Swap(false) {
		log.Print("lastfm: invalid session key (error 9); disabling until re-auth (run `exit66 lastfm-auth`)")
		if l.onDisabled != nil {
			l.onDisabled()
		}
	}
}

// signParams computes a Last.fm api_sig: every request param except format and
// callback, sorted by name and concatenated as name+value, with the shared
// secret appended, hashed with md5. (Last.fm API auth spec.)
func signParams(params map[string]string, secret string) string {
	names := make([]string, 0, len(params))
	for name := range params {
		if name == "format" || name == "callback" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	var b []byte
	for _, name := range names {
		b = append(b, name...)
		b = append(b, params[name]...)
	}
	b = append(b, secret...)
	sum := md5.Sum(b)
	return hex.EncodeToString(sum[:])
}
