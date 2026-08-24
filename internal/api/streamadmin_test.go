package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/auth"
	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

// personalStreamID names the private stream these tests exercise. It is the one
// global row every listener currently shares (#128); production code has no
// need of the literal, so it lives here rather than adding another hardcoded
// site alongside main.go's boot-time ensure.
const personalStreamID = "me"

func userSession(t *testing.T, srv *Server, email string) *http.Cookie {
	t.Helper()
	uid, err := store.CreateUser(srv.db, email, "User", "h", false)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	raw, _ := auth.GenerateToken()
	if err := store.CreateSession(srv.db, auth.HashToken(raw), uid, 4_000_000_000); err != nil {
		t.Fatalf("session: %v", err)
	}
	return &http.Cookie{Name: sessionCookie, Value: raw}
}

func insertTrack(t *testing.T, srv *Server, title string) int64 {
	t.Helper()
	id, err := store.UpsertTrack(srv.db, model.Track{Path: "/m/" + title + ".mp3", Title: title}, "Band", "", "LP")
	if err != nil {
		t.Fatalf("upsert track: %v", err)
	}
	return id
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// postForm posts a urlencoded body, which is what the request surface takes.
func postForm(srv *Server, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func do(srv *Server, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

// createSharedVia posts a create and returns the new stream's id.
func createSharedVia(t *testing.T, srv *Server, name string, cookie *http.Cookie) string {
	t.Helper()
	rec := do(srv, http.MethodPost, "/api/streams", `{"name":"`+name+`"}`, cookie)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %q: status %d body %s", name, rec.Code, rec.Body.String())
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if out.ID == "" {
		t.Fatal("create returned an empty id")
	}
	return out.ID
}

// Criterion 1: an admin creates a named shared stream and it appears in the
// list alongside house.
func TestAdminCreatesStreamAndItIsListedWithHouse(t *testing.T) {
	srv, _ := newTestServer(t)
	admin := adminSession(t, srv.db)
	id := createSharedVia(t, srv, "Kitchen", admin)

	rec := do(srv, http.MethodGet, "/api/streams", "", admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status %d", rec.Code)
	}
	var list []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		House bool   `json:"house"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	var sawHouse, sawNew bool
	for _, st := range list {
		if st.ID == "house" && st.House {
			sawHouse = true
		}
		if st.ID == id && st.Name == "Kitchen" && !st.House {
			sawNew = true
		}
	}
	if !sawHouse || !sawNew {
		t.Fatalf("want house and %q in the list, got %+v", id, list)
	}
}

// Criterion 2: rename changes the label, not the id, and names may collide.
func TestRenameKeepsIDAndAllowsDuplicateNames(t *testing.T) {
	srv, _ := newTestServer(t)
	admin := adminSession(t, srv.db)
	a := createSharedVia(t, srv, "Kitchen", admin)
	b := createSharedVia(t, srv, "Patio", admin)

	rec := do(srv, http.MethodPatch, "/api/streams/"+b, `{"name":"Kitchen"}`, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("rename status %d body %s", rec.Code, rec.Body.String())
	}
	for _, id := range []string{a, b} {
		st, ok, err := store.GetStream(srv.db, id)
		if err != nil || !ok {
			t.Fatalf("get %s: ok=%v err=%v", id, ok, err)
		}
		if st.Name != "Kitchen" {
			t.Fatalf("%s name: want Kitchen, got %q", id, st.Name)
		}
	}
}

// Criterion 3: delete removes the stream's queue rows.
func TestDeleteStreamRemovesQueueRows(t *testing.T) {
	srv, _ := newTestServer(t)
	admin := adminSession(t, srv.db)
	id := createSharedVia(t, srv, "Party", admin)
	tid := insertTrack(t, srv, "Song")
	if err := store.Enqueue(srv.db, id, tid, "someone"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	if rec := do(srv, http.MethodDelete, "/api/streams/"+id, "", admin); rec.Code != http.StatusOK {
		t.Fatalf("delete status %d body %s", rec.Code, rec.Body.String())
	}
	if _, ok, _ := store.GetStream(srv.db, id); ok {
		t.Fatal("stream row survived delete")
	}
	ids, err := store.QueueTrackIDs(srv.db, id)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("queue rows survived delete: %v", ids)
	}
}

// house is always-on and must not be deletable.
func TestHouseStreamCannotBeDeleted(t *testing.T) {
	srv, _ := newTestServer(t)
	admin := adminSession(t, srv.db)
	rec := do(srv, http.MethodDelete, "/api/streams/house", "", admin)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 deleting house, got %d body %s", rec.Code, rec.Body.String())
	}
	if _, ok, _ := store.GetStream(srv.db, "house"); !ok {
		t.Fatal("house row was deleted")
	}
}

// Criterion 4: creating past the cap is a 409 naming the limit. The store-level
// half of this criterion lives in store/streams_test.go.
func TestCreateBeyondCapIs409NamingTheLimit(t *testing.T) {
	srv, _ := newTestServer(t)
	admin := adminSession(t, srv.db)
	// house already occupies one slot.
	for i := 0; i < store.MaxSharedStreams-1; i++ {
		createSharedVia(t, srv, "Room", admin)
	}
	rec := do(srv, http.MethodPost, "/api/streams", `{"name":"One Too Many"}`, admin)
	if rec.Code != http.StatusConflict {
		t.Fatalf("want 409 past the cap, got %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "4") {
		t.Fatalf("409 body should name the limit, got %s", rec.Body.String())
	}
}

// Criterion 5, and the hole Andy exploited on main: posting to an invented
// stream URL must not produce a shared stream, and the privileged routes must
// not create the row they were handed.
func TestInventedStreamIDNeverBecomesShared(t *testing.T) {
	srv, _ := newTestServer(t)
	user := userSession(t, srv, "bob@example.com")
	tid := insertTrack(t, srv, "Song")

	// The request surface still creates the personal stream on first touch, but
	// only ever as private.
	rec := postForm(srv, "/api/streams/bobs-invented-stream/requests",
		"kind=track&id="+itoa(tid), user)
	if rec.Code != http.StatusOK {
		t.Fatalf("request status %d body %s", rec.Code, rec.Body.String())
	}
	st, ok, err := store.GetStream(srv.db, "bobs-invented-stream")
	if err != nil || !ok {
		t.Fatalf("get invented stream: ok=%v err=%v", ok, err)
	}
	if st.Kind != store.KindPrivate {
		t.Fatalf("invented id kind: want private, got %q", st.Kind)
	}
	shared, err := store.ListStreams(srv.db, store.KindShared)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, s := range shared {
		if s.ID != "house" {
			t.Fatalf("an invented id produced a shared stream: %+v", s)
		}
	}
}

// The privileged routes must reject an id with no row rather than creating one:
// that is what stops the gate from deciding against a row that does not exist.
func TestPrivilegedRoutesDoNotCreateStreams(t *testing.T) {
	srv, _ := newTestServer(t)
	user := userSession(t, srv, "bob@example.com")
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/streams/ghost/next"},
		{http.MethodPost, "/api/streams/ghost/shuffle"},
		{http.MethodDelete, "/api/streams/ghost/requests"},
		{http.MethodDelete, "/api/streams/ghost/requests/1"},
		{http.MethodPost, "/api/streams/ghost/station"},
		{http.MethodDelete, "/api/streams/ghost/station"},
		{http.MethodPatch, "/api/streams/ghost"},
		{http.MethodDelete, "/api/streams/ghost"},
	} {
		rec := do(srv, tc.method, tc.path, "", user)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s %s: want 404, got %d body %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
		if _, ok, _ := store.GetStream(srv.db, "ghost"); ok {
			t.Fatalf("%s %s created the stream row", tc.method, tc.path)
		}
	}
}

// A GET must not mint a stream row either.
func TestGetStreamDoesNotCreateARow(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := do(srv, http.MethodGet, "/api/streams/never-seen", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if _, ok, _ := store.GetStream(srv.db, "never-seen"); ok {
		t.Fatal("GET /api/streams/{id} created a stream row")
	}
}

// Criterion 6, the security fix: a non-admin is rejected on EVERY shared
// stream, specifically including one created after boot — not only on house.
func TestNonAdminRejectedOnNewlyCreatedSharedStream(t *testing.T) {
	srv, _ := newTestServer(t)
	admin := adminSession(t, srv.db)
	id := createSharedVia(t, srv, "Kitchen", admin)
	user := userSession(t, srv, "bob@example.com")

	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/streams/" + id + "/next", ""},
		{http.MethodPost, "/api/streams/" + id + "/shuffle", ""},
		{http.MethodDelete, "/api/streams/" + id + "/requests", ""},
		{http.MethodDelete, "/api/streams/" + id + "/requests/1", ""},
		{http.MethodPost, "/api/streams/" + id + "/station", `{"genre":"rock"}`},
		{http.MethodDelete, "/api/streams/" + id + "/station", ""},
		{http.MethodPatch, "/api/streams/" + id, `{"name":"Mine"}`},
		{http.MethodDelete, "/api/streams/" + id, ""},
	} {
		rec := do(srv, tc.method, tc.path, tc.body, user)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("non-admin %s %s: want 403, got %d body %s",
				tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
	// And it is still there afterwards.
	if _, ok, _ := store.GetStream(srv.db, id); !ok {
		t.Fatal("a non-admin managed to delete the stream")
	}
}

// A non-admin must not be able to create a shared stream at all.
func TestNonAdminCannotCreateSharedStream(t *testing.T) {
	srv, _ := newTestServer(t)
	user := userSession(t, srv, "bob@example.com")
	rec := do(srv, http.MethodPost, "/api/streams", `{"name":"Mine"}`, user)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d body %s", rec.Code, rec.Body.String())
	}
	shared, _ := store.ListStreams(srv.db, store.KindShared)
	if len(shared) != 1 {
		t.Fatalf("want only house, got %+v", shared)
	}
}

// A guest's own private stream stays open — the gate keys on kind, so it must
// not start gating private queues.
func TestPrivateStreamControlsStayOpen(t *testing.T) {
	srv, _ := newTestServer(t)
	user := userSession(t, srv, "bob@example.com")
	tid := insertTrack(t, srv, "Song")
	if rec := postForm(srv, "/api/streams/me/requests", "kind=track&id="+itoa(tid), user); rec.Code != http.StatusOK {
		t.Fatalf("request: %d", rec.Code)
	}
	if rec := do(srv, http.MethodPost, "/api/streams/me/next", "", user); rec.Code != http.StatusOK {
		t.Fatalf("non-admin next on own stream: want 200, got %d", rec.Code)
	}
}

// Criterion 7: under SecurityModeOpen the same operations are permitted with no
// admin at all, matching the pre-existing house behavior.
func TestOpenModePermitsSharedStreamOpsWithoutAdmin(t *testing.T) {
	srv, _ := newTestServer(t)
	if err := store.SetSecurityMode(srv.db, store.SecurityModeOpen); err != nil {
		t.Fatalf("set mode: %v", err)
	}
	id := createSharedVia(t, srv, "Kitchen", nil)
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/streams/" + id + "/next", ""},
		{http.MethodPost, "/api/streams/" + id + "/shuffle", ""},
		{http.MethodPatch, "/api/streams/" + id, `{"name":"Patio"}`},
		{http.MethodDelete, "/api/streams/" + id, ""},
	} {
		rec := do(srv, tc.method, tc.path, tc.body, nil)
		if rec.Code == http.StatusForbidden || rec.Code == http.StatusUnauthorized {
			t.Fatalf("open mode %s %s: want permitted, got %d body %s",
				tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

func TestCreateStreamRejectsEmptyName(t *testing.T) {
	srv, _ := newTestServer(t)
	admin := adminSession(t, srv.db)
	if rec := do(srv, http.MethodPost, "/api/streams", `{"name":"   "}`, admin); rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for a blank name, got %d", rec.Code)
	}
}

// A private stream is nobody's to rename or destroy. The queue controls behind
// requireAdminShared deliberately fall through for private streams so a guest
// can drive their own queue, but rename and delete are not queue controls:
// falling through there let any authenticated non-admin wipe a private
// stream's queue rows, its station, and the row itself. The personal stream is
// still one global row shared by every listener (#128), so that is everyone's
// queue and station, destroyed by any logged-in user.
func TestNonAdminCannotRenameOrDeleteAPrivateStream(t *testing.T) {
	srv, _ := newTestServer(t)
	user := userSession(t, srv, "bob@example.com")
	tid := insertTrack(t, srv, "Song")
	if err := store.Enqueue(srv.db, personalStreamID, tid, "Bob"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := store.UpsertStation(srv.db, store.Station{
		StreamID: personalStreamID, Genre: "rock", Threshold: 3, Batch: 10,
	}); err != nil {
		t.Fatalf("station: %v", err)
	}

	for _, tc := range []struct{ method, body string }{
		{http.MethodDelete, ""},
		{http.MethodPatch, `{"name":"mine now"}`},
	} {
		rec := do(srv, tc.method, "/api/streams/"+personalStreamID, tc.body, user)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("non-admin %s on a private stream: want 404, got %d body %s",
				tc.method, rec.Code, rec.Body.String())
		}
	}

	if _, ok, _ := store.GetStream(srv.db, personalStreamID); !ok {
		t.Fatal("the private stream row was destroyed")
	}
	ids, err := store.QueueTrackIDs(srv.db, personalStreamID)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("private stream queue rows destroyed: %v", ids)
	}
	if _, ok := store.GetStation(srv.db, personalStreamID); !ok {
		t.Fatal("the private stream's station was destroyed")
	}
}

// An admin has no business there either: rename and delete are shared-stream
// operations, so a private stream is simply not a valid target.
func TestAdminCannotDeleteAPrivateStream(t *testing.T) {
	srv, _ := newTestServer(t)
	admin := adminSession(t, srv.db)
	rec := do(srv, http.MethodDelete, "/api/streams/"+personalStreamID, "", admin)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("admin DELETE on a private stream: want 404, got %d body %s", rec.Code, rec.Body.String())
	}
	if _, ok, _ := store.GetStream(srv.db, personalStreamID); !ok {
		t.Fatal("the private stream row was destroyed")
	}
}

// house is undeletable but not unrenameable: only its id is load-bearing, so
// the label is editable like any other shared stream's.
func TestHouseStreamCanBeRenamed(t *testing.T) {
	srv, _ := newTestServer(t)
	admin := adminSession(t, srv.db)
	if rec := do(srv, http.MethodPatch, "/api/streams/house", `{"name":"The Big Room"}`, admin); rec.Code != http.StatusOK {
		t.Fatalf("rename house: want 200, got %d body %s", rec.Code, rec.Body.String())
	}
	st, ok, err := store.GetStream(srv.db, "house")
	if err != nil || !ok {
		t.Fatalf("get house: ok=%v err=%v", ok, err)
	}
	if st.ID != "house" {
		t.Fatalf("house id changed to %q", st.ID)
	}
	if st.Name != "The Big Room" {
		t.Fatalf("house name: want The Big Room, got %q", st.Name)
	}
}

// The client hides its "new stream" affordance at the cap, so the limit has to
// reach it from the one place that enforces it. Serving it via /api/config
// means changing store.MaxSharedStreams cannot silently leave the UI offering a
// button whose only outcome is a 409.
func TestConfigCarriesTheSharedStreamCap(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := do(srv, http.MethodGet, "/api/config", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("config status %d", rec.Code)
	}
	var cfg struct {
		MaxSharedStreams int `json:"max_shared_streams"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if cfg.MaxSharedStreams != store.MaxSharedStreams {
		t.Fatalf("config max_shared_streams = %d, want store.MaxSharedStreams = %d",
			cfg.MaxSharedStreams, store.MaxSharedStreams)
	}
}
