package fed

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/store"
	"github.com/hashicorp/yamux"
)

// TestPeerVisibleAppRoutesIsUnchanged pins the allowlist to exactly one route.
// Listening groups scope discovery only, so nothing in #88 may widen what a peer
// reaches in the application — growing this list is the audio-scoping decision
// being made by accident (#136, and the decision recorded on #88).
func TestPeerVisibleAppRoutesIsUnchanged(t *testing.T) {
	want := []string{"GET /api/tracks/{id}/audio"}
	if len(peerVisibleAppRoutes) != len(want) {
		t.Fatalf("peerVisibleAppRoutes = %q, want exactly %q", peerVisibleAppRoutes, want)
	}
	for i, route := range want {
		if peerVisibleAppRoutes[i] != route {
			t.Fatalf("peerVisibleAppRoutes[%d] = %q, want %q", i, peerVisibleAppRoutes[i], route)
		}
	}
}

func groupDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func addTrack(t *testing.T, db *sql.DB, title string) {
	t.Helper()
	if _, err := store.UpsertTrack(db, model.Track{Path: "/music/" + title + ".mp3", Title: title}, "Artist", "Artist", "Album"); err != nil {
		t.Fatalf("upsert %s: %v", title, err)
	}
}

func joinGroup(t *testing.T, db *sql.DB, name string, peers ...string) {
	t.Helper()
	g, err := store.CreateFederationGroup(db, name)
	if err != nil {
		t.Fatalf("create group %s: %v", name, err)
	}
	for _, p := range peers {
		if err := store.AddFederationGroupMember(db, g.ID, p); err != nil {
			t.Fatalf("add %s: %v", p, err)
		}
	}
}

// fetchPeerCatalog asks handler for /fed/catalog as the peer named by viewer.
func fetchPeerCatalog(t *testing.T, handler http.Handler, viewer string) []store.CatalogRow {
	t.Helper()
	rec := httptest.NewRecorder()
	WithSessionPeer(viewer, handler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/fed/catalog", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("catalog status for %s = %d (%s)", viewer, rec.Code, rec.Body)
	}
	var rows []store.CatalogRow
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode catalog for %s: %v", viewer, err)
	}
	return rows
}

// TestPeerCatalogScopedToGroupButAudioIsNot drives both halves of the #88
// decision in one fixture, deliberately: the same peer that is denied the
// catalog must still fetch audio. Asserting audio in a fixture where nothing is
// ever denied would pass whether or not any scoping code ran.
func TestPeerCatalogScopedToGroupButAudioIsNot(t *testing.T) {
	db := groupDB(t)
	addTrack(t, db, "Song")
	joinGroup(t, db, "family", "home", "office")
	spy := &spyHandler{}
	handler := PeerRoutes(db, "home", spy)

	if rows := fetchPeerCatalog(t, handler, "office"); len(rows) != 1 || rows[0].Title != "Song" {
		t.Fatalf("office shares a group with home, catalog = %#v, want one Song", rows)
	}
	if rows := fetchPeerCatalog(t, handler, "stranger"); len(rows) != 0 {
		t.Fatalf("stranger shares no group with home, catalog = %#v, want empty", rows)
	}

	// The peer just denied the catalog fetches audio anyway. Groups organise
	// what peers see; they are not a playback boundary.
	rec := httptest.NewRecorder()
	WithSessionPeer("stranger", handler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tracks/5/audio", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("audio status for a peer outside every group = %d, want 200", rec.Code)
	}
	if len(spy.hits) != 1 || spy.hits[0] != "GET /api/tracks/5/audio" {
		t.Fatalf("app hits = %v, want the audio request to reach the application", spy.hits)
	}
}

// An install that never creates a group keeps serving every peer, which is how
// federation behaved before groups existed.
func TestPeerCatalogUnscopedWhenNoGroupsExist(t *testing.T) {
	db := groupDB(t)
	addTrack(t, db, "Song")

	rows := fetchPeerCatalog(t, PeerRoutes(db, "home", nil), "stranger")

	if len(rows) != 1 {
		t.Fatalf("no groups exist, catalog = %#v, want one row", rows)
	}
}

// A peer with no identified session is denied once groups are active: the only
// production path into /fed/catalog tags the request at the handshake.
func TestPeerCatalogDeniedForUnidentifiedSession(t *testing.T) {
	db := groupDB(t)
	addTrack(t, db, "Song")
	joinGroup(t, db, "family", "home", "office")
	rec := httptest.NewRecorder()

	PeerRoutes(db, "home", nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/fed/catalog", nil))

	var rows []store.CatalogRow
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("catalog = %#v, want empty for an unidentified session", rows)
	}
}

// pushCatalog posts a peer's rows to the hub the way a member's sync loop does.
func pushCatalog(t *testing.T, relay *Relay, peer string, rows []store.CatalogRow) {
	t.Helper()
	body, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/fed/catalog/"+peer, bytes.NewReader(body))
	relay.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("push for %s = %d (%s)", peer, rec.Code, rec.Body)
	}
}

// fetchMerged asks the hub for the merged catalog as viewer, over a session the
// hub tagged with viewer's handshake id.
func fetchMerged(t *testing.T, relay *Relay, viewer string) MergedCatalog {
	t.Helper()
	rec := httptest.NewRecorder()
	WithSessionPeer(viewer, relay.Routes()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/fed/catalog/"+viewer+"/merged", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("merged status for %s = %d (%s)", viewer, rec.Code, rec.Body)
	}
	var merged MergedCatalog
	if err := json.Unmarshal(rec.Body.Bytes(), &merged); err != nil {
		t.Fatalf("decode merged: %v", err)
	}
	return merged
}

func rowsFor(title string) []store.CatalogRow {
	return []store.CatalogRow{{RemoteID: 1, Title: title, ArtistName: "Artist", AlbumName: "Album"}}
}

// In a hub topology the hub's group table is the authority: members never talk
// to each other, so what one discovers of another is decided here.
func TestHubMergedCatalogScopedToGroups(t *testing.T) {
	db := groupDB(t)
	joinGroup(t, db, "family", "home", "office")
	joinGroup(t, db, "friends", "home", "dave")
	relay := NewRelay(NewRegistry(), db)
	pushCatalog(t, relay, "office", rowsFor("OfficeSong"))
	pushCatalog(t, relay, "dave", rowsFor("DaveSong"))

	// home is in both groups, so it sees both catalogs.
	merged := fetchMerged(t, relay, "home")
	for _, peer := range []string{"office", "dave"} {
		if len(merged.Catalogs[peer]) != 1 {
			t.Fatalf("home should see %s: catalogs = %#v", peer, merged.Catalogs)
		}
	}

	// office and dave share no group, so neither sees the other.
	merged = fetchMerged(t, relay, "office")
	if len(merged.Catalogs["dave"]) != 0 {
		t.Fatalf("office must not see dave: %#v", merged.Catalogs["dave"])
	}
}

// Revoking membership must delete rows the peer already cached, so a denied peer
// is named with an empty catalog rather than dropped from the payload —
// ApplyCatalog with no rows deletes, absence does nothing.
func TestHubMergedCatalogEmptiesRevokedPeer(t *testing.T) {
	db := groupDB(t)
	joinGroup(t, db, "family", "home", "office")
	relay := NewRelay(NewRegistry(), db)
	pushCatalog(t, relay, "office", rowsFor("OfficeSong"))
	if len(fetchMerged(t, relay, "home").Catalogs["office"]) != 1 {
		t.Fatal("precondition: home should see office while they share a group")
	}

	groups, err := store.ListFederationGroups(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RemoveFederationGroupMember(db, groups[0].ID, "office"); err != nil {
		t.Fatal(err)
	}

	merged := fetchMerged(t, relay, "home")
	rows, named := merged.Catalogs["office"]
	if !named {
		t.Fatal("a revoked peer must still be named, with an empty catalog, or its rows are never deleted")
	}
	if len(rows) != 0 {
		t.Fatalf("revoked peer catalog = %#v, want empty", rows)
	}
}

// A peer the hub holds no rows for is omitted rather than emptied: the cache is
// in-memory, so treating absence as revocation would wipe a member's whole
// remote library every time the hub restarted.
func TestHubMergedCatalogOmitsPermittedPeerWithNoCachedRows(t *testing.T) {
	db := groupDB(t)
	joinGroup(t, db, "family", "home", "office")
	relay := NewRelay(NewRegistry(), db)

	merged := fetchMerged(t, relay, "home")

	if _, named := merged.Catalogs["office"]; named {
		t.Fatalf("office has pushed nothing since the hub started; naming it deletes the member's cache: %#v", merged.Catalogs)
	}
}

// The hub is a peer too, so a catalog it receives lands in its own library only
// when the sending peer shares one of its groups.
func TestRelayAppliesReceivedCatalogOnlyWithinAGroup(t *testing.T) {
	db := groupDB(t)
	joinGroup(t, db, "family", "hub", "office")
	relay := NewRelay(NewRegistry(), db)
	relay.SetSelf("hub")

	pushCatalog(t, relay, "office", rowsFor("OfficeSong"))
	pushCatalog(t, relay, "stranger", rowsFor("StrangerSong"))

	for peer, want := range map[string]int{"office": 1, "stranger": 0} {
		ids, err := store.RemoteSourceLibraryIDs(db, peer)
		if err != nil {
			t.Fatal(err)
		}
		if len(ids) != want {
			t.Fatalf("%s remote libraries = %d, want %d", peer, len(ids), want)
		}
	}
	// The rows are still cached for fan-out: whether another member may see
	// stranger is that member's own membership question, decided in serveMerged.
	relay.mu.Lock()
	cached := len(relay.catalogs["stranger"])
	relay.mu.Unlock()
	if cached != 1 {
		t.Fatalf("stranger rows cached for fan-out = %d, want 1", cached)
	}
}

// The merged fan-out scopes on the session's handshake id, not the one in the
// path, so a member cannot read another group's catalogs by asking under a
// different name. Both ids are only claimed (#167); the path one is chosen per
// request.
func TestHubMergedCatalogPrefersTheSessionPeerID(t *testing.T) {
	db := groupDB(t)
	joinGroup(t, db, "family", "home", "office")
	relay := NewRelay(NewRegistry(), db)
	pushCatalog(t, relay, "office", rowsFor("OfficeSong"))

	rec := httptest.NewRecorder()
	WithSessionPeer("stranger", relay.Routes()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/fed/catalog/home/merged", nil))

	var merged MergedCatalog
	if err := json.Unmarshal(rec.Body.Bytes(), &merged); err != nil {
		t.Fatal(err)
	}
	if len(merged.Catalogs["office"]) != 0 {
		t.Fatalf("stranger asked as home and got office's catalog: %#v", merged.Catalogs["office"])
	}
}

// TestBothPeerSessionDirectionsScopeOnTheRemotePeer runs two real Managers over
// TCP and drives a catalog pull down each direction of the peer link.
//
// It exists because only one direction was wrong: the outbound-dial path tagged
// its session with THIS instance's peer id, so a peer in no group of ours was
// answered as though it were us — and it kept receiving the full catalog. The
// inbound path was correct, so every single-direction test passed.
func TestBothPeerSessionDirectionsScopeOnTheRemotePeer(t *testing.T) {
	homeDB := groupDB(t)
	addTrack(t, homeDB, "HomeSong")
	// home shares a group with itself only, so "stranger" is in none of its groups.
	joinGroup(t, homeDB, "family", "home")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	home := &Manager{
		Role: "peer", Token: "tok", PeerID: "home", Registry: NewRegistry(),
		PeerHandler: PeerSessionHandler(Capabilities{}, NewSignaler(), homeDB, "home", nil),
	}

	// Direction 1: stranger dials home, home accepts and serves (servePeerConn).
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		p, err := acceptAndRegister(conn, "tok", home.Registry)
		if err != nil {
			return
		}
		_ = http.Serve(p.Session, WithSessionPeer(p.ID, home.PeerHandler))
	}()
	inbound := dialAsPeer(t, ln.Addr().String(), "stranger")
	if rows := catalogOverEventually(t, inbound); len(rows) != 0 {
		t.Fatalf("inbound session: stranger is in no group of home's, catalog = %#v", rows)
	}

	// Direction 2: home dials stranger (the real Manager.dialPeer), and serves
	// over the session IT opened. The requests arriving on it are still
	// stranger's, so its membership is what must be answered against.
	strangerLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer strangerLn.Close()
	sessions := make(chan *yamux.Session, 1)
	go func() {
		conn, err := strangerLn.Accept()
		if err != nil {
			return
		}
		sess, err := acceptAsPeer(conn, "tok")
		if err != nil {
			return
		}
		// Answer home's post-handshake callbacks (caps, and its own sync pull)
		// so dialPeer reaches its http.Serve instead of blocking in learnCaps.
		mux := http.NewServeMux()
		mux.HandleFunc("GET /fed/caps", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"direct_webrtc":false}`))
		})
		mux.HandleFunc("GET /fed/catalog", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("[]"))
		})
		go http.Serve(sess, mux)
		sessions <- sess
	}()
	go home.dialPeer("stranger", strangerLn.Addr().String())

	outSess := <-sessions
	defer outSess.Close()
	if rows := catalogOverEventually(t, outSess); len(rows) != 0 {
		t.Fatalf("outbound session: stranger is in no group of home's, catalog = %#v", rows)
	}
}

// acceptAsPeer is the server half of the peer handshake: read the token line,
// ack it, and hand back the multiplexed session.
func acceptAsPeer(conn net.Conn, token string) (*yamux.Session, error) {
	br := bufio.NewReader(conn)
	line, err := br.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if fields := strings.Fields(strings.TrimSpace(line)); len(fields) != 2 || fields[0] != token {
		return nil, fmt.Errorf("bad handshake %q", line)
	}
	if _, err := conn.Write([]byte{1}); err != nil {
		return nil, err
	}
	return yamux.Server(&bufferedConn{Reader: br, Conn: conn}, nil)
}

// catalogOverEventually retries while dialPeer is still in learnCaps and has not
// yet started serving, so the assertion is about the answer and not the timing.
func catalogOverEventually(t *testing.T, sess *yamux.Session) []store.CatalogRow {
	t.Helper()
	var lastErr error
	for i := 0; i < 100; i++ {
		resp, err := SessionClient(sess).Get("http://peer/fed/catalog")
		if err != nil {
			lastErr = err
			time.Sleep(20 * time.Millisecond)
			continue
		}
		defer resp.Body.Close()
		var rows []store.CatalogRow
		if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
			t.Fatalf("decode catalog: %v", err)
		}
		return rows
	}
	t.Fatalf("no catalog answer over the dialled session: %v", lastErr)
	return nil
}

// dialAsPeer completes the handshake against addr as peerID and returns the
// multiplexed session.
func dialAsPeer(t *testing.T, addr, peerID string) *yamux.Session {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	if err := dialHandshake(conn, "tok", peerID); err != nil {
		t.Fatal(err)
	}
	sess, err := yamux.Client(conn, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	return sess
}

// The hub scopes on the session's identity ONLY. An untagged request has no
// viewer, so once groups exist it discovers nothing — the same fail-closed rule
// PeerRoutes follows. An earlier version fell back to the path value here, which
// would have scoped the request against a name its caller chose.
func TestHubMergedCatalogDeniesAnUnidentifiedSession(t *testing.T) {
	db := groupDB(t)
	joinGroup(t, db, "family", "home", "office")
	relay := NewRelay(NewRegistry(), db)
	pushCatalog(t, relay, "office", rowsFor("OfficeSong"))

	rec := httptest.NewRecorder()
	relay.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/fed/catalog/home/merged", nil))

	var merged MergedCatalog
	if err := json.Unmarshal(rec.Body.Bytes(), &merged); err != nil {
		t.Fatal(err)
	}
	if len(merged.Catalogs["office"]) != 0 {
		t.Fatalf("untagged request got office's catalog by naming home in the path: %#v", merged.Catalogs["office"])
	}
}

// deniedAcceptedPeers is the backstop for a denied peer the hub holds no rows
// for — offline since the hub restarted, so the main loop never names it and the
// member would keep the rows forever. It reads federation_peer, which on a hub
// is populated by an admin adding peers through the API.
func TestHubMergedCatalogEmptiesADeniedPeerItHasNoRowsFor(t *testing.T) {
	db := groupDB(t)
	joinGroup(t, db, "family", "home")
	if err := store.SaveFederationPeer(db, store.FederationPeer{
		PeerID: "office", Address: "office:9000", Status: store.PeerStatusAccepted, TokenAuthenticated: true,
	}); err != nil {
		t.Fatal(err)
	}
	// Nothing pushed: the hub's in-memory cache holds no rows for office.
	relay := NewRelay(NewRegistry(), db)

	merged := fetchMerged(t, relay, "home")

	rows, named := merged.Catalogs["office"]
	if !named {
		t.Fatal("a denied peer with no cached rows must still be named, or the member never deletes its copy")
	}
	if len(rows) != 0 {
		t.Fatalf("catalog = %#v, want empty", rows)
	}
}
