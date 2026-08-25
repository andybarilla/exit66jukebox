package fed

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
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
	warnAboutApprovalsDiscoveryMayHaveReset(db)
	go broadcastLANPeer(ctx, announcement)
	go listenForLANPeers(ctx, db, announcement.PeerID)
}

// warnAboutApprovalsDiscoveryMayHaveReset names #187's damage on the way past,
// because it is deliberately not repaired. The clobber overwrote every field
// that told an approved peer from a never-approved one — status, the token flag
// and manual — so a migration that re-approved anything would be guessing, and
// a guess in a trust decision fails in the wrong direction.
//
// The count is a hint, not a diagnosis: a peer that has genuinely never been
// approved matches this shape too. It is the closest honest predicate there is,
// which is the point.
func warnAboutApprovalsDiscoveryMayHaveReset(db *sql.DB) {
	count, err := store.CountUnauthenticatedSeenFederationPeers(db)
	if err != nil || count == 0 {
		return
	}
	log.Printf("fed: %d discovered peer(s) sit unauthenticated. Before #187 a LAN announcement reset an approved peer to pending every 30s, so a peer you did approve may be among them; approve it once more and it will hold. Approval is not repaired automatically because the reset erased what distinguished an approved peer from a new one.", count)
}

func parseLANPeerAnnouncement(data []byte, sender *net.UDPAddr) (lanPeerAnnouncement, bool) {
	var msg lanPeerAnnouncement
	if err := json.Unmarshal(data, &msg); err != nil {
		return lanPeerAnnouncement{}, false
	}
	address, ok := lanDialAddress(strings.TrimSpace(msg.Address), sender)
	if !ok {
		return lanPeerAnnouncement{}, false
	}
	msg.PeerID = strings.TrimSpace(msg.PeerID)
	msg.DisplayName = strings.TrimSpace(msg.DisplayName)
	msg.Address = address
	if msg.PeerID == "" {
		return lanPeerAnnouncement{}, false
	}
	return msg, true
}

// handleLANPeerAnnouncement files one announcement as a sighting. A sighting
// only ever learns where a peer is; it carries no status, so no amount of
// announcing can walk an operator's approval back (#187).
func handleLANPeerAnnouncement(db *sql.DB, data []byte, sender *net.UDPAddr, selfID string) {
	msg, ok := parseLANPeerAnnouncement(data, sender)
	if !ok || msg.PeerID == selfID {
		return
	}
	_ = store.RecordFederationPeerSighting(db, msg.PeerID, msg.DisplayName, msg.Address)
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
		handleLANPeerAnnouncement(db, buf[:n], sender, selfID)
	}
}
