package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/scan"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

func withAPIPathHomeDir(t *testing.T, fn func() (string, error)) {
	t.Helper()
	old := apiPathHomeDir
	apiPathHomeDir = fn
	t.Cleanup(func() { apiPathHomeDir = old })
}

type testLibraryPathEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type testLibraryPathsResponse struct {
	Path        string                 `json:"path"`
	Parent      string                 `json:"parent"`
	Directories []testLibraryPathEntry `json:"directories"`
}

func TestAdminLibrariesRequiresAdmin(t *testing.T) {
	s, _ := newTestServer(t)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/libraries", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET libraries without session: want 401, got %d", rec.Code)
	}
}

func TestAdminLibrariesSaveValidatesPathsAndPreservesSettings(t *testing.T) {
	s, db := newTestServer(t)
	req := adminReq(t, db, http.MethodPost, "/api/admin/libraries", `{"local_libraries":[{"path":"/music"},{"path":"/music/."}],"federation":{"enabled":false}}`)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("duplicate paths: want 400, got %d (%s)", rec.Code, rec.Body)
	}
}

func TestAdminLibrariesSaveRejectsInvalidFederationWithoutSavingLibraries(t *testing.T) {
	s, db := newTestServer(t)
	if err := store.SaveLocalLibraries(db, []store.LocalLibrary{{Path: "/original", Enabled: true}}); err != nil {
		t.Fatalf("save original library: %v", err)
	}
	body := `{"local_libraries":[{"path":"/changed","enabled":true}],"federation":{"enabled":true,"role":"member","token":"secret","peer_id":"peer"}}`
	req := adminReq(t, db, http.MethodPost, "/api/admin/libraries", body)
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid federation: want 400, got %d (%s)", rec.Code, rec.Body)
	}
	libs, err := store.ListLocalLibraries(db)
	if err != nil {
		t.Fatalf("list libraries: %v", err)
	}
	if len(libs) != 1 || libs[0].Path != "/original" {
		t.Fatalf("library changes should not persist after invalid federation: %#v", libs)
	}
}

func TestAdminLibrariesSaveAndScanRejectsConcurrentScan(t *testing.T) {
	s, db := newTestServer(t)
	p := &scan.Progress{}
	p.SetRunning(true)
	s.SetScanProgress(p)

	req := adminReq(t, db, http.MethodPost, "/api/admin/libraries", `{"save_and_scan":true,"local_libraries":[{"path":"/tmp","enabled":true}],"federation":{"enabled":false}}`)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("concurrent scan: want 409, got %d (%s)", rec.Code, rec.Body)
	}
	libs, _ := store.ListLocalLibraries(db)
	if len(libs) != 1 || libs[0].Path != "/tmp" {
		t.Fatalf("save should persist despite scan conflict: %#v", libs)
	}
}

func TestScanProgressAccessIsConcurrentSafe(t *testing.T) {
	s, _ := newTestServer(t)

	var wg sync.WaitGroup
	for range 64 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			s.SetScanProgress(&scan.Progress{})
		}()
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			s.scanStatus(rec, httptest.NewRequest(http.MethodGet, "/api/scan", nil))
		}()
	}
	wg.Wait()
}

func TestAdminLibrariesReadMasksTokenAndReportsRestartRequired(t *testing.T) {
	s, db := newTestServer(t)
	s.SetActiveFederation(store.FederationSettings{Enabled: false})
	if err := store.SaveFederationSettings(db, store.FederationSettings{Enabled: true, Role: "hub", Listen: ":9443", Token: "secret", PeerID: "peer"}); err != nil {
		t.Fatalf("save federation: %v", err)
	}
	req := adminReq(t, db, http.MethodGet, "/api/admin/libraries", "")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("read libraries: want 200, got %d (%s)", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if strings.Contains(body, "secret") {
		t.Fatalf("response leaked federation token: %s", body)
	}
	if !strings.Contains(body, `"restart_required":true`) {
		t.Fatalf("response should require restart: %s", body)
	}
}

func TestAdminFederationPeersManualAddAndList(t *testing.T) {
	s, db := newTestServer(t)
	req := adminReq(t, db, http.MethodPost, "/api/admin/federation/peers", `{"peer_id":"peer-a","display_name":"Peer A","address":"127.0.0.1:9443"}`)
	cookies := req.Cookies()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("manual peer add: want 200, got %d (%s)", rec.Code, rec.Body)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/admin/federation/peers", nil)
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list peers: want 200, got %d (%s)", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"peer_id":"peer-a"`) || !strings.Contains(body, `"status":"accepted"`) {
		t.Fatalf("manual accepted peer missing from response: %s", body)
	}
}

func TestAdminFederationPeersApprovesAuthenticatedDiscovery(t *testing.T) {
	s, db := newTestServer(t)
	if err := store.SaveFederationPeer(db, store.FederationPeer{PeerID: "peer-b", Address: "192.168.1.9:9443", Status: store.PeerStatusPending, TokenAuthenticated: true}); err != nil {
		t.Fatalf("seed peer: %v", err)
	}
	req := adminReq(t, db, http.MethodPost, "/api/admin/federation/peers/peer-b/approve", "")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("approve peer: want 200, got %d (%s)", rec.Code, rec.Body)
	}
	peers, err := store.ListFederationPeers(db, store.PeerStatusAccepted)
	if err != nil || len(peers) != 1 || peers[0].PeerID != "peer-b" {
		t.Fatalf("accepted peers = %#v err=%v", peers, err)
	}
}

func TestAdminLibraryPathsRequiresAdmin(t *testing.T) {
	s, db := newTestServer(t)

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/library-paths", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("library paths without session: want 401, got %d", rec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/library-paths", nil)
	req.AddCookie(nonAdminCookie(t, db))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("library paths as non-admin: want 403, got %d", rec.Code)
	}
}

func TestAdminLibraryPathsListsSortedDirectories(t *testing.T) {
	s, db := newTestServer(t)
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "zeta"))
	mkdir(t, filepath.Join(root, "Alpha"))
	unreadable := filepath.Join(root, "unreadable")
	makeUnreadableDir(t, unreadable)
	writeFile(t, filepath.Join(root, "track.mp3"))

	body := getLibraryPaths(t, s, adminReq(t, db, http.MethodGet, libraryPathURL(root), ""), http.StatusOK)
	cleanedRoot := filepath.Clean(root)
	if body.Path != cleanedRoot {
		t.Fatalf("path: want %q, got %q", cleanedRoot, body.Path)
	}
	if body.Parent != filepath.Dir(cleanedRoot) {
		t.Fatalf("parent: want %q, got %q", filepath.Dir(cleanedRoot), body.Parent)
	}
	want := []testLibraryPathEntry{{Name: "Alpha", Path: filepath.Clean(filepath.Join(root, "Alpha"))}, {Name: "zeta", Path: filepath.Clean(filepath.Join(root, "zeta"))}}
	if !sameLibraryPathEntries(body.Directories, want) {
		t.Fatalf("directories: want %#v, got %#v", want, body.Directories)
	}
}

func TestAdminLibraryPathsHidesHiddenDirectories(t *testing.T) {
	s, db := newTestServer(t)
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "Music"))
	mkdir(t, filepath.Join(root, ".git"))
	mkdir(t, filepath.Join(root, "@eaDir"))

	body := getLibraryPaths(t, s, adminReq(t, db, http.MethodGet, libraryPathURL(root), ""), http.StatusOK)
	want := []testLibraryPathEntry{{Name: "Music", Path: filepath.Clean(filepath.Join(root, "Music"))}}
	if !sameLibraryPathEntries(body.Directories, want) {
		t.Fatalf("directories: want %#v, got %#v", want, body.Directories)
	}
}

func TestAdminLibraryPathsDefaultStartsAtSavedLibrary(t *testing.T) {
	s, db := newTestServer(t)
	missing := filepath.Join(t.TempDir(), "missing")
	unreadable := filepath.Join(t.TempDir(), "unreadable")
	makeUnreadableDir(t, unreadable)
	readable := t.TempDir()
	if err := store.SaveLocalLibraries(db, []store.LocalLibrary{{Path: missing, Enabled: true}, {Path: unreadable, Enabled: true}, {Path: readable, Enabled: false}}); err != nil {
		t.Fatalf("save libraries: %v", err)
	}

	body := getLibraryPaths(t, s, adminReq(t, db, http.MethodGet, "/api/admin/library-paths", ""), http.StatusOK)
	if body.Path != filepath.Clean(readable) {
		t.Fatalf("default path: want %q, got %q", filepath.Clean(readable), body.Path)
	}
}

func TestAdminLibraryPathsRejectsInvalidRequestedPaths(t *testing.T) {
	s, db := newTestServer(t)
	adminCookie := adminReq(t, db, http.MethodGet, "/api/admin/library-paths", "").Cookies()[0]
	missing := filepath.Join(t.TempDir(), "missing")
	assertLibraryPathError(t, s, adminLibraryPathReq(libraryPathURL(missing), adminCookie), http.StatusBadRequest, "")

	filePath := filepath.Join(t.TempDir(), "track.mp3")
	writeFile(t, filePath)
	assertLibraryPathError(t, s, adminLibraryPathReq(libraryPathURL(filePath), adminCookie), http.StatusBadRequest, "not a directory")

	unreadable := filepath.Join(t.TempDir(), "unreadable")
	makeUnreadableDir(t, unreadable)
	assertLibraryPathError(t, s, adminLibraryPathReq(libraryPathURL(unreadable), adminCookie), http.StatusBadRequest, "not readable")

	assertLibraryPathError(t, s, adminLibraryPathReq(libraryPathURL("~other/Music"), adminCookie), http.StatusBadRequest, "unsupported")
}

func TestAdminLibraryPathsExpandsHomeAndReportsHomeFailure(t *testing.T) {
	s, db := newTestServer(t)
	adminCookie := adminReq(t, db, http.MethodGet, "/api/admin/library-paths", "").Cookies()[0]
	home := t.TempDir()
	withAPIPathHomeDir(t, func() (string, error) { return home, nil })

	body := getLibraryPaths(t, s, adminLibraryPathReq(libraryPathURL("~"), adminCookie), http.StatusOK)
	if body.Path != filepath.Clean(home) {
		t.Fatalf("expanded home path: want %q, got %q", filepath.Clean(home), body.Path)
	}
	child := filepath.Join(home, "Music")
	mkdir(t, child)
	body = getLibraryPaths(t, s, adminLibraryPathReq(libraryPathURL("~/Music"), adminCookie), http.StatusOK)
	if body.Path != filepath.Clean(child) {
		t.Fatalf("expanded home child path: want %q, got %q", filepath.Clean(child), body.Path)
	}

	withAPIPathHomeDir(t, func() (string, error) { return "", os.ErrPermission })
	assertLibraryPathError(t, s, adminLibraryPathReq(libraryPathURL("~"), adminCookie), http.StatusInternalServerError, "")
}

func TestAdminLibraryPathsAllowsSymlinkDirectoriesWithLexicalPaths(t *testing.T) {
	s, db := newTestServer(t)
	root := t.TempDir()
	target := t.TempDir()
	link := filepath.Join(root, "linked")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	body := getLibraryPaths(t, s, adminReq(t, db, http.MethodGet, libraryPathURL(root), ""), http.StatusOK)
	want := []testLibraryPathEntry{{Name: "linked", Path: filepath.Clean(link)}}
	if !sameLibraryPathEntries(body.Directories, want) {
		t.Fatalf("directories: want lexical symlink path %#v, got %#v", want, body.Directories)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("music"), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

func makeUnreadableDir(t *testing.T, path string) {
	t.Helper()
	mkdir(t, path)
	if err := os.Chmod(path, 0); err != nil {
		t.Fatalf("chmod unreadable %s: %v", path, err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o755) })
	if _, err := os.ReadDir(path); err == nil {
		t.Skip("filesystem permissions allow reading chmod 000 directories")
	}
}

func libraryPathURL(path string) string {
	return "/api/admin/library-paths?path=" + url.QueryEscape(path)
}

func adminLibraryPathReq(path string, cookie *http.Cookie) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(cookie)
	return req
}

func getLibraryPaths(t *testing.T, s *Server, req *http.Request, wantStatus int) testLibraryPathsResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("library paths: want %d, got %d (%s)", wantStatus, rec.Code, rec.Body)
	}
	var body testLibraryPathsResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode library paths: %v", err)
	}
	return body
}

func assertLibraryPathError(t *testing.T, s *Server, req *http.Request, wantStatus int, wantErrorPart string) {
	t.Helper()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("library paths error: want %d, got %d (%s)", wantStatus, rec.Code, rec.Body)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode library paths error: %v", err)
	}
	if body.Error == "" {
		t.Fatalf("error response should include an error message")
	}
	if wantErrorPart != "" && !strings.Contains(body.Error, wantErrorPart) {
		t.Fatalf("error: want substring %q, got %q", wantErrorPart, body.Error)
	}
}

func sameLibraryPathEntries(got, want []testLibraryPathEntry) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
