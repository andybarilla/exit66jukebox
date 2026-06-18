package store

import "testing"

func TestAcceptedPeersPersistManualConfiguration(t *testing.T) {
	db := mustOpenMem(t)
	peer := FederationPeer{PeerID: "peer-a", DisplayName: "Peer A", Address: "127.0.0.1:9443", Status: PeerStatusAccepted, Manual: true}

	if err := SaveFederationPeer(db, peer); err != nil {
		t.Fatalf("save peer: %v", err)
	}
	got, err := ListFederationPeers(db, PeerStatusAccepted)
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if len(got) != 1 || got[0].PeerID != peer.PeerID || got[0].Address != peer.Address || !got[0].Manual {
		t.Fatalf("accepted peers = %#v", got)
	}
}

func TestDiscoveredPeerApprovalRequiresTokenAuth(t *testing.T) {
	db := mustOpenMem(t)
	discovered := FederationPeer{PeerID: "peer-b", DisplayName: "Peer B", Address: "192.168.1.9:9443", Status: PeerStatusPending}

	if err := SaveFederationPeer(db, discovered); err != nil {
		t.Fatalf("save discovered peer: %v", err)
	}
	if err := ApproveFederationPeer(db, "peer-b"); err == nil {
		t.Fatal("approval without token auth should fail")
	}
	if err := MarkFederationPeerAuthenticated(db, "peer-b"); err != nil {
		t.Fatalf("mark authenticated: %v", err)
	}
	if err := ApproveFederationPeer(db, "peer-b"); err != nil {
		t.Fatalf("approve authenticated peer: %v", err)
	}
	got, err := ListFederationPeers(db, PeerStatusAccepted)
	if err != nil {
		t.Fatalf("list accepted peers: %v", err)
	}
	if len(got) != 1 || got[0].Status != PeerStatusAccepted || !got[0].TokenAuthenticated {
		t.Fatalf("accepted peer = %#v", got)
	}
}

func TestDuplicatePeerIDsAreQuarantined(t *testing.T) {
	db := mustOpenMem(t)
	if err := SaveFederationPeer(db, FederationPeer{PeerID: "peer-c", Address: "10.0.0.1:9443", Status: PeerStatusAccepted}); err != nil {
		t.Fatalf("save peer: %v", err)
	}
	if err := SaveFederationPeer(db, FederationPeer{PeerID: "peer-c", Address: "10.0.0.2:9443", Status: PeerStatusPending}); err != nil {
		t.Fatalf("save duplicate peer: %v", err)
	}
	got, err := ListFederationPeers(db, PeerStatusQuarantined)
	if err != nil {
		t.Fatalf("list quarantined peers: %v", err)
	}
	if len(got) != 1 || got[0].Address != "10.0.0.2:9443" {
		t.Fatalf("quarantined duplicate peers = %#v", got)
	}
}
