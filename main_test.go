package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/config"
	"github.com/andybarilla/exit66jukebox/internal/external"
	"github.com/andybarilla/exit66jukebox/internal/fed"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

func testClient() *external.Client { return external.New("test", time.Second) }

// No credentials -> no client (fully disabled).
func TestNewLastfmNilWhenUnconfigured(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	if lfm := newLastfm(testClient(), db, config.Services{}); lfm != nil {
		t.Fatal("expected nil client with no creds")
	}
}

// Credentials present but no persisted session -> pending auth, still nil so
// nothing is enqueued or sent until `exit66jukebox lastfm-auth` runs.
func TestNewLastfmNilWhenPendingAuth(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	svc := config.Services{LastfmAPIKey: "k", LastfmAPISecret: "s"}
	if lfm := newLastfm(testClient(), db, svc); lfm != nil {
		t.Fatal("expected nil client when configured but not authorized")
	}
}

// Credentials + a persisted session row -> an authorized client.
func TestNewLastfmAuthorizedWithSession(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	if err := store.PutServiceAuth(db, "lastfm", "sk", "alice"); err != nil {
		t.Fatalf("PutServiceAuth: %v", err)
	}
	svc := config.Services{LastfmAPIKey: "k", LastfmAPISecret: "s"}
	lfm := newLastfm(testClient(), db, svc)
	if lfm == nil || !lfm.Authorized() {
		t.Fatalf("expected authorized client, got %v", lfm)
	}
}

// The enqueue gate is computed live: both services when ListenBrainz is on and
// Last.fm is authorized; Last.fm drops out when its client is nil.
func TestActiveScrobbleServices(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	store.PutServiceAuth(db, "lastfm", "sk", "alice")
	lfm := newLastfm(testClient(), db, config.Services{LastfmAPIKey: "k", LastfmAPISecret: "s"})

	if got := activeScrobbleServices(true, lfm); !eq(got, []string{"listenbrainz", "lastfm"}) {
		t.Errorf("both on = %v, want [listenbrainz lastfm]", got)
	}
	if got := activeScrobbleServices(false, lfm); !eq(got, []string{"lastfm"}) {
		t.Errorf("only lastfm = %v, want [lastfm]", got)
	}
	if got := activeScrobbleServices(true, nil); !eq(got, []string{"listenbrainz"}) {
		t.Errorf("lastfm nil = %v, want [listenbrainz]", got)
	}
	if got := activeScrobbleServices(false, nil); len(got) != 0 {
		t.Errorf("none = %v, want empty", got)
	}
}

func TestStartupLibraryRootsSeedFlagsThenUseEnabledDBRoots(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	cliRoot := filepath.Clean("/music/roots/../library")

	roots, err := startupLibraryRoots(db, []string{cliRoot})
	if err != nil {
		t.Fatalf("startup roots: %v", err)
	}
	if !eq(roots, []string{cliRoot}) {
		t.Fatalf("startup roots = %v, want [%s]", roots, cliRoot)
	}

	if err := store.SaveLocalLibraries(db, []store.LocalLibrary{{Path: cliRoot, Enabled: false}}); err != nil {
		t.Fatalf("save disabled library: %v", err)
	}
	roots, err = startupLibraryRoots(db, []string{cliRoot})
	if err != nil {
		t.Fatalf("startup roots after disable: %v", err)
	}
	if len(roots) != 0 {
		t.Fatalf("disabled DB library should suppress CLI root, got %v", roots)
	}
}

func TestFederationSettingsPreferDBOverEnv(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	dbSettings := store.FederationSettings{Enabled: true, Role: "hub", Listen: ":9443", Token: "db-token", PeerID: "db-peer", DirectP2P: true}
	if err := store.SaveFederationSettings(db, dbSettings); err != nil {
		t.Fatalf("save federation settings: %v", err)
	}

	got, err := federationSettings(db, config.Federation{Role: "member", HubAddr: "env-hub", Token: "env-token", PeerID: "env-peer"})
	if err != nil {
		t.Fatalf("federation settings: %v", err)
	}
	if !store.FederationSettingsEqual(got, dbSettings) {
		t.Fatalf("federation settings = %#v, want %#v", got, dbSettings)
	}
}

func TestMainWiresConfiguredMFAKeyIntoAPIServer(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}

	serverIndex := strings.Index(string(source), "srv := api.NewServer(db, jb, uiFS)")
	if serverIndex < 0 {
		t.Fatal("main.go should construct the API server")
	}

	mfaKeyIndex := strings.Index(string(source)[serverIndex:], "srv.SetMFAKey(cfg.MFAKey)")
	if mfaKeyIndex < 0 {
		t.Fatal("main.go should pass cfg.MFAKey to the API server after construction")
	}
}

func TestBootstrapTokenOnlyGeneratedForEmptyUserTable(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()

	first, ok := bootstrapToken(db)
	if !ok || first == "" {
		t.Fatal("empty user table should get a bootstrap token")
	}
	second, ok := bootstrapToken(db)
	if !ok || second == "" || second == first {
		t.Fatalf("new startup should get a fresh token, first=%q second=%q ok=%v", first, second, ok)
	}
	if _, err := store.CreateUser(db, "admin@example.com", "Admin", "h", true, true); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if token, ok := bootstrapToken(db); ok || token != "" {
		t.Fatalf("existing users should not get bootstrap token: token=%q ok=%v", token, ok)
	}
}

// waitForClose returns true once the hub goroutine signals done, false if the
// bounded timeout expires first (so shutdown can't hang on a stuck goroutine).
func TestWaitForClose(t *testing.T) {
	closed := make(chan struct{})
	close(closed)
	if !waitForClose(closed, 50*time.Millisecond) {
		t.Error("already-closed channel should return true")
	}

	done := make(chan struct{})
	go func() {
		time.Sleep(10 * time.Millisecond)
		close(done)
	}()
	if !waitForClose(done, time.Second) {
		t.Error("channel closing before timeout should return true")
	}

	start := time.Now()
	if waitForClose(make(chan struct{}), 20*time.Millisecond) {
		t.Error("never-closing channel should return false at timeout")
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Errorf("timeout wait took %v, expected ~20ms", elapsed)
	}
}

func eq(a, b []string) bool {
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

// A personal stream id is derived from a user and boot knows no users, so the
// only row boot could create is the global one every listener shared (#128).
// Reintroducing the call would leave every test green while re-creating it, so
// this stays a source grep. It matches the call shape rather than the bare
// name, which would fire on any legitimate future use of the function.
func TestMainDoesNotProvisionAPersonalStreamAtBoot(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	if strings.Contains(string(source), "store.EnsurePrivateStream(db,") {
		t.Fatal("main.go must not provision a personal stream at boot: the id is per-user and boot has no user")
	}
}

// TestFedManagerHandlersRefuseApplicationRoutes observes the handlers as
// newFedManager attaches them, so a member- or peer-side assignment that
// bypassed fed.AppRoutes would fail here. The allowlist's own behaviour is
// covered in internal/fed and internal/api; this pins the wiring.
func TestFedManagerHandlersRefuseApplicationRoutes(t *testing.T) {
	reached := ""
	app := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = r.Method + " " + r.URL.Path
		w.WriteHeader(http.StatusOK)
	})
	fm := newFedManager(
		store.FederationSettings{Enabled: true, Role: "peer", PeerID: "p1", DirectP2P: true},
		nil, app, fed.NewRegistry(), fed.NewSignaler())

	for name, h := range map[string]http.Handler{"member": fm.MemberHandler, "peer": fm.PeerHandler} {
		if h == nil {
			t.Fatalf("%s handler not attached", name)
		}
		reached = ""
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/streams/house/requests", nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d, want 404", name, rec.Code)
		}
		if reached != "" {
			t.Fatalf("%s: %s reached the application handler", name, reached)
		}
		// The one allowlisted application route still gets through.
		reached = ""
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tracks/7/audio", nil))
		if rec.Code != http.StatusOK || reached != "GET /api/tracks/7/audio" {
			t.Fatalf("%s track audio: status %d, reached %q", name, rec.Code, reached)
		}
	}
}

// The lastfm-auth hint is the only instruction an operator gets — the
// subcommand appears nowhere in the UI — so a hint naming a binary the build
// does not produce sends them to `command not found` (#149). The expected name
// is read out of the Makefile's build rule rather than written here, so
// renaming the binary fails this test instead of silently stranding the hints.
//
// The name pattern starts at [a-z0-9] so this file's own regexp literal, whose
// first character after the backtick is `(`, is not mistaken for a hint.
func TestLastfmAuthHintsNameTheBuiltBinary(t *testing.T) {
	makefile, err := os.ReadFile("Makefile")
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	build := regexp.MustCompile(`go build -o (\S+) \.`).FindSubmatch(makefile)
	if build == nil {
		t.Fatal("Makefile should build the binary with `go build -o <name> .`")
	}
	binary := string(build[1])

	hint := regexp.MustCompile("`([a-z0-9][^`]*) lastfm-auth`")
	found := 0
	err = filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range hint.FindAllSubmatch(source, -1) {
			found++
			if got := string(m[1]); got != binary {
				t.Errorf("%s: lastfm-auth hint names %q, Makefile builds %q", path, got, binary)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if found == 0 {
		t.Fatal("no `<binary> lastfm-auth` hints found; this test no longer guards anything")
	}
}
