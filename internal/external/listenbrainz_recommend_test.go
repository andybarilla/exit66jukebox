package external

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListenBrainzUsername(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte(`{"code":200,"message":"Token valid.","valid":true,"user_name":"alice"}`))
	}))
	defer srv.Close()

	c, _ := newTestClient(0)
	lb := NewListenBrainz(c, "tok-xyz")
	lb.baseURL = srv.URL

	name, err := lb.Username(context.Background())
	if err != nil {
		t.Fatalf("Username: %v", err)
	}
	if gotPath != "/1/validate-token" {
		t.Errorf("path = %q, want /1/validate-token", gotPath)
	}
	if gotAuth != "Token tok-xyz" {
		t.Errorf("auth = %q, want Token tok-xyz", gotAuth)
	}
	if name != "alice" {
		t.Errorf("name = %q, want alice", name)
	}
}

func TestListenBrainzUsernameInvalidToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"code":200,"message":"Token invalid.","valid":false}`))
	}))
	defer srv.Close()

	c, _ := newTestClient(0)
	lb := NewListenBrainz(c, "bad")
	lb.baseURL = srv.URL

	if _, err := lb.Username(context.Background()); err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestListenBrainzRecommendations(t *testing.T) {
	var gotPath, gotQuery, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte(`{"payload":{"mbids":[
			{"recording_mbid":"rec-1","score":0.91},
			{"recording_mbid":"rec-2","score":0.42}
		],"user_name":"alice","count":2}}`))
	}))
	defer srv.Close()

	c, _ := newTestClient(0)
	lb := NewListenBrainz(c, "tok")
	lb.baseURL = srv.URL

	recs, err := lb.Recommendations(context.Background(), "alice", 25)
	if err != nil {
		t.Fatalf("Recommendations: %v", err)
	}
	if gotPath != "/1/cf/recommendation/user/alice/recording" {
		t.Errorf("path = %q", gotPath)
	}
	if gotQuery != "count=25" {
		t.Errorf("query = %q, want count=25", gotQuery)
	}
	// The token is sent as a fail-safe: harmless on a public endpoint, required
	// if ListenBrainz gates per-user recommendations behind auth.
	if gotAuth != "Token tok" {
		t.Errorf("auth = %q, want Token tok", gotAuth)
	}
	if len(recs) != 2 {
		t.Fatalf("recs = %d, want 2", len(recs))
	}
	if recs[0].RecordingMBID != "rec-1" || recs[0].Score != 0.91 {
		t.Errorf("recs[0] = %+v", recs[0])
	}
	if recs[1].RecordingMBID != "rec-2" {
		t.Errorf("recs[1] = %+v", recs[1])
	}
}
