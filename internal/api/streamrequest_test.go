package api

import (
	"net/http"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

// streamIDs returns every stream id in the table, so a test can assert the set
// is unchanged rather than only that one particular row is absent.
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

func sameIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The hole this issue closes: the request surface was the last route that
// created the stream row it was handed, so any authenticated listener could
// mint unbounded private streams by choosing URLs.
func TestRequestToUnknownStreamCreatesNothing(t *testing.T) {
	srv, _ := newTestServer(t)
	user := userSession(t, srv, "bob@example.com")
	tid := insertTrack(t, srv, "Song")
	before := streamIDs(t, srv)

	for _, kind := range []string{"track", "album", "artist"} {
		rec := postForm(srv, "/api/streams/bobs-invented-stream/requests",
			"kind="+kind+"&id="+itoa(tid), user)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("kind=%s: want 404, got %d body %s", kind, rec.Code, rec.Body.String())
		}
	}
	if _, ok, _ := store.GetStream(srv.db, "bobs-invented-stream"); ok {
		t.Fatal("a request to an invented id created the stream row")
	}
	if after := streamIDs(t, srv); !sameIDs(before, after) {
		t.Fatalf("stream table changed: before %v, after %v", before, after)
	}
}

// The personal stream is what the implicit create was covering, so its
// first-use path is the thing most likely to break silently. Provisioning it
// is what main.go does at boot; with no row, the request is refused like any
// other unknown id, and once provisioned it takes requests.
func TestPersonalStreamWorksFromFirstUse(t *testing.T) {
	srv, db := newTestServer(t)
	user := userSession(t, srv, "bob@example.com")
	tid := insertTrack(t, srv, "Song")

	if err := store.DeleteStream(db, store.PersonalStreamID); err != nil {
		t.Fatalf("delete personal stream: %v", err)
	}
	rec := postForm(srv, "/api/streams/"+store.PersonalStreamID+"/requests",
		"kind=track&id="+itoa(tid), user)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unprovisioned personal stream: want 404, got %d", rec.Code)
	}

	// Exactly what boot does.
	if err := store.EnsurePrivateStream(db, store.PersonalStreamID); err != nil {
		t.Fatalf("provision personal stream: %v", err)
	}
	rec = postForm(srv, "/api/streams/"+store.PersonalStreamID+"/requests",
		"kind=track&id="+itoa(tid), user)
	if rec.Code != http.StatusOK {
		t.Fatalf("first use after provisioning: status %d body %s", rec.Code, rec.Body.String())
	}
	q, err := srv.jb.Queue(store.PersonalStreamID)
	if err != nil || len(q) != 1 {
		t.Fatalf("personal queue: len=%d err=%v", len(q), err)
	}
}

// The house stream is provisioned at boot and is unaffected by the request
// surface no longer creating rows.
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
