package fed

import "testing"

func TestParseLANPeerAnnouncement(t *testing.T) {
	peer, ok := parseLANPeerAnnouncement([]byte(`{"peer_id":"peer-a","display_name":"Peer A","address":"host.local:9443"}`))
	if !ok {
		t.Fatal("announcement should parse")
	}
	if peer.PeerID != "peer-a" || peer.DisplayName != "Peer A" || peer.Address != "host.local:9443" {
		t.Fatalf("peer announcement = %#v", peer)
	}
}
