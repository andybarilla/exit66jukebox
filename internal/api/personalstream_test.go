package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/auth"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

// profileSession creates a household profile (a user row carrying the
// passwordless flag) and returns its id and a session cookie for it.
func profileSession(t *testing.T, srv *Server, name string) (int64, *http.Cookie) {
	t.Helper()
	uid, err := store.CreatePasswordlessProfile(srv.db, name)
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	raw, _ := auth.GenerateToken()
	if err := store.CreateSession(srv.db, auth.HashToken(raw), uid, 4_000_000_000); err != nil {
		t.Fatalf("profile session: %v", err)
	}
	return uid, &http.Cookie{Name: sessionCookie, Value: raw}
}

// queueOf reads a stream's queued track ids straight from the store, so an
// assertion about whose queue holds what does not depend on the same handler
// the test is exercising.
func queueOf(t *testing.T, srv *Server, streamID string) []int64 {
	t.Helper()
	ids, err := store.QueueTrackIDs(srv.db, streamID)
	if err != nil {
		t.Fatalf("queue %s: %v", streamID, err)
	}
	return ids
}

// Criterion 1 and 2, and the reproduction on #128: Alice queues a track to her
// personal stream; Bob, a different non-admin, must neither see it nor be able
// to destroy it. Both send the same alias — the server decides which stream
// that names.
func TestTwoUsersHaveSeparatePersonalQueues(t *testing.T) {
	srv, _ := newTestServer(t)
	aliceID, alice := userSessionWithID(t, srv, "alice@example.com")
	bobID, bob := userSessionWithID(t, srv, "bob@example.com")
	tid := insertTrack(t, srv, "Song")

	if rec := postForm(srv, "/api/streams/me/requests", "kind=track&id="+itoa(tid), alice); rec.Code != http.StatusOK {
		t.Fatalf("alice request: %d %s", rec.Code, rec.Body)
	}

	// Bob reads the same alias and must see his own empty queue.
	rec := do(srv, http.MethodGet, "/api/streams/me", "", bob)
	if rec.Code != http.StatusOK {
		t.Fatalf("bob read: %d %s", rec.Code, rec.Body)
	}
	var got struct {
		ID    string `json:"id"`
		Queue []struct {
			ID int64 `json:"id"`
		} `json:"queue"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Queue) != 0 {
		t.Fatalf("bob sees alice's queue: %+v", got.Queue)
	}
	if got.ID != store.PersonalStreamID(bobID) {
		t.Fatalf("alias resolved to %q, want bob's own %q", got.ID, store.PersonalStreamID(bobID))
	}

	// The destructive half: Bob clearing his own personal stream must not
	// touch Alice's.
	if rec := do(srv, http.MethodDelete, "/api/streams/me/requests", "", bob); rec.Code != http.StatusOK {
		t.Fatalf("bob clear: %d %s", rec.Code, rec.Body)
	}
	if ids := queueOf(t, srv, store.PersonalStreamID(aliceID)); len(ids) != 1 {
		t.Fatalf("bob's clear wiped alice's queue: %v", ids)
	}
}

// Criterion 6: the id is derived server-side, so a client that works out
// another user's derived id and puts it in the path must get nowhere. Every
// operation on the personal surface is checked, not just the read.
func TestPersonalStreamNotAddressableByDerivedID(t *testing.T) {
	srv, _ := newTestServer(t)
	aliceID, alice := userSessionWithID(t, srv, "alice@example.com")
	_, bob := userSessionWithID(t, srv, "bob@example.com")
	tid := insertTrack(t, srv, "Song")
	if rec := postForm(srv, "/api/streams/me/requests", "kind=track&id="+itoa(tid), alice); rec.Code != http.StatusOK {
		t.Fatalf("alice request: %d %s", rec.Code, rec.Body)
	}
	hers := store.PersonalStreamID(aliceID)

	// Every route the seam wraps, so a route added without it shows up here.
	for _, tc := range []struct {
		name, method, path, body string
		form                     bool
	}{
		{name: "read", method: http.MethodGet, path: "/api/streams/" + hers},
		{name: "events", method: http.MethodGet, path: "/api/streams/" + hers + "/events"},
		{name: "request", method: http.MethodPost, path: "/api/streams/" + hers + "/requests",
			body: "kind=track&id=" + itoa(tid), form: true},
		{name: "next", method: http.MethodPost, path: "/api/streams/" + hers + "/next"},
		{name: "clear", method: http.MethodDelete, path: "/api/streams/" + hers + "/requests"},
		{name: "remove", method: http.MethodDelete, path: "/api/streams/" + hers + "/requests/" + itoa(tid)},
		{name: "shuffle", method: http.MethodPost, path: "/api/streams/" + hers + "/shuffle",
			body: "value=true", form: true},
		{name: "get station", method: http.MethodGet, path: "/api/streams/" + hers + "/station"},
		{name: "start station", method: http.MethodPost, path: "/api/streams/" + hers + "/station",
			body: `{"genre":"rock"}`},
		{name: "stop station", method: http.MethodDelete, path: "/api/streams/" + hers + "/station"},
		{name: "rename", method: http.MethodPatch, path: "/api/streams/" + hers, body: `{"name":"mine now"}`},
		{name: "delete", method: http.MethodDelete, path: "/api/streams/" + hers},
	} {
		var rec *httptest.ResponseRecorder
		if tc.form {
			rec = postForm(srv, tc.path, tc.body, bob)
		} else {
			rec = do(srv, tc.method, tc.path, tc.body, bob)
		}
		if rec.Code != http.StatusNotFound {
			t.Errorf("bob %s on alice's derived id: want 404, got %d %s", tc.name, rec.Code, rec.Body)
		}
	}

	// Nothing above may have reached her queue.
	if ids := queueOf(t, srv, hers); len(ids) != 1 || ids[0] != tid {
		t.Fatalf("alice's queue was modified: %v", ids)
	}
}

// The refusal must key on the id's shape, not on finding a private row behind
// it. A user who has never used their personal stream has no row at all, so a
// check that only looks at stored rows lets that id through — and since the
// ids are consecutive, guessing one is trivial. Separate from the test above,
// which necessarily exercises a provisioned stream.
func TestUnprovisionedPersonalStreamIDIsRefused(t *testing.T) {
	srv, _ := newTestServer(t)
	carolID, _ := userSessionWithID(t, srv, "carol@example.com")
	_, bob := userSessionWithID(t, srv, "bob@example.com")
	// Carol has never made a request, so there is no row to find.
	hers := store.PersonalStreamID(carolID)
	if _, ok, _ := store.GetStream(srv.db, hers); ok {
		t.Fatal("precondition: carol should have no personal stream row yet")
	}

	rec := do(srv, http.MethodGet, "/api/streams/"+hers, "", bob)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("read of an unprovisioned personal id: want 404, got %d %s", rec.Code, rec.Body)
	}
	// And it must not have been created as a side effect of being asked for.
	if _, ok, _ := store.GetStream(srv.db, hers); ok {
		t.Fatal("addressing another user's personal id provisioned it")
	}
}

// Criterion 5: boot no longer provisions a personal stream row, so the first
// request a user makes has to create theirs. This is the path boot was
// covering; without it every user's first request 404s.
func TestPersonalStreamProvisionedOnFirstUse(t *testing.T) {
	srv, _ := newTestServer(t)
	uid, user := userSessionWithID(t, srv, "new@example.com")
	if _, ok, _ := store.GetStream(srv.db, store.PersonalStreamID(uid)); ok {
		t.Fatal("a new user should not have a personal stream row before first use")
	}
	tid := insertTrack(t, srv, "Song")

	if rec := postForm(srv, "/api/streams/me/requests", "kind=track&id="+itoa(tid), user); rec.Code != http.StatusOK {
		t.Fatalf("first use: want 200, got %d %s", rec.Code, rec.Body)
	}
	st, ok, err := store.GetStream(srv.db, store.PersonalStreamID(uid))
	if err != nil || !ok {
		t.Fatalf("first use did not provision the row: ok=%v err=%v", ok, err)
	}
	if st.Kind != store.KindPrivate {
		t.Fatalf("provisioned kind = %q, want private", st.Kind)
	}
	if ids := queueOf(t, srv, st.ID); len(ids) != 1 || ids[0] != tid {
		t.Fatalf("track did not land in the new stream: %v", ids)
	}
}

// Criterion 4: open and open_admin_locked admit requests that carry no user at
// all, so there is no personal stream to address there — with or without a
// session, since those modes do not require one.
func TestNoPersonalStreamInOpenModes(t *testing.T) {
	for _, mode := range []store.SecurityMode{store.SecurityModeOpen, store.SecurityModeOpenAdminLocked} {
		t.Run(string(mode), func(t *testing.T) {
			srv, _ := newTestServer(t)
			uid, user := userSessionWithID(t, srv, "bob@example.com")
			if err := store.SetSecurityMode(srv.db, mode); err != nil {
				t.Fatalf("set mode: %v", err)
			}
			tid := insertTrack(t, srv, "Song")

			for _, cookie := range []*http.Cookie{nil, user} {
				if rec := do(srv, http.MethodGet, "/api/streams/me", "", cookie); rec.Code != http.StatusNotFound {
					t.Errorf("read alias in %s: want 404, got %d %s", mode, rec.Code, rec.Body)
				}
				if rec := postForm(srv, "/api/streams/me/requests", "kind=track&id="+itoa(tid), cookie); rec.Code != http.StatusNotFound {
					t.Errorf("request alias in %s: want 404, got %d %s", mode, rec.Code, rec.Body)
				}
			}
			if _, ok, _ := store.GetStream(srv.db, store.PersonalStreamID(uid)); ok {
				t.Errorf("%s provisioned a personal stream row", mode)
			}
		})
	}
}

// Criterion 3: a household profile is a user row, so it gets its own personal
// stream and cannot reach another profile's.
func TestHouseholdProfilesGetSeparatePersonalStreams(t *testing.T) {
	srv, _ := newTestServer(t)
	if err := store.SetSecurityMode(srv.db, store.SecurityModeHouseholdProfiles); err != nil {
		t.Fatalf("set mode: %v", err)
	}
	kidID, kid := profileSession(t, srv, "Kid")
	adultID, adult := profileSession(t, srv, "Adult")
	tid := insertTrack(t, srv, "Song")

	if rec := postForm(srv, "/api/streams/me/requests", "kind=track&id="+itoa(tid), kid); rec.Code != http.StatusOK {
		t.Fatalf("kid request: %d %s", rec.Code, rec.Body)
	}
	if rec := do(srv, http.MethodDelete, "/api/streams/me/requests", "", adult); rec.Code != http.StatusOK {
		t.Fatalf("adult clear: %d %s", rec.Code, rec.Body)
	}
	if ids := queueOf(t, srv, store.PersonalStreamID(kidID)); len(ids) != 1 {
		t.Fatalf("one profile's clear emptied another's queue: %v", ids)
	}
	if store.PersonalStreamID(kidID) == store.PersonalStreamID(adultID) {
		t.Fatal("two profiles derived the same personal stream id")
	}
}

// The config payload tells the client whether to offer the Personal control at
// all, the same class of fact as the security mode it already carries.
func TestConfigReportsPersonalStreamAvailability(t *testing.T) {
	srv, _ := newTestServer(t)
	_, user := userSessionWithID(t, srv, "bob@example.com")

	for _, tc := range []struct {
		mode   store.SecurityMode
		cookie *http.Cookie
		want   bool
	}{
		{store.SecurityModeFullLogin, user, true},
		{store.SecurityModeFullLogin, nil, false},
		{store.SecurityModeHouseholdProfiles, user, true},
		{store.SecurityModeOpen, user, false},
		{store.SecurityModeOpenAdminLocked, user, false},
	} {
		if err := store.SetSecurityMode(srv.db, tc.mode); err != nil {
			t.Fatalf("set mode: %v", err)
		}
		rec := do(srv, http.MethodGet, "/api/config", "", tc.cookie)
		var cfg struct {
			PersonalStream bool `json:"personal_stream"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &cfg); err != nil {
			t.Fatalf("decode config: %v", err)
		}
		if cfg.PersonalStream != tc.want {
			t.Errorf("mode=%s authed=%v personal_stream=%v, want %v",
				tc.mode, tc.cookie != nil, cfg.PersonalStream, tc.want)
		}
	}
}

// The continuous-MP3 route parses the id out of the path itself rather than
// through a {id} wildcard, so it is the one stream-addressing surface not
// behind resolvePersonalStream. It must still not serve a personal stream:
// only a shared stream gets a pipeline, and without one there is nothing to
// attach to. Asserted rather than assumed, because the resolver does not cover
// this route and a future lazy-start change could quietly make it do so.
func TestPersonalStreamIsNotServedAsContinuousAudio(t *testing.T) {
	srv, _ := newTestServer(t)
	uid, user := userSessionWithID(t, srv, "bob@example.com")
	tid := insertTrack(t, srv, "Song")
	// Provision it and give it a queued track, so a pipeline would have
	// something to play if one were ever started.
	if rec := postForm(srv, "/api/streams/me/requests", "kind=track&id="+itoa(tid), user); rec.Code != http.StatusOK {
		t.Fatalf("provision: %d %s", rec.Code, rec.Body)
	}

	rec := do(srv, http.MethodGet, "/stream/"+store.PersonalStreamID(uid)+".mp3", "", user)
	if rec.Code == http.StatusOK {
		t.Fatalf("a personal stream was served as continuous audio: %d", rec.Code)
	}
}

// R5: rename and delete refuse a private stream, so resolving the alias for
// them must not create the row on the way to the 404. Otherwise a request that
// is refused still writes, and a user who only ever tried to delete their
// personal stream ends up with one.
func TestRefusedAliasOperationsDoNotProvision(t *testing.T) {
	for _, tc := range []struct{ name, method, body string }{
		{"delete", http.MethodDelete, ""},
		{"rename", http.MethodPatch, `{"name":"mine now"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newTestServer(t)
			uid, user := userSessionWithID(t, srv, "bob@example.com")
			mine := store.PersonalStreamID(uid)

			rec := do(srv, tc.method, "/api/streams/me", tc.body, user)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("want 404, got %d %s", rec.Code, rec.Body)
			}
			if _, ok, _ := store.GetStream(srv.db, mine); ok {
				t.Fatal("a refused request provisioned a personal stream row")
			}
		})
	}
}

// The client decides at mount whether to run its heavy loads, using
// guest_access — and only re-reads config once it does. That is sound only
// while the modes allowing anonymous access are exactly the modes with no
// personal stream: a mode that allowed both would let a logged-in user's
// personal_stream go stale behind a mount-time fetch. Nothing enforces that
// pairing, so assert it.
func TestAnonymousModesAreExactlyTheModesWithoutAPersonalStream(t *testing.T) {
	for _, mode := range []store.SecurityMode{
		store.SecurityModeOpen, store.SecurityModeOpenAdminLocked,
		store.SecurityModeHouseholdProfiles, store.SecurityModeFullLogin,
	} {
		anonymous := store.SecurityModeAllowsAnonymous(mode)
		// Ask with a user present, so only the mode decides the answer.
		_, hasPersonal := personalStreamFor(mode, store.User{ID: 1}, true)
		if anonymous == hasPersonal {
			t.Errorf("mode %s: allows anonymous = %v and has a personal stream = %v; "+
				"these must stay mutually exclusive or the client's mount-time config fetch goes stale",
				mode, anonymous, hasPersonal)
		}
	}
}

// R7: two middlewares classify a stream's kind with opposite consequences —
// resolvePersonalStream 404s a private row, streamGate lets one through
// ungated. That is layered rather than duplicated (reachability, then
// authorization), and it holds only while the sole private stream able to
// reach streamGate is the caller's own. If the resolver were ever loosened,
// streamGate's ungated fall-through would silently become another user's
// queue, so assert the invariant directly rather than trusting the layering.
func TestOnlyTheCallersOwnPrivateStreamReachesTheQueueControls(t *testing.T) {
	srv, _ := newTestServer(t)
	aliceID, alice := userSessionWithID(t, srv, "alice@example.com")
	_, bob := userSessionWithID(t, srv, "bob@example.com")
	tid := insertTrack(t, srv, "Song")
	if rec := postForm(srv, "/api/streams/me/requests", "kind=track&id="+itoa(tid), alice); rec.Code != http.StatusOK {
		t.Fatalf("alice request: %d %s", rec.Code, rec.Body)
	}
	// A private stream outside the per-user namespace, as an older build could
	// have left behind: it belongs to nobody the gate can check.
	if err := store.EnsurePrivateStream(srv.db, "legacy-private"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := store.Enqueue(srv.db, "legacy-private", tid, "someone"); err != nil {
		t.Fatalf("seed queue: %v", err)
	}

	for _, id := range []string{store.PersonalStreamID(aliceID), "legacy-private"} {
		if rec := do(srv, http.MethodDelete, "/api/streams/"+id+"/requests", "", bob); rec.Code != http.StatusNotFound {
			t.Errorf("clear on private stream %q: want 404, got %d %s", id, rec.Code, rec.Body)
		}
		if ids, _ := store.QueueTrackIDs(srv.db, id); len(ids) != 1 {
			t.Errorf("private stream %q was cleared through the ungated fall-through: %v", id, ids)
		}
	}
}
