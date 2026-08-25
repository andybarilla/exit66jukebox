package fed

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"log"
	"net"
	"strings"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

func TestParseLANPeerAnnouncementUsesSenderIPForWildcardAddress(t *testing.T) {
	body, _ := json.Marshal(lanPeerAnnouncement{PeerID: "peer-a", Address: ":9443"})
	peer, ok := parseLANPeerAnnouncement(body, &net.UDPAddr{IP: net.ParseIP("192.0.2.45"), Port: 5353})
	if !ok {
		t.Fatal("announcement should parse")
	}
	if peer.Address != "192.0.2.45:9443" {
		t.Fatalf("address = %q, want sender IP with advertised port", peer.Address)
	}
}

func TestParseLANPeerAnnouncementPreservesRoutableAddress(t *testing.T) {
	body, _ := json.Marshal(lanPeerAnnouncement{PeerID: "peer-a", Address: "203.0.113.10:9443"})
	peer, ok := parseLANPeerAnnouncement(body, &net.UDPAddr{IP: net.ParseIP("192.0.2.45"), Port: 5353})
	if !ok {
		t.Fatal("announcement should parse")
	}
	if peer.Address != "203.0.113.10:9443" {
		t.Fatalf("address = %q, want advertised routable address", peer.Address)
	}
}

func TestParseLANPeerAnnouncementUsesSenderIPForLoopbackAddress(t *testing.T) {
	body, _ := json.Marshal(lanPeerAnnouncement{PeerID: "peer-a", Address: "127.0.0.1:9443"})
	peer, ok := parseLANPeerAnnouncement(body, &net.UDPAddr{IP: net.ParseIP("192.0.2.45"), Port: 5353})
	if !ok {
		t.Fatal("announcement should parse")
	}
	if peer.Address != "192.0.2.45:9443" {
		t.Fatalf("address = %q, want sender IP with advertised port", peer.Address)
	}
}

func TestParseLANPeerAnnouncementUsesSenderIPForLocalhostAddress(t *testing.T) {
	body, _ := json.Marshal(lanPeerAnnouncement{PeerID: "peer-a", Address: "localhost:9443"})
	peer, ok := parseLANPeerAnnouncement(body, &net.UDPAddr{IP: net.ParseIP("192.0.2.45"), Port: 5353})
	if !ok {
		t.Fatal("announcement should parse")
	}
	if peer.Address != "192.0.2.45:9443" {
		t.Fatalf("address = %q, want sender IP with advertised port", peer.Address)
	}
}

func TestAnnouncementDoesNotDowngradeAnAcceptedPeer(t *testing.T) {
	db := mustOpenFedDB(t)
	if err := store.SaveFederationPeer(db, store.FederationPeer{PeerID: "peer-a", DisplayName: "Peer A",
		Address: "192.0.2.45:9443", Status: store.PeerStatusAccepted, Manual: true, TokenAuthenticated: true}); err != nil {
		t.Fatalf("save approved peer: %v", err)
	}
	body, _ := json.Marshal(lanPeerAnnouncement{PeerID: "peer-a", DisplayName: "Peer A", Address: ":9443"})
	sender := &net.UDPAddr{IP: net.ParseIP("192.0.2.45"), Port: 5353}

	// Several ticks of the 30s announcement loop.
	for i := 0; i < 3; i++ {
		handleLANPeerAnnouncement(db, body, sender, "self")
	}

	peers, err := store.ListFederationPeers(db, store.PeerStatusAccepted)
	if err != nil {
		t.Fatalf("list accepted peers: %v", err)
	}
	if len(peers) != 1 {
		t.Fatalf("accepted peers = %#v, want the approved peer still accepted", peers)
	}
	if !peers[0].TokenAuthenticated {
		t.Error("token_authenticated cleared by an announcement")
	}
	if !peers[0].Manual {
		t.Error("manual cleared by an announcement")
	}
}

func TestAnnouncementSurfacesAnUnknownPeerForApproval(t *testing.T) {
	db := mustOpenFedDB(t)
	body, _ := json.Marshal(lanPeerAnnouncement{PeerID: "peer-new", DisplayName: "Peer New", Address: ":9443"})

	handleLANPeerAnnouncement(db, body, &net.UDPAddr{IP: net.ParseIP("192.0.2.60"), Port: 5353}, "self")

	peers, err := store.ListFederationPeers(db, store.PeerStatusPending)
	if err != nil {
		t.Fatalf("list pending peers: %v", err)
	}
	if len(peers) != 1 || peers[0].PeerID != "peer-new" || peers[0].Address != "192.0.2.60:9443" {
		t.Fatalf("pending peers = %#v", peers)
	}
	if peers[0].TokenAuthenticated || peers[0].Manual {
		t.Errorf("discovered peer = %#v, want unauthenticated and not manual", peers[0])
	}
}

func TestOwnAnnouncementIsIgnored(t *testing.T) {
	db := mustOpenFedDB(t)
	body, _ := json.Marshal(lanPeerAnnouncement{PeerID: "self", Address: ":9443"})

	handleLANPeerAnnouncement(db, body, &net.UDPAddr{IP: net.ParseIP("192.0.2.45"), Port: 5353}, "self")

	peers, err := store.ListFederationPeers(db, "")
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if len(peers) != 0 {
		t.Fatalf("peers = %#v, want this instance not to record itself", peers)
	}
}

func TestStartupNamesApprovalsDiscoveryMayHaveReset(t *testing.T) {
	db := mustOpenFedDB(t)
	body, _ := json.Marshal(lanPeerAnnouncement{PeerID: "peer-new", Address: ":9443"})
	handleLANPeerAnnouncement(db, body, &net.UDPAddr{IP: net.ParseIP("192.0.2.60"), Port: 5353}, "self")

	if got := captureLog(t, func() { warnAboutApprovalsDiscoveryMayHaveReset(db) }); !strings.Contains(got, "#187") ||
		!strings.Contains(got, "approve it once more") {
		t.Errorf("startup log = %q, want it to name #187 and the remedy", got)
	}
}

func TestStartupIsQuietWithNothingToWarnAbout(t *testing.T) {
	db := mustOpenFedDB(t)
	if err := store.SaveFederationPeer(db, store.FederationPeer{PeerID: "peer-a", Address: "192.0.2.45:9443",
		Status: store.PeerStatusAccepted, TokenAuthenticated: true}); err != nil {
		t.Fatalf("save approved peer: %v", err)
	}

	if got := captureLog(t, func() { warnAboutApprovalsDiscoveryMayHaveReset(db) }); got != "" {
		t.Errorf("startup log = %q, want silence when every peer is authenticated", got)
	}
}

func mustOpenFedDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func captureLog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	out, flags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(out); log.SetFlags(flags) })
	fn()
	return buf.String()
}
