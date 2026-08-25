package store

import (
	"database/sql"
	"testing"
)

func groupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func acceptPeer(t *testing.T, db *sql.DB, peerID string) {
	t.Helper()
	if err := SaveFederationPeer(db, FederationPeer{
		PeerID: peerID, Address: peerID + ":9000", Status: PeerStatusAccepted, TokenAuthenticated: true,
	}); err != nil {
		t.Fatalf("save peer %s: %v", peerID, err)
	}
}

func mustGroup(t *testing.T, db *sql.DB, name string, members ...string) FederationGroup {
	t.Helper()
	g, err := CreateFederationGroup(db, name)
	if err != nil {
		t.Fatalf("create group %s: %v", name, err)
	}
	for _, m := range members {
		if err := AddFederationGroupMember(db, g.ID, m); err != nil {
			t.Fatalf("add %s to %s: %v", m, name, err)
		}
	}
	return g
}

func mustVisible(t *testing.T, db *sql.DB, owner, viewer string) bool {
	t.Helper()
	ok, err := FederationCatalogVisible(db, owner, viewer)
	if err != nil {
		t.Fatalf("visible(%s,%s): %v", owner, viewer, err)
	}
	return ok
}

// With no group created the feature is dormant: every peer sees every other,
// which is what an install that never creates a group must keep doing.
func TestFederationCatalogVisibleWithNoGroups(t *testing.T) {
	db := groupTestDB(t)
	if !mustVisible(t, db, "home", "stranger") {
		t.Fatal("no groups exist, so catalog must be visible")
	}
}

func TestFederationCatalogVisibleOnlyWithinAGroup(t *testing.T) {
	db := groupTestDB(t)
	mustGroup(t, db, "family", "home", "office")

	if !mustVisible(t, db, "home", "office") {
		t.Fatal("home and office share family, want visible")
	}
	if !mustVisible(t, db, "office", "home") {
		t.Fatal("visibility must hold in both directions")
	}
	if mustVisible(t, db, "home", "stranger") {
		t.Fatal("stranger shares no group with home, want hidden")
	}
	if mustVisible(t, db, "stranger", "other") {
		t.Fatal("two ungrouped peers share nothing, want hidden")
	}
}

func TestFederationCatalogVisibleAcrossTwoGroups(t *testing.T) {
	db := groupTestDB(t)
	mustGroup(t, db, "family", "home", "office")
	mustGroup(t, db, "friends", "home", "dave")

	for _, peer := range []string{"office", "dave"} {
		if !mustVisible(t, db, "home", peer) {
			t.Fatalf("home shares a group with %s, want visible", peer)
		}
	}
	if mustVisible(t, db, "office", "dave") {
		t.Fatal("office and dave share no group, want hidden")
	}
}

func TestRemoveFederationGroupMemberHidesTheCatalog(t *testing.T) {
	db := groupTestDB(t)
	g := mustGroup(t, db, "family", "home", "office")

	if err := RemoveFederationGroupMember(db, g.ID, "office"); err != nil {
		t.Fatalf("remove member: %v", err)
	}
	if mustVisible(t, db, "home", "office") {
		t.Fatal("office was removed from the only shared group, want hidden")
	}
}

func TestDeleteFederationGroupCascadesMembers(t *testing.T) {
	db := groupTestDB(t)
	g := mustGroup(t, db, "family", "home", "office")

	if err := DeleteFederationGroup(db, g.ID); err != nil {
		t.Fatalf("delete group: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM federation_group_member WHERE group_id = ?`, g.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d member rows survived the group delete, want 0", n)
	}
	// Back to dormant: the last group is gone, so discovery is unscoped again.
	if !mustVisible(t, db, "home", "office") {
		t.Fatal("no groups remain, want visible")
	}
}

func TestListFederationGroupsReturnsMembers(t *testing.T) {
	db := groupTestDB(t)
	mustGroup(t, db, "family", "office", "home")
	mustGroup(t, db, "friends", "dave")

	groups, err := ListFederationGroups(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || groups[0].Name != "family" || groups[1].Name != "friends" {
		t.Fatalf("groups = %+v, want family then friends", groups)
	}
	if len(groups[0].Members) != 2 || groups[0].Members[0] != "home" || groups[0].Members[1] != "office" {
		t.Fatalf("family members = %v, want [home office]", groups[0].Members)
	}
	if len(groups[1].Members) != 1 || groups[1].Members[0] != "dave" {
		t.Fatalf("friends members = %v, want [dave]", groups[1].Members)
	}
}

// The upgrade path: an install that already had accepted peers gets one group
// holding this instance and all of them, so nothing it could see before the
// upgrade becomes invisible after it.
func TestSeedDefaultFederationGroupMigratesAcceptedPeers(t *testing.T) {
	db := groupTestDB(t)
	acceptPeer(t, db, "office")
	acceptPeer(t, db, "dave")
	if err := SaveFederationPeer(db, FederationPeer{PeerID: "pending", Address: "p:9000", Status: PeerStatusPending}); err != nil {
		t.Fatal(err)
	}

	if err := SeedDefaultFederationGroup(db, "home"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	groups, err := ListFederationGroups(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].Name != DefaultFederationGroupName {
		t.Fatalf("groups = %+v, want one %q", groups, DefaultFederationGroupName)
	}
	for _, peer := range []string{"office", "dave"} {
		if !mustVisible(t, db, "home", peer) {
			t.Fatalf("accepted peer %s must stay visible after the upgrade", peer)
		}
	}
	if mustVisible(t, db, "home", "pending") {
		t.Fatal("a peer that was only pending must not be seeded into the default group")
	}
}

// migrate-style seeding runs on every start, so a group the admin deleted must
// not come back.
func TestSeedDefaultFederationGroupRunsOnce(t *testing.T) {
	db := groupTestDB(t)
	acceptPeer(t, db, "office")
	if err := SeedDefaultFederationGroup(db, "home"); err != nil {
		t.Fatal(err)
	}
	groups, err := ListFederationGroups(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := DeleteFederationGroup(db, groups[0].ID); err != nil {
		t.Fatal(err)
	}

	if err := SeedDefaultFederationGroup(db, "home"); err != nil {
		t.Fatal(err)
	}

	groups, err = ListFederationGroups(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("groups = %+v, want the deleted group to stay deleted", groups)
	}
}

// A fresh install has no accepted peers, so there is nothing to migrate and the
// feature stays dormant even after peers are approved later.
func TestSeedDefaultFederationGroupSkipsFreshInstall(t *testing.T) {
	db := groupTestDB(t)
	if err := SeedDefaultFederationGroup(db, "home"); err != nil {
		t.Fatal(err)
	}
	acceptPeer(t, db, "office")
	if err := SeedDefaultFederationGroup(db, "home"); err != nil {
		t.Fatal(err)
	}

	groups, err := ListFederationGroups(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("groups = %+v, want none on a fresh install", groups)
	}
	if !mustVisible(t, db, "home", "office") {
		t.Fatal("dormant install must keep catalogs visible")
	}
}

// Membership is a stored row, so it survives a restart.
func TestFederationGroupMembershipSurvivesReopen(t *testing.T) {
	path := t.TempDir() + "/groups.db"
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	mustGroup(t, db, "family", "home", "office")
	db.Close()

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if !mustVisible(t, reopened, "home", "office") {
		t.Fatal("membership did not survive the restart")
	}
	if mustVisible(t, reopened, "home", "stranger") {
		t.Fatal("groups were not active after the restart")
	}
}
