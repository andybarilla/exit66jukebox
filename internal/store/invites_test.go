package store

import "testing"

func TestInviteCreateConsume(t *testing.T) {
	db := mustOpenMem(t)
	admin, _ := CreateUser(db, "a@b.com", "A", "h", true, true)
	id, err := CreateInvite(db, "thash", "new@b.com", true, admin, 4_000_000_000)
	if err != nil {
		t.Fatal(err)
	}
	inv, ok, err := PendingInvite(db, "thash")
	if err != nil || !ok || inv.ID != id || !inv.IsAdmin || inv.Email != "new@b.com" {
		t.Fatalf("pending: %+v ok=%v err=%v", inv, ok, err)
	}
	if err := MarkInviteAccepted(db, id); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := PendingInvite(db, "thash"); ok {
		t.Fatal("accepted invite still pending (not single-use)")
	}
}

func TestExpiredInviteNotPending(t *testing.T) {
	db := mustOpenMem(t)
	admin, _ := CreateUser(db, "a@b.com", "A", "h", true, true)
	CreateInvite(db, "old", "", false, admin, 100)
	if _, ok, _ := PendingInvite(db, "old"); ok {
		t.Fatal("expired invite pending")
	}
}

func TestListRevokeInvite(t *testing.T) {
	db := mustOpenMem(t)
	admin, _ := CreateUser(db, "a@b.com", "A", "h", true, true)
	id, _ := CreateInvite(db, "t", "x@y.com", false, admin, 4_000_000_000)
	if invs, _ := ListInvites(db); len(invs) != 1 {
		t.Fatalf("want 1 invite, got %d", len(invs))
	}
	if err := DeleteInvite(db, id); err != nil {
		t.Fatal(err)
	}
	if invs, _ := ListInvites(db); len(invs) != 0 {
		t.Fatalf("want 0 after revoke, got %d", len(invs))
	}
}
