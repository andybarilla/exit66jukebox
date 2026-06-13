package external

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type capturedSubmit struct {
	auth string
	path string
	body struct {
		ListenType string `json:"listen_type"`
		Payload    []struct {
			ListenedAt int64 `json:"listened_at"`
			TrackMeta  struct {
				ArtistName  string `json:"artist_name"`
				TrackName   string `json:"track_name"`
				ReleaseName string `json:"release_name"`
			} `json:"track_metadata"`
		} `json:"payload"`
	}
	// rawHasListenedAt records whether listened_at appeared in the wire payload.
	rawHasListenedAt bool
}

func lbTestServer(t *testing.T, cap *capturedSubmit) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.auth = r.Header.Get("Authorization")
		cap.path = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		var probe struct {
			Payload []map[string]json.RawMessage `json:"payload"`
		}
		json.Unmarshal(raw, &probe)
		if len(probe.Payload) > 0 {
			_, cap.rawHasListenedAt = probe.Payload[0]["listened_at"]
		}
		json.Unmarshal(raw, &cap.body)
		w.Write([]byte(`{"status":"ok"}`))
	}))
}

func TestListenBrainzNowPlaying(t *testing.T) {
	var cap capturedSubmit
	srv := lbTestServer(t, &cap)
	defer srv.Close()

	c, _ := newTestClient(0)
	lb := NewListenBrainz(c, "tok-xyz")
	lb.baseURL = srv.URL

	err := lb.NowPlaying(context.Background(), ListenMeta{
		ArtistName: "A", TrackName: "T", ReleaseName: "R"})
	if err != nil {
		t.Fatalf("NowPlaying: %v", err)
	}
	if cap.path != "/1/submit-listens" {
		t.Errorf("path = %q, want /1/submit-listens", cap.path)
	}
	if cap.auth != "Token tok-xyz" {
		t.Errorf("auth = %q, want Token tok-xyz", cap.auth)
	}
	if cap.body.ListenType != "playing_now" {
		t.Errorf("listen_type = %q, want playing_now", cap.body.ListenType)
	}
	if len(cap.body.Payload) != 1 {
		t.Fatalf("payload len = %d, want 1", len(cap.body.Payload))
	}
	if cap.rawHasListenedAt {
		t.Error("playing_now must omit listened_at")
	}
	if cap.body.Payload[0].TrackMeta.TrackName != "T" {
		t.Errorf("track_name = %q, want T", cap.body.Payload[0].TrackMeta.TrackName)
	}
}

func TestListenBrainzSubmitBatch(t *testing.T) {
	var cap capturedSubmit
	srv := lbTestServer(t, &cap)
	defer srv.Close()

	c, _ := newTestClient(0)
	lb := NewListenBrainz(c, "tok")
	lb.baseURL = srv.URL

	listens := []Listen{
		{ListenedAt: 1000, Meta: ListenMeta{ArtistName: "A1", TrackName: "T1", ReleaseName: "R1"}},
		{ListenedAt: 2000, Meta: ListenMeta{ArtistName: "A2", TrackName: "T2"}},
	}
	if err := lb.Submit(context.Background(), listens); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	// The ListenBrainz spec requires "import" (not "single") for multi-listen
	// payloads; "single" accepts exactly one listen.
	if cap.body.ListenType != "import" {
		t.Errorf("listen_type = %q, want import for a batch", cap.body.ListenType)
	}
	if len(cap.body.Payload) != 2 {
		t.Fatalf("payload len = %d, want 2", len(cap.body.Payload))
	}
	if !cap.rawHasListenedAt {
		t.Error("completed listens must carry listened_at")
	}
	if cap.body.Payload[0].ListenedAt != 1000 || cap.body.Payload[1].ListenedAt != 2000 {
		t.Errorf("listened_at = %d,%d want 1000,2000",
			cap.body.Payload[0].ListenedAt, cap.body.Payload[1].ListenedAt)
	}
}

func TestListenBrainzSubmitServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	c, _ := newTestClient(0)
	lb := NewListenBrainz(c, "tok")
	lb.baseURL = srv.URL
	if err := lb.Submit(context.Background(), []Listen{{ListenedAt: 1, Meta: ListenMeta{TrackName: "x"}}}); err == nil {
		t.Fatal("expected error on 400")
	}
}

func TestListenBrainzSubmitEmptyNoop(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()
	c, _ := newTestClient(0)
	lb := NewListenBrainz(c, "tok")
	lb.baseURL = srv.URL
	if err := lb.Submit(context.Background(), nil); err != nil {
		t.Fatalf("Submit(nil): %v", err)
	}
	if called {
		t.Error("Submit with no listens must not hit the network")
	}
}
