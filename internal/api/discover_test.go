package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/recommend"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

func TestDiscoverRediscoverEndpoint(t *testing.T) {
	srv, _ := newTestServer(t)
	store.UpsertTrack(srv.db, model.Track{Path: "/m/a.mp3", Title: "A", Genre: "Rock"}, "B", "", "X")

	req := httptest.NewRequest(http.MethodGet, "/api/discover/rediscover?genre=Rock", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d", rec.Code)
	}
	var got []model.Track
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	if len(got) != 1 || got[0].Title != "A" {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestDiscoverGenresEndpoint(t *testing.T) {
	srv, _ := newTestServer(t)
	store.UpsertTrack(srv.db, model.Track{Path: "/m/a.mp3", Title: "A", Genre: "Rock"}, "B", "", "X")

	req := httptest.NewRequest(http.MethodGet, "/api/discover/genres", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Rock") {
		t.Fatalf("unexpected genres response: %d %s", rec.Code, rec.Body.String())
	}
}

func TestStationStartGetStopEndpoints(t *testing.T) {
	srv, _ := newTestServer(t)
	for _, p := range []string{"/m/1.mp3", "/m/2.mp3", "/m/3.mp3"} {
		store.UpsertTrack(srv.db, model.Track{Path: p, Title: p, Genre: "Rock"}, "B", "", "X")
	}
	// A station on the caller's own personal stream: the alias resolves to it
	// and provisions it, so no test-only stream row is needed. A private
	// stream belonging to anyone else is not addressable at all (#128).
	uid, user := userSessionWithID(t, srv, "bob@example.com")
	mine := store.PersonalStreamID(uid)

	// Start
	body := strings.NewReader(`{"genre":"Rock"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/streams/me/station", body)
	req.AddCookie(user)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("start status: %d body=%s", rec.Code, rec.Body.String())
	}

	// Get
	req2 := httptest.NewRequest(http.MethodGet, "/api/streams/me/station", nil)
	req2.AddCookie(user)
	rec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK || !strings.Contains(rec2.Body.String(), "Rock") {
		t.Fatalf("get station: %d %s", rec2.Code, rec2.Body.String())
	}

	// Queue should have been filled immediately.
	n, _ := store.QueueLen(srv.db, mine)
	if n == 0 {
		t.Fatalf("expected immediate fill, queue empty")
	}

	// Stop
	req3 := httptest.NewRequest(http.MethodDelete, "/api/streams/me/station", nil)
	req3.AddCookie(user)
	rec3 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("stop status: %d", rec3.Code)
	}
	if _, ok := store.GetStation(srv.db, mine); ok {
		t.Fatalf("expected station removed after stop")
	}
}

func TestDiscoverRecommendedNoRunnerReturnsEmptyArray(t *testing.T) {
	srv, _ := newTestServer(t) // no recommend runner wired
	req := httptest.NewRequest(http.MethodGet, "/api/discover/recommended", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (not 503)", rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("body = %q, want []", rec.Body.String())
	}
}

func TestDiscoverRecommendedServesRunnerCache(t *testing.T) {
	srv, _ := newTestServer(t)
	// A runner with no configured sources serves an empty (non-null) array.
	srv.SetRecommendRunner(recommend.NewRunner(srv.db, nil, nil))
	req := httptest.NewRequest(http.MethodGet, "/api/discover/recommended", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []model.EnrichedTrack
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	if len(got) != 0 {
		t.Fatalf("got %d tracks, want 0", len(got))
	}
}

// rediscoverTitlesFor calls the rediscover endpoint as the given caller (nil
// cookie = unauthenticated) and returns the order it served.
func rediscoverTitlesFor(t *testing.T, srv *Server, cookie *http.Cookie) []string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/discover/rediscover?genre=Rock", nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var got []model.EnrichedTrack
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	out := make([]string, len(got))
	for i, tr := range got {
		out[i] = tr.Title
	}
	return out
}

func TestRediscoverEndpointIsScopedToTheCaller(t *testing.T) {
	srv, db := newTestServer(t)
	ids := map[string]int64{}
	for _, title := range []string{"Alpha", "Bravo", "Charlie"} {
		id, err := store.UpsertTrack(srv.db,
			model.Track{Path: "/m/" + title + ".mp3", Title: title, Genre: "Rock"}, "B", "", "X")
		if err != nil {
			t.Fatalf("upsert %s: %v", title, err)
		}
		ids[title] = id
	}
	aliceID, alice := userSessionWithID(t, srv, "alice@example.com")
	_, bob := userSessionWithID(t, srv, "bob@example.com")
	if err := store.EnsurePrivateStream(db, store.PersonalStreamID(aliceID)); err != nil {
		t.Fatalf("alice stream: %v", err)
	}
	// Alice plays Bravo on her own personal stream, and nowhere else.
	if _, err := db.Exec(`INSERT INTO history(stream_id, track_id, played_at) VALUES(?,?,9999)`,
		store.PersonalStreamID(aliceID), ids["Bravo"]); err != nil {
		t.Fatalf("history: %v", err)
	}

	natural := []string{"Alpha", "Bravo", "Charlie"}
	demoted := []string{"Alpha", "Charlie", "Bravo"}
	if got := rediscoverTitlesFor(t, srv, alice); !slices.Equal(got, demoted) {
		t.Errorf("alice = %v, want %v (her own play demotes Bravo)", got, demoted)
	}
	if got := rediscoverTitlesFor(t, srv, bob); !slices.Equal(got, natural) {
		t.Errorf("bob = %v, want %v (alice's private play must not shape his ranking)", got, natural)
	}
	// An unauthenticated caller has no personal stream: shared streams only,
	// served without error.
	if got := rediscoverTitlesFor(t, srv, nil); !slices.Equal(got, natural) {
		t.Errorf("anonymous = %v, want %v", got, natural)
	}
	// Neither does a caller in either open mode, even a signed-in one: those
	// modes have no personal streams at all.
	for _, mode := range []store.SecurityMode{store.SecurityModeOpen, store.SecurityModeOpenAdminLocked} {
		if err := store.SetSecurityMode(db, mode); err != nil {
			t.Fatalf("SetSecurityMode(%s): %v", mode, err)
		}
		if got := rediscoverTitlesFor(t, srv, alice); !slices.Equal(got, natural) {
			t.Errorf("%s = %v, want %v", mode, got, natural)
		}
	}
}

func TestRediscoverEndpointRanksOnTheCallersOwnPlayCount(t *testing.T) {
	srv, db := newTestServer(t)
	ids := map[string]int64{}
	for _, title := range []string{"Alpha", "Bravo", "Charlie"} {
		id, err := store.UpsertTrack(srv.db,
			model.Track{Path: "/m/" + title + ".mp3", Title: title, Genre: "Rock"}, "B", "", "X")
		if err != nil {
			t.Fatalf("upsert %s: %v", title, err)
		}
		ids[title] = id
	}
	if err := store.EnsureSharedStream(db, "house", "House"); err != nil {
		t.Fatalf("house: %v", err)
	}
	aliceID, alice := userSessionWithID(t, srv, "alice@example.com")
	_, bob := userSessionWithID(t, srv, "bob@example.com")
	if err := store.EnsurePrivateStream(db, store.PersonalStreamID(aliceID)); err != nil {
		t.Fatalf("alice stream: %v", err)
	}
	// Alice hears Bravo twice, privately. The house heard Alpha once.
	for _, at := range []int64{9000, 9999} {
		if _, err := db.Exec(`INSERT INTO history(stream_id, track_id, played_at) VALUES(?,?,?)`,
			store.PersonalStreamID(aliceID), ids["Bravo"], at); err != nil {
			t.Fatalf("history: %v", err)
		}
	}
	if _, err := db.Exec(`INSERT INTO history(stream_id, track_id, played_at) VALUES('house',?,100)`,
		ids["Alpha"]); err != nil {
		t.Fatalf("history: %v", err)
	}

	// Alice: 0, 1, 2 plays. Bob never heard Bravo, so for him it leads.
	if got := rediscoverTitlesFor(t, srv, alice); !slices.Equal(got, []string{"Charlie", "Alpha", "Bravo"}) {
		t.Errorf("alice = %v, want her two private plays to sink Bravo", got)
	}
	if got := rediscoverTitlesFor(t, srv, bob); !slices.Equal(got, []string{"Bravo", "Charlie", "Alpha"}) {
		t.Errorf("bob = %v, want alice's private plays not to count against him", got)
	}
}
