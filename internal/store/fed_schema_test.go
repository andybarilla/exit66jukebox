package store

import "testing"

func TestRemoteColumnsExist(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, col := range []string{"source_peer", "remote_id"} {
		has, err := columnExists(db, "track", col)
		if err != nil {
			t.Fatal(err)
		}
		if !has {
			t.Fatalf("track.%s missing", col)
		}
	}
}

func TestRemoteUniqueIndexRejectsDuplicate(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO artist(name, sort_key) VALUES('A','a')`)
	if err != nil {
		t.Fatal(err)
	}
	ins := `INSERT INTO track(path, source_peer, remote_id, title, artist_id, album_id, added_at)
	        VALUES(?, 'home', 5, 't', 1, 0, 0)`
	if _, err := db.Exec(ins, ""); err != nil {
		t.Fatalf("first remote insert failed: %v", err)
	}
	if _, err := db.Exec(ins, ""); err == nil {
		t.Fatal("expected unique-index violation on duplicate (source_peer, remote_id)")
	}
}
