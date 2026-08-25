package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

const (
	PeerStatusPending     = "pending"
	PeerStatusAccepted    = "accepted"
	PeerStatusQuarantined = "quarantined"
)

type FederationPeer struct {
	ID                 int64  `json:"id"`
	PeerID             string `json:"peer_id"`
	DisplayName        string `json:"display_name"`
	Address            string `json:"address"`
	Status             string `json:"status"`
	Manual             bool   `json:"manual"`
	TokenAuthenticated bool   `json:"token_authenticated"`
	LastSeenAt         int64  `json:"last_seen_at"`
	CreatedAt          int64  `json:"created_at"`
	UpdatedAt          int64  `json:"updated_at"`
}

func SaveFederationPeer(db *sql.DB, peer FederationPeer) error {
	peer, err := normalizeFederationPeer(peer)
	if err != nil {
		return err
	}
	if hasDuplicateFederationPeerID(db, peer.PeerID, peer.Address) {
		peer.Status = PeerStatusQuarantined
		peer.TokenAuthenticated = false
	}
	_, err = db.Exec(
		`INSERT INTO federation_peer(peer_id, display_name, address, status, manual, token_authenticated, last_seen_at, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, strftime('%s','now'), strftime('%s','now'), strftime('%s','now'))
		 ON CONFLICT(peer_id, address) DO UPDATE SET
		   display_name=excluded.display_name, status=excluded.status, manual=excluded.manual,
		   token_authenticated=excluded.token_authenticated, last_seen_at=excluded.last_seen_at,
		   updated_at=strftime('%s','now')`,
		peer.PeerID, peer.DisplayName, peer.Address, peer.Status, boolToInt(peer.Manual), boolToInt(peer.TokenAuthenticated),
	)
	return err
}

// RecordFederationPeerSighting records that a peer announced itself, and
// nothing else. It writes address, display name and last-seen; on a row that
// already exists it may never touch status, manual or token_authenticated.
//
// Those three are the operator's, not discovery's: an announcement arrives
// every 30 seconds, and a trust decision that a ticker can revise is not a
// decision (#187). The rule is the absence of those columns from DO UPDATE SET
// rather than a branch in Go, so there is one statement, no read-then-write
// race, and no way for a later caller to write a status through this door.
// SaveFederationPeer keeps the old semantics for the admin path, which sets a
// status deliberately.
//
// A peer never seen before is still inserted as pending and unauthenticated —
// surfacing unknown peers for approval is what discovery is for.
func RecordFederationPeerSighting(db *sql.DB, peerID, displayName, address string) error {
	peer, err := normalizeFederationPeer(FederationPeer{PeerID: peerID, DisplayName: displayName, Address: address})
	if err != nil {
		return err
	}
	// Only reachable on the insert branch: an existing row's status is
	// protected below, so a duplicate found here cannot re-quarantine one.
	// It still decides how a newly-seen ambiguous address is filed.
	status := peer.Status
	if hasDuplicateFederationPeerID(db, peer.PeerID, peer.Address) {
		status = PeerStatusQuarantined
	}
	_, err = db.Exec(
		`INSERT INTO federation_peer(peer_id, display_name, address, status, manual, token_authenticated, last_seen_at, created_at, updated_at)
		 VALUES(?, ?, ?, ?, 0, 0, strftime('%s','now'), strftime('%s','now'), strftime('%s','now'))
		 ON CONFLICT(peer_id, address) DO UPDATE SET
		   display_name=CASE WHEN excluded.display_name <> '' THEN excluded.display_name ELSE federation_peer.display_name END,
		   last_seen_at=excluded.last_seen_at,
		   updated_at=strftime('%s','now')`,
		peer.PeerID, peer.DisplayName, peer.Address, status,
	)
	return err
}

func ListFederationPeers(db *sql.DB, status string) ([]FederationPeer, error) {
	status = strings.TrimSpace(status)
	query := `SELECT id, peer_id, display_name, address, status, manual, token_authenticated, last_seen_at, created_at, updated_at
		 FROM federation_peer`
	args := []any{}
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY peer_id, address`
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	peers := []FederationPeer{}
	for rows.Next() {
		peer, err := scanFederationPeer(rows)
		if err != nil {
			return nil, err
		}
		peers = append(peers, peer)
	}
	return peers, rows.Err()
}

func MarkFederationPeerAuthenticated(db *sql.DB, peerID string) error {
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return errors.New("peer id is required")
	}
	res, err := db.Exec(
		`UPDATE federation_peer SET token_authenticated = 1, last_seen_at = strftime('%s','now'), updated_at = strftime('%s','now')
		 WHERE peer_id = ? AND status != ?`, peerID, PeerStatusQuarantined,
	)
	if err != nil {
		return err
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return fmt.Errorf("peer %s not found", peerID)
	}
	return nil
}

func ApproveFederationPeer(db *sql.DB, peerID string) error {
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return errors.New("peer id is required")
	}
	res, err := db.Exec(
		`UPDATE federation_peer SET status = ?, updated_at = strftime('%s','now')
		 WHERE peer_id = ? AND status = ? AND token_authenticated != 0`,
		PeerStatusAccepted, peerID, PeerStatusPending,
	)
	if err != nil {
		return err
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return fmt.Errorf("peer %s is not authenticated and pending approval", peerID)
	}
	return nil
}

func AcceptedFederationPeerAddresses(db *sql.DB) (map[string]string, error) {
	peers, err := ListFederationPeers(db, PeerStatusAccepted)
	if err != nil {
		return nil, err
	}
	addresses := make(map[string]string, len(peers))
	for _, peer := range peers {
		addresses[peer.PeerID] = peer.Address
	}
	return addresses, nil
}

func normalizeFederationPeer(peer FederationPeer) (FederationPeer, error) {
	peer.PeerID = strings.TrimSpace(peer.PeerID)
	peer.DisplayName = strings.TrimSpace(peer.DisplayName)
	peer.Address = strings.TrimSpace(peer.Address)
	peer.Status = strings.TrimSpace(peer.Status)
	if peer.PeerID == "" {
		return FederationPeer{}, errors.New("peer id is required")
	}
	if peer.Address == "" {
		return FederationPeer{}, errors.New("peer address is required")
	}
	if peer.Status == "" {
		peer.Status = PeerStatusPending
	}
	if peer.Status != PeerStatusPending && peer.Status != PeerStatusAccepted && peer.Status != PeerStatusQuarantined {
		return FederationPeer{}, fmt.Errorf("unsupported peer status: %s", peer.Status)
	}
	return peer, nil
}

func hasDuplicateFederationPeerID(db *sql.DB, peerID, address string) bool {
	var existing string
	err := db.QueryRow(`SELECT address FROM federation_peer WHERE peer_id = ? AND address <> ? LIMIT 1`, peerID, address).Scan(&existing)
	return err == nil
}

type federationPeerScanner interface {
	Scan(dest ...any) error
}

func scanFederationPeer(row federationPeerScanner) (FederationPeer, error) {
	var peer FederationPeer
	var manual, tokenAuthenticated int
	err := row.Scan(&peer.ID, &peer.PeerID, &peer.DisplayName, &peer.Address, &peer.Status,
		&manual, &tokenAuthenticated, &peer.LastSeenAt, &peer.CreatedAt, &peer.UpdatedAt)
	peer.Manual = manual != 0
	peer.TokenAuthenticated = tokenAuthenticated != 0
	return peer, err
}
