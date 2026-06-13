package external

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLastfmSimilarArtists(t *testing.T) {
	var gotMethod, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotQuery = r.URL.RawQuery
		w.Write([]byte(`{"similarartists":{"artist":[
			{"name":"Boards of Canada","mbid":"mbid-boc","match":"1.0"},
			{"name":"Aphex Twin","mbid":"","match":"0.62"}
		]}}`))
	}))
	defer srv.Close()

	c, _ := newTestClient(0)
	lf := NewLastfm(c, "key-123", "secret", "")
	lf.baseURL = srv.URL

	got, err := lf.SimilarArtists(context.Background(), "Autechre", "", 10)
	if err != nil {
		t.Fatalf("SimilarArtists: %v", err)
	}
	// getSimilar is a read method: GET, api_key only, no signature/session.
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	for _, banned := range []string{"api_sig", "sk="} {
		if contains(gotQuery, banned) {
			t.Errorf("query %q must not carry %q (unsigned read path)", gotQuery, banned)
		}
	}
	for _, want := range []string{"method=artist.getSimilar", "api_key=key-123", "artist=Autechre", "format=json"} {
		if !contains(gotQuery, want) {
			t.Errorf("query %q missing %q", gotQuery, want)
		}
	}
	if len(got) != 2 {
		t.Fatalf("artists = %d, want 2", len(got))
	}
	if got[0].Name != "Boards of Canada" || got[0].MBID != "mbid-boc" || got[0].Match != 1.0 {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[1].Name != "Aphex Twin" || got[1].Match != 0.62 {
		t.Errorf("got[1] = %+v", got[1])
	}
}

func TestLastfmSimilarArtistsByMBID(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Write([]byte(`{"similarartists":{"artist":[]}}`))
	}))
	defer srv.Close()

	c, _ := newTestClient(0)
	lf := NewLastfm(c, "key", "secret", "")
	lf.baseURL = srv.URL

	if _, err := lf.SimilarArtists(context.Background(), "", "the-mbid", 5); err != nil {
		t.Fatalf("SimilarArtists: %v", err)
	}
	if !contains(gotQuery, "mbid=the-mbid") {
		t.Errorf("query %q should carry mbid", gotQuery)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
