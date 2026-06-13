package external

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// lfmServer is an httptest Last.fm endpoint. handler inspects the parsed POST
// form and writes a JSON response (or error envelope).
func lfmServer(t *testing.T, handler func(form map[string][]string) string) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q, want form-urlencoded", r.Header.Get("Content-Type"))
		}
		w.Write([]byte(handler(r.PostForm)))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func newTestLastfm(t *testing.T, baseURL, sessionKey string) *Lastfm {
	t.Helper()
	c, _ := newTestClient(0)
	l := NewLastfm(c, "api-key", "api-secret", sessionKey)
	l.baseURL = baseURL
	return l
}

// The api_sig sent on the wire must match a recomputation over the actual posted
// params (minus api_sig/format) — proving post() signs the right set, the single
// thing most likely to make real Last.fm reject every request.
func TestLastfmAssembledRequestIsCorrectlySigned(t *testing.T) {
	srv, _ := lfmServer(t, func(f map[string][]string) string {
		params := map[string]string{}
		for k, v := range f {
			if k == "api_sig" || k == "format" {
				continue
			}
			params[k] = v[0]
		}
		want := signParams(params, "api-secret")
		if got := f["api_sig"][0]; got != want {
			t.Errorf("api_sig = %q, want %q (over %v)", got, want, params)
		}
		return `{"nowplaying":{}}`
	})
	l := newTestLastfm(t, srv.URL, "sess")
	if err := l.NowPlaying(context.Background(), ListenMeta{ArtistName: "A", TrackName: "T"}); err != nil {
		t.Fatalf("NowPlaying: %v", err)
	}
}

func TestLastfmNowPlaying(t *testing.T) {
	srv, _ := lfmServer(t, func(f map[string][]string) string {
		if got := f["method"]; len(got) != 1 || got[0] != "track.updateNowPlaying" {
			t.Errorf("method = %v, want track.updateNowPlaying", got)
		}
		if f["sk"][0] != "sess" {
			t.Errorf("sk = %v, want sess", f["sk"])
		}
		if f["api_sig"][0] == "" {
			t.Error("missing api_sig")
		}
		if f["artist"][0] != "A" || f["track"][0] != "T" || f["album"][0] != "R" {
			t.Errorf("track params = %v", f)
		}
		return `{"nowplaying":{}}`
	})
	l := newTestLastfm(t, srv.URL, "sess")
	if err := l.NowPlaying(context.Background(), ListenMeta{ArtistName: "A", TrackName: "T", ReleaseName: "R"}); err != nil {
		t.Fatalf("NowPlaying: %v", err)
	}
}

func TestLastfmScrobbleIndexedBatch(t *testing.T) {
	srv, calls := lfmServer(t, func(f map[string][]string) string {
		if f["method"][0] != "track.scrobble" {
			t.Errorf("method = %v, want track.scrobble", f["method"])
		}
		if f["artist[0]"][0] != "A1" || f["track[0]"][0] != "T1" || f["timestamp[0]"][0] != "1000" {
			t.Errorf("index 0 params wrong: %v", f)
		}
		if f["artist[1]"][0] != "A2" || f["track[1]"][0] != "T2" || f["timestamp[1]"][0] != "2000" {
			t.Errorf("index 1 params wrong: %v", f)
		}
		return `{"scrobbles":{"@attr":{"accepted":2,"ignored":0}}}`
	})
	l := newTestLastfm(t, srv.URL, "sess")
	listens := []Listen{
		{ListenedAt: 1000, Meta: ListenMeta{ArtistName: "A1", TrackName: "T1", ReleaseName: "R1"}},
		{ListenedAt: 2000, Meta: ListenMeta{ArtistName: "A2", TrackName: "T2"}},
	}
	if err := l.Submit(context.Background(), listens); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if *calls != 1 {
		t.Errorf("calls = %d, want 1", *calls)
	}
}

// track.scrobble accepts at most 50 per request; a larger batch is chunked.
func TestLastfmScrobbleChunksOver50(t *testing.T) {
	srv, calls := lfmServer(t, func(f map[string][]string) string {
		return `{"scrobbles":{}}`
	})
	l := newTestLastfm(t, srv.URL, "sess")
	listens := make([]Listen, 60)
	for i := range listens {
		listens[i] = Listen{ListenedAt: int64(i), Meta: ListenMeta{ArtistName: "A", TrackName: "T"}}
	}
	if err := l.Submit(context.Background(), listens); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if *calls != 2 {
		t.Errorf("calls = %d, want 2 (50 + 10)", *calls)
	}
}

func TestLastfmGetTokenAndSession(t *testing.T) {
	srv, _ := lfmServer(t, func(f map[string][]string) string {
		switch f["method"][0] {
		case "auth.getToken":
			return `{"token":"tok-123"}`
		case "auth.getSession":
			if f["token"][0] != "tok-123" {
				t.Errorf("getSession token = %v, want tok-123", f["token"])
			}
			return `{"session":{"name":"alice","key":"sk-999","subscriber":0}}`
		}
		t.Errorf("unexpected method %v", f["method"])
		return `{}`
	})
	l := newTestLastfm(t, srv.URL, "")
	tok, err := l.GetToken(context.Background())
	if err != nil || tok != "tok-123" {
		t.Fatalf("GetToken = (%q, %v), want (tok-123, nil)", tok, err)
	}
	key, user, err := l.GetSession(context.Background(), tok)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if key != "sk-999" || user != "alice" {
		t.Errorf("GetSession = (%q, %q), want (sk-999, alice)", key, user)
	}
}

// auth.getSession with an unapproved token yields Last.fm error 14; the CLI flow
// must see it surfaced.
func TestLastfmGetSessionError14(t *testing.T) {
	srv, _ := lfmServer(t, func(f map[string][]string) string {
		return `{"error":14,"message":"Unauthorized Token"}`
	})
	l := newTestLastfm(t, srv.URL, "")
	_, _, err := l.GetSession(context.Background(), "tok")
	if err == nil {
		t.Fatal("expected error on error 14")
	}
	if !errorContains(err, "14") {
		t.Errorf("error = %v, want it to mention code 14", err)
	}
}

// Error 9 during a scrobble disables Last.fm: the error matches ErrServiceDisabled,
// the authorized flag flips false, onDisabled fires once, and a subsequent call
// short-circuits without hitting the network.
func TestLastfmInvalidSessionDisables(t *testing.T) {
	srv, calls := lfmServer(t, func(f map[string][]string) string {
		return `{"error":9,"message":"Invalid session key - Please re-authenticate"}`
	})
	l := newTestLastfm(t, srv.URL, "sess")
	var disabled int32
	l.SetOnDisabled(func() { atomic.AddInt32(&disabled, 1) })

	err := l.Submit(context.Background(), []Listen{{ListenedAt: 1, Meta: ListenMeta{TrackName: "x"}}})
	if !errors.Is(err, ErrServiceDisabled) {
		t.Fatalf("Submit error = %v, want ErrServiceDisabled", err)
	}
	if l.Authorized() {
		t.Error("Authorized() = true after error 9, want false")
	}
	if disabled != 1 {
		t.Errorf("onDisabled fired %d times, want 1", disabled)
	}
	callsAfterFirst := *calls
	// Already disabled: must not hit the network again.
	if err := l.Submit(context.Background(), []Listen{{ListenedAt: 2, Meta: ListenMeta{TrackName: "y"}}}); !errors.Is(err, ErrServiceDisabled) {
		t.Fatalf("second Submit = %v, want ErrServiceDisabled", err)
	}
	if *calls != callsAfterFirst {
		t.Errorf("disabled client made a network call: calls went %d -> %d", callsAfterFirst, *calls)
	}
}

// An unauthorized (no session key) client no-ops now-playing without a request.
func TestLastfmNowPlayingUnauthorizedNoNetwork(t *testing.T) {
	srv, calls := lfmServer(t, func(f map[string][]string) string { return `{}` })
	l := newTestLastfm(t, srv.URL, "")
	if err := l.NowPlaying(context.Background(), ListenMeta{TrackName: "x"}); !errors.Is(err, ErrServiceDisabled) {
		t.Fatalf("NowPlaying = %v, want ErrServiceDisabled", err)
	}
	if *calls != 0 {
		t.Errorf("unauthorized client hit the network %d times", *calls)
	}
}

func errorContains(err error, sub string) bool {
	return err != nil && strings.Contains(err.Error(), sub)
}


// Known vector: md5("api_keyabcmethodauth.getSessiontokenxyzsecret123"), i.e.
// params sorted by name, concatenated as name+value, secret appended.
func TestSignParamsKnownVector(t *testing.T) {
	got := signParams(map[string]string{
		"method":  "auth.getSession",
		"token":   "xyz",
		"api_key": "abc",
	}, "secret123")
	const want = "8117c5d4c40b151f6c064254246786da"
	if got != want {
		t.Errorf("signParams = %q, want %q", got, want)
	}
}

// format and callback are excluded from the signature base string per the
// Last.fm spec, so adding them must not change the api_sig.
func TestSignParamsExcludesFormatAndCallback(t *testing.T) {
	base := signParams(map[string]string{"api_key": "abc", "method": "m"}, "s")
	withExtras := signParams(map[string]string{
		"api_key": "abc", "method": "m", "format": "json", "callback": "cb",
	}, "s")
	if base != withExtras {
		t.Errorf("format/callback changed the signature: %q vs %q", base, withExtras)
	}
}
