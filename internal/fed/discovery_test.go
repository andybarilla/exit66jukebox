package fed

import (
	"encoding/json"
	"net"
	"testing"
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
