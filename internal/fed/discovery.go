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

func parseLANPeerAnnouncement(data []byte, sender *net.UDPAddr) (store.FederationPeer, bool) {
	var msg lanPeerAnnouncement
	if err := json.Unmarshal(data, &msg); err != nil {
		return store.FederationPeer{}, false
	}
	address, ok := lanDialAddress(strings.TrimSpace(msg.Address), sender)
	if !ok {
		return store.FederationPeer{}, false
	}
	peer := store.FederationPeer{
		PeerID:      strings.TrimSpace(msg.PeerID),
		DisplayName: strings.TrimSpace(msg.DisplayName),
		Address:     address,
		Status:      store.PeerStatusPending,
	}
	if peer.PeerID == "" {
		return store.FederationPeer{}, false
	}
	return peer, true
}

func lanDialAddress(address string, sender *net.UDPAddr) (string, bool) {
	if address == "" {
		return "", false
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil || port == "" {
		return "", false
	}
	parsedHost := net.ParseIP(host)
	if strings.EqualFold(host, "localhost") {
		return lanDialAddressFromSender(sender, port)
	}
	if host != "" && parsedHost == nil {
		return address, true
	}
	if parsedHost != nil && !parsedHost.IsUnspecified() && !parsedHost.IsLoopback() {
		return address, true
	}
	return lanDialAddressFromSender(sender, port)
}

func lanDialAddressFromSender(sender *net.UDPAddr, port string) (string, bool) {
	if sender == nil || sender.IP == nil {
		return "", false
	}
	return net.JoinHostPort(sender.IP.String(), port), true
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
		n, sender, err := conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				continue
			}
		}
		peer, ok := parseLANPeerAnnouncement(buf[:n], sender)
		if !ok || peer.PeerID == selfID {
			continue
		}
		_ = store.SaveFederationPeer(db, peer)
	}
}
