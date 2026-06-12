package external

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const cannedRecordingJSON = `{
  "recordings": [
    {
      "id": "rec-mbid-1",
      "score": 97,
      "title": "Karma Police",
      "artist-credit": [
        {"artist": {"id": "art-mbid-1", "name": "Radiohead"}}
      ],
      "releases": [
        {"id": "rel-mbid-1", "title": "OK Computer"}
      ]
    }
  ]
}`

func TestSearchRecordingParses(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("query")
		w.Write([]byte(cannedRecordingJSON))
	}))
	defer srv.Close()

	c, _ := newTestClient(0)
	mb := NewMusicBrainz(c)
	mb.baseURL = srv.URL

	match, ok, err := mb.SearchRecording(context.Background(), "Radiohead", "Karma Police", "OK Computer")
	if err != nil {
		t.Fatalf("SearchRecording: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if match.Score != 97 || match.RecordingMBID != "rec-mbid-1" ||
		match.ArtistMBID != "art-mbid-1" || match.ArtistName != "Radiohead" ||
		match.ReleaseMBID != "rel-mbid-1" || match.ReleaseTitle != "OK Computer" {
		t.Fatalf("parsed %+v", match)
	}
	// The query should carry all three quoted, AND-joined terms.
	for _, want := range []string{`recording:"Karma Police"`, `artist:"Radiohead"`, `release:"OK Computer"`, " AND "} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query %q missing %q", gotQuery, want)
		}
	}
}

func TestSearchRecordingOmitsPlaceholders(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("query")
		w.Write([]byte(cannedRecordingJSON))
	}))
	defer srv.Close()

	c, _ := newTestClient(0)
	mb := NewMusicBrainz(c)
	mb.baseURL = srv.URL

	_, _, err := mb.SearchRecording(context.Background(), "Unknown Artist", "Karma Police", "Unknown Album")
	if err != nil {
		t.Fatalf("SearchRecording: %v", err)
	}
	if strings.Contains(gotQuery, "Unknown") {
		t.Errorf("query %q should omit placeholder terms", gotQuery)
	}
	if !strings.Contains(gotQuery, `recording:"Karma Police"`) {
		t.Errorf("query %q should keep the real title", gotQuery)
	}
	if strings.Contains(gotQuery, " AND ") {
		t.Errorf("query %q should have a single term", gotQuery)
	}
}

func TestSearchRecordingAllPlaceholders(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte(cannedRecordingJSON))
	}))
	defer srv.Close()

	c, _ := newTestClient(0)
	mb := NewMusicBrainz(c)
	mb.baseURL = srv.URL

	// Title falls back to the filename placeholder => no usable query terms.
	_, ok, err := mb.SearchRecording(context.Background(), "Unknown Artist", "", "Unknown Album")
	if err != nil {
		t.Fatalf("SearchRecording: %v", err)
	}
	if ok {
		t.Error("expected ok=false when no query terms remain")
	}
	if called {
		t.Error("should not hit the network with an empty query")
	}
}

func TestSearchRecordingNoHits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"recordings": []}`))
	}))
	defer srv.Close()

	c, _ := newTestClient(0)
	mb := NewMusicBrainz(c)
	mb.baseURL = srv.URL

	_, ok, err := mb.SearchRecording(context.Background(), "Nobody", "Nothing", "Nowhere")
	if err != nil {
		t.Fatalf("SearchRecording: %v", err)
	}
	if ok {
		t.Error("expected ok=false with empty recordings")
	}
}
