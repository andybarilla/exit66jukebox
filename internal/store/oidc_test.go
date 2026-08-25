package store

import "testing"

func TestOIDCIdentityLinkAndLookup(t *testing.T) {
	db := mustOpenMem(t)
	uid, _ := CreateUser(db, "a@b.com", "A", "h", false, true)
	if err := LinkOIDCIdentity(db, "https://idp.example", "sub-1", uid); err != nil {
		t.Fatal(err)
	}
	got, ok, err := UserByOIDCIdentity(db, "https://idp.example", "sub-1")
	if err != nil || !ok || got.ID != uid {
		t.Fatalf("lookup: ok=%v err=%v id=%d, want id=%d", ok, err, got.ID, uid)
	}
	if _, ok, _ := UserByOIDCIdentity(db, "https://other.example", "sub-1"); ok {
		t.Fatal("subject matched under a different issuer")
	}
	if _, ok, _ := UserByOIDCIdentity(db, "https://idp.example", "sub-2"); ok {
		t.Fatal("unlinked subject resolved to a user")
	}
}

// A second link for the same issuer+subject must fail rather than silently move
// the identity to another account.
func TestOIDCIdentityRelinkRefused(t *testing.T) {
	db := mustOpenMem(t)
	first, _ := CreateUser(db, "a@b.com", "A", "h", false, true)
	second, _ := CreateUser(db, "c@d.com", "C", "h", false, true)
	if err := LinkOIDCIdentity(db, "https://idp.example", "sub-1", first); err != nil {
		t.Fatal(err)
	}
	if err := LinkOIDCIdentity(db, "https://idp.example", "sub-1", second); err == nil {
		t.Fatal("relinking an existing identity was accepted")
	}
	got, _, _ := UserByOIDCIdentity(db, "https://idp.example", "sub-1")
	if got.ID != first {
		t.Fatalf("identity moved to user %d, want %d", got.ID, first)
	}
}

// The row is a child of the account: deleting the user must not strand it.
func TestOIDCIdentityDeletedWithUser(t *testing.T) {
	db := mustOpenMem(t)
	uid, _ := CreateUser(db, "a@b.com", "A", "h", false, true)
	if err := LinkOIDCIdentity(db, "https://idp.example", "sub-1", uid); err != nil {
		t.Fatal(err)
	}
	if err := DeleteUser(db, uid); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM oidc_identity`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("oidc_identity rows after user delete = %d, want 0", n)
	}
}
