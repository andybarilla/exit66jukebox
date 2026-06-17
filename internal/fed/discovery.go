package fed

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"strings"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

const lanDiscoveryAddr = "224.0.0.251:5353"

type lanPeerAnnouncement struct {
	PeerID      string `json:"peer_id"`
	DisplayName string `json:"display_name"`
	Address     string `json:"address"`
}

func StartLANDiscovery(ctx context.Context, db *sql.DB, selfID, displayName, address string) {
	announcement := lanPeerAnnouncement{PeerID: strings.TrimSpace(selfID), DisplayName: strings.TrimSpace(displayName), Address: strings.TrimSpace(address)}
	if db == nil || announcement.PeerID == "" || announcement.Address == "" {
		return
	}
	go broadcastLANPeer(ctx, announcement)
	go listenForLANPeers(ctx, db, announcement.PeerID)
}

func parseLANPeerAnnouncement(data []byte) (store.FederationPeer, bool) {
	var msg lanPeerAnnouncement
	if err := json.Unmarshal(data, &msg); err != nil {
		return store.FederationPeer{}, false
	}
	peer := store.FederationPeer{
		PeerID:      strings.TrimSpace(msg.PeerID),
		DisplayName: strings.TrimSpace(msg.DisplayName),
		Address:     strings.TrimSpace(msg.Address),
		Status:      store.PeerStatusPending,
	}
	if peer.PeerID == "" || peer.Address == "" {
		return store.FederationPeer{}, false
	}
	return peer, true
}

func broadcastLANPeer(ctx context.Context, msg lanPeerAnnouncement) {
	addr, err := net.ResolveUDPAddr("udp4", lanDiscoveryAddr)
	if err != nil {
		return
	}
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return
	}
	defer conn.Close()
	body, err := json.Marshal(msg)
	if err != nil {
		return
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		conn.Write(body)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func listenForLANPeers(ctx context.Context, db *sql.DB, selfID string) {
	addr, err := net.ResolveUDPAddr("udp4", lanDiscoveryAddr)
	if err != nil {
		return
	}
	conn, err := net.ListenMulticastUDP("udp4", nil, addr)
	if err != nil {
		return
	}
	defer conn.Close()
	buf := make([]byte, 4096)
	for {
		conn.SetReadDeadline(time.Now().Add(time.Second))
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				continue
			}
		}
		peer, ok := parseLANPeerAnnouncement(buf[:n])
		if !ok || peer.PeerID == selfID {
			continue
		}
		_ = store.SaveFederationPeer(db, peer)
	}
}
