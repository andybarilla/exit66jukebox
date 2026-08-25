package api

import (
	"net/http"
	"slices"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

func streamIDs(t *testing.T, srv *Server) []string {
	t.Helper()
	all, err := store.ListStreams(srv.db, "")
	if err != nil {
		t.Fatalf("list streams: %v", err)
	}
	ids := make([]string, 0, len(all))
	for _, st := range all {
		ids = append(ids, st.ID)
	}
	return ids
}

// Every request kind must refuse an id with no row, and the stream table must
// come out unchanged. TestInventedStreamIDNeverBecomesShared covers the other
// axis: that an invented id cannot acquire the shared kind.
func TestRequestToUnknownStreamCreatesNothing(t *testing.T) {
	srv, _ := newTestServer(t)
	user := userSession(t, srv, "bob@example.com")
	tid := insertTrack(t, srv, "Song")
	before := streamIDs(t, srv)

	for _, kind := range []string{"track", "album", "artist"} {
		rec := postForm(srv, "/api/streams/no-such-stream/requests",
			"kind="+kind+"&id="+itoa(tid), user)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("kind=%s: want 404, got %d body %s", kind, rec.Code, rec.Body.String())
		}
	}
	if after := streamIDs(t, srv); !slices.Equal(before, after) {
		t.Fatalf("stream table changed: before %v, after %v", before, after)
	}
}

func TestHouseStreamTakesRequests(t *testing.T) {
	srv, _ := newTestServer(t)
	admin := adminSession(t, srv.db)
	tid := insertTrack(t, srv, "Song")

	rec := postForm(srv, "/api/streams/house/requests", "kind=track&id="+itoa(tid), admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("house request: status %d body %s", rec.Code, rec.Body.String())
	}
	st, ok, err := store.GetStream(srv.db, "house")
	if err != nil || !ok || st.Kind != store.KindShared {
		t.Fatalf("house stream: ok=%v kind=%q err=%v", ok, st.Kind, err)
	}
}
