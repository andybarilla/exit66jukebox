package store

import (
	"database/sql"
	"testing"
)

// A sighting is what a LAN announcement is allowed to be: it learns an address
// and a name, and it never has an opinion about trust. #187 is the bug where it
// did — the announcement ticker walked an operator's approval back to pending
// every 30 seconds, permanently.

func peerRow(t *testing.T, db *sql.DB, peerID, address string) FederationPeer {
	t.Helper()
	row := db.QueryRow(`SELECT id, peer_id, display_name, address, status, manual, token_authenticated, last_seen_at, created_at, updated_at
		 FROM federation_peer WHERE peer_id = ? AND address = ?`, peerID, address)
	peer, err := scanFederationPeer(row)
	if err != nil {
		t.Fatalf("read peer %s at %s: %v", peerID, address, err)
	}
	return peer
}

func TestSightingLeavesAnAcceptedPeerAccepted(t *testing.T) {
	db := mustOpenMem(t)
	approved := FederationPeer{PeerID: "peer-a", DisplayName: "Peer A", Address: "192.168.1.9:9443",
		Status: PeerStatusAccepted, Manual: true, TokenAuthenticated: true}
	if err := SaveFederationPeer(db, approved); err != nil {
		t.Fatalf("save approved peer: %v", err)
	}

	// Several ticks of the announcement loop, which is how the bug reproduced.
	for i := 0; i < 3; i++ {
		if err := RecordFederationPeerSighting(db, "peer-a", "Peer A", "192.168.1.9:9443"); err != nil {
			t.Fatalf("sighting %d: %v", i, err)
		}
	}

	got := peerRow(t, db, "peer-a", "192.168.1.9:9443")
	if got.Status != PeerStatusAccepted {
		t.Errorf("status = %q, want %q", got.Status, PeerStatusAccepted)
	}
	if !got.TokenAuthenticated {
		t.Error("token_authenticated cleared by a sighting")
	}
	if !got.Manual {
		t.Error("manual cleared by a sighting")
	}
}

func TestSightingUpdatesAddressNameAndLastSeen(t *testing.T) {
	db := mustOpenMem(t)
	if err := SaveFederationPeer(db, FederationPeer{PeerID: "peer-a", DisplayName: "Old Name",
		Address: "192.168.1.9:9443", Status: PeerStatusAccepted, TokenAuthenticated: true}); err != nil {
		t.Fatalf("save peer: %v", err)
	}
	// SaveFederationPeer stamps last_seen_at with the same second a sighting
	// would, so zero it to tell "the sighting wrote it" from "the save did".
	if _, err := db.Exec(`UPDATE federation_peer SET last_seen_at = 0`); err != nil {
		t.Fatalf("clear last_seen_at: %v", err)
	}

	if err := RecordFederationPeerSighting(db, "peer-a", "New Name", "192.168.1.9:9443"); err != nil {
		t.Fatalf("sighting: %v", err)
	}

	got := peerRow(t, db, "peer-a", "192.168.1.9:9443")
	if got.DisplayName != "New Name" {
		t.Errorf("display_name = %q, want the announced name", got.DisplayName)
	}
	if got.LastSeenAt == 0 {
		t.Error("last_seen_at not advanced by a sighting")
	}
	if got.Address != "192.168.1.9:9443" {
		t.Errorf("address = %q", got.Address)
	}
}

func TestSightingKeepsTheOperatorsNameWhenTheAnnouncementHasNone(t *testing.T) {
	db := mustOpenMem(t)
	if err := SaveFederationPeer(db, FederationPeer{PeerID: "peer-a", DisplayName: "Kitchen",
		Address: "192.168.1.9:9443", Status: PeerStatusAccepted, TokenAuthenticated: true}); err != nil {
		t.Fatalf("save peer: %v", err)
	}

	if err := RecordFederationPeerSighting(db, "peer-a", "", "192.168.1.9:9443"); err != nil {
		t.Fatalf("sighting: %v", err)
	}

	if got := peerRow(t, db, "peer-a", "192.168.1.9:9443"); got.DisplayName != "Kitchen" {
		t.Errorf("display_name = %q, want the operator's name kept", got.DisplayName)
	}
}

func TestSightingInsertsAnUnknownPeerAsPendingAndUnauthenticated(t *testing.T) {
	db := mustOpenMem(t)

	if err := RecordFederationPeerSighting(db, "peer-new", "Peer New", "192.168.1.40:9443"); err != nil {
		t.Fatalf("sighting: %v", err)
	}

	got := peerRow(t, db, "peer-new", "192.168.1.40:9443")
	if got.Status != PeerStatusPending {
		t.Errorf("status = %q, want %q so the peer surfaces for approval", got.Status, PeerStatusPending)
	}
	if got.TokenAuthenticated {
		t.Error("an unhandshaken peer must not be token_authenticated")
	}
	if got.Manual {
		t.Error("a discovered peer must not be marked manual")
	}
	if got.DisplayName != "Peer New" || got.LastSeenAt == 0 {
		t.Errorf("inserted peer = %#v", got)
	}
}

func TestSightingDoesNotClearQuarantine(t *testing.T) {
	db := mustOpenMem(t)
	if err := SaveFederationPeer(db, FederationPeer{PeerID: "peer-q", Address: "192.168.1.9:9443",
		Status: PeerStatusQuarantined}); err != nil {
		t.Fatalf("save quarantined peer: %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := RecordFederationPeerSighting(db, "peer-q", "Peer Q", "192.168.1.9:9443"); err != nil {
			t.Fatalf("sighting %d: %v", i, err)
		}
	}

	if got := peerRow(t, db, "peer-q", "192.168.1.9:9443"); got.Status != PeerStatusQuarantined {
		t.Errorf("status = %q, want the quarantine to hold", got.Status)
	}
}

func TestSightingQuarantinesADuplicatePeerIDOnInsert(t *testing.T) {
	db := mustOpenMem(t)
	if err := SaveFederationPeer(db, FederationPeer{PeerID: "peer-c", Address: "10.0.0.1:9443",
		Status: PeerStatusAccepted, TokenAuthenticated: true}); err != nil {
		t.Fatalf("save peer: %v", err)
	}

	if err := RecordFederationPeerSighting(db, "peer-c", "Peer C", "10.0.0.2:9443"); err != nil {
		t.Fatalf("sighting: %v", err)
	}

	if got := peerRow(t, db, "peer-c", "10.0.0.2:9443"); got.Status != PeerStatusQuarantined {
		t.Errorf("new address status = %q, want %q", got.Status, PeerStatusQuarantined)
	}
	if got := peerRow(t, db, "peer-c", "10.0.0.1:9443"); got.Status != PeerStatusAccepted {
		t.Errorf("original address status = %q, want it untouched", got.Status)
	}
}

func TestApprovingAPeerAfterASightingSucceeds(t *testing.T) {
	db := mustOpenMem(t)
	if err := RecordFederationPeerSighting(db, "peer-b", "Peer B", "192.168.1.9:9443"); err != nil {
		t.Fatalf("first sighting: %v", err)
	}
	if err := MarkFederationPeerAuthenticated(db, "peer-b"); err != nil {
		t.Fatalf("mark authenticated: %v", err)
	}
	// The tick that used to clear token_authenticated out from under approval.
	if err := RecordFederationPeerSighting(db, "peer-b", "Peer B", "192.168.1.9:9443"); err != nil {
		t.Fatalf("second sighting: %v", err)
	}

	if err := ApproveFederationPeer(db, "peer-b"); err != nil {
		t.Fatalf("approve after sighting: %v", err)
	}
	if got := peerRow(t, db, "peer-b", "192.168.1.9:9443"); got.Status != PeerStatusAccepted {
		t.Errorf("status = %q, want %q", got.Status, PeerStatusAccepted)
	}
}

func TestSightingRejectsAnIncompleteAnnouncement(t *testing.T) {
	if err := RecordFederationPeerSighting(mustOpenMem(t), "", "Peer", "192.168.1.9:9443"); err == nil {
		t.Error("a sighting with no peer id should be rejected")
	}
	if err := RecordFederationPeerSighting(mustOpenMem(t), "peer-a", "Peer", ""); err == nil {
		t.Error("a sighting with no address should be rejected")
	}
}
