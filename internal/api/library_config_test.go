package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/scan"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

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
