package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// keyFederationGroupsSeeded records that SeedDefaultFederationGroup has already
// run. migrate() and federation startup both run on every process start, so
// without a one-shot marker an admin who deletes the seeded group gets it back
// on the next restart.
const keyFederationGroupsSeeded = "federation_groups_seeded"

// DefaultFederationGroupName is the group existing accepted peers are seeded
// into on upgrade.
const DefaultFederationGroupName = "Default"

// FederationGroup is a named listening group: a set of peer ids (this instance's
// own id included when it participates) that may discover each other's
// catalogs. Groups scope discovery only — audio fetching is not gated on
// membership, by the deliberate design in #88.
type FederationGroup struct {
	ID      int64    `json:"id"`
	Name    string   `json:"name"`
	Members []string `json:"members"`
}

func CreateFederationGroup(db *sql.DB, name string) (FederationGroup, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return FederationGroup{}, errors.New("group name is required")
	}
	res, err := db.Exec(`INSERT INTO federation_group(name, created_at) VALUES(?, strftime('%s','now'))`, name)
	if err != nil {
		return FederationGroup{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return FederationGroup{}, err
	}
	return FederationGroup{ID: id, Name: name, Members: []string{}}, nil
}

func DeleteFederationGroup(db *sql.DB, id int64) error {
	// The member rows go with it via ON DELETE CASCADE; Open enables
	// foreign_keys on every pooled connection.
	res, err := db.Exec(`DELETE FROM federation_group WHERE id = ?`, id)
	if err != nil {
		return err
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return fmt.Errorf("group %d not found", id)
	}
	return nil
}

// ListFederationGroups returns every group with its members, ordered by name.
func ListFederationGroups(db *sql.DB) ([]FederationGroup, error) {
	rows, err := db.Query(`SELECT id, name FROM federation_group ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := []FederationGroup{}
	byID := map[int64]int{}
	for rows.Next() {
		var g FederationGroup
		if err := rows.Scan(&g.ID, &g.Name); err != nil {
			return nil, err
		}
		g.Members = []string{}
		byID[g.ID] = len(groups)
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	members, err := db.Query(`SELECT group_id, peer_id FROM federation_group_member ORDER BY peer_id`)
	if err != nil {
		return nil, err
	}
	defer members.Close()
	for members.Next() {
		var groupID int64
		var peerID string
		if err := members.Scan(&groupID, &peerID); err != nil {
			return nil, err
		}
		if i, ok := byID[groupID]; ok {
			groups[i].Members = append(groups[i].Members, peerID)
		}
	}
	return groups, members.Err()
}

// normalizeGroupMemberPeerID trims a hand-typed peer id and refuses one that
// could never match at the check.
//
// Membership is compared against the id a peer claims at the token handshake,
// which is read with strings.Fields on a single space-delimited line — so an id
// containing whitespace cannot arrive over the wire at all. Storing one would
// seed a member row that silently never matches, and the operator would see a
// correctly-populated group moving no catalogs. Refuse it at the door instead.
//
// Case is NOT folded: the handshake compares ids byte for byte, so folding here
// would make a member row match nothing just as surely.
func normalizeGroupMemberPeerID(peerID string) (string, error) {
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return "", errors.New("peer id is required")
	}
	if strings.ContainsFunc(peerID, unicode.IsSpace) {
		return "", fmt.Errorf("peer id %q contains whitespace, which a peer cannot claim at the handshake", peerID)
	}
	return peerID, nil
}

func AddFederationGroupMember(db *sql.DB, groupID int64, peerID string) error {
	peerID, err := normalizeGroupMemberPeerID(peerID)
	if err != nil {
		return err
	}
	_, err = db.Exec(
		`INSERT INTO federation_group_member(group_id, peer_id) VALUES(?, ?)
		 ON CONFLICT(group_id, peer_id) DO NOTHING`, groupID, peerID)
	return err
}

func RemoveFederationGroupMember(db *sql.DB, groupID int64, peerID string) error {
	// Trim but do not validate: a row stored before this check existed must
	// still be removable.
	_, err := db.Exec(`DELETE FROM federation_group_member WHERE group_id = ? AND peer_id = ?`, groupID, strings.TrimSpace(peerID))
	return err
}

// FederationGroupsExist reports whether any group has been created. No groups
// means the feature is dormant and catalog sync is unscoped, which is how an
// install that never creates a group keeps behaving exactly as it did before
// groups existed.
func FederationGroupsExist(db *sql.DB) (bool, error) {
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM federation_group`).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// FederationCatalogVisible reports whether the peer viewer may discover owner's
// catalog. It is the whole group policy in one place:
//
//   - no groups at all: visible, the dormant pre-groups behaviour
//   - otherwise: visible only when both ids are members of some one group
//
// Both arguments are peer ids as they appear on the wire. Per #167 a peer id is
// claimed rather than proved, so this scopes what a peer *discovers*, not what
// it is authorized to fetch.
func FederationCatalogVisible(db *sql.DB, owner, viewer string) (bool, error) {
	active, err := FederationGroupsExist(db)
	if err != nil {
		return false, err
	}
	if !active {
		return true, nil
	}
	owner, viewer = strings.TrimSpace(owner), strings.TrimSpace(viewer)
	if owner == "" || viewer == "" {
		return false, nil
	}
	var n int
	err = db.QueryRow(
		`SELECT count(*) FROM federation_group_member a
		 JOIN federation_group_member b ON b.group_id = a.group_id
		 WHERE a.peer_id = ? AND b.peer_id = ?`, owner, viewer).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// SeedDefaultFederationGroup migrates an install that federated before groups
// existed: every already-accepted peer, plus this instance, joins one group
// named DefaultFederationGroupName, so the peers that could see each other
// before upgrade still can. It runs at most once (keyFederationGroupsSeeded).
//
// An install with no accepted peers seeds nothing and is marked done, so peers
// approved later leave it dormant rather than silently switching groups on.
func SeedDefaultFederationGroup(db *sql.DB, selfPeerID string) error {
	selfPeerID = strings.TrimSpace(selfPeerID)
	if selfPeerID == "" {
		// Without this instance's own id the seeded group could not contain it,
		// and every peer in it would be invisible. Leave the install dormant.
		return nil
	}
	if metaFlag(db, keyFederationGroupsSeeded) {
		return nil
	}
	exist, err := FederationGroupsExist(db)
	if err != nil {
		return err
	}
	peers, err := ListFederationPeers(db, PeerStatusAccepted)
	if err != nil {
		return err
	}
	if exist || len(peers) == 0 {
		return setMetaFlag(db, keyFederationGroupsSeeded, true)
	}
	group, err := CreateFederationGroup(db, DefaultFederationGroupName)
	if err != nil {
		return err
	}
	if err := AddFederationGroupMember(db, group.ID, selfPeerID); err != nil {
		return err
	}
	for _, p := range peers {
		if err := AddFederationGroupMember(db, group.ID, p.PeerID); err != nil {
			return err
		}
	}
	return setMetaFlag(db, keyFederationGroupsSeeded, true)
}
