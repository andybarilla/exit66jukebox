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
	if _, err := db.Exec(`INSERT INTO artist(name, sort_key) VALUES('A','a')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO album(name, artist_id, sort_key) VALUES('Al', 1, 'al')`); err != nil {
		t.Fatal(err)
	}
	ins := `INSERT INTO track(path, mod_time, size, source_peer, remote_id, title, artist_id, album_id, added_at)
	        VALUES(?, 0, 0, 'home', 5, 't', 1, 1, 0)`
	if _, err := db.Exec(ins, "fed://home/5#a"); err != nil {
		t.Fatalf("first remote insert failed: %v", err)
	}
	if _, err := db.Exec(ins, "fed://home/5#b"); err == nil {
		t.Fatal("expected unique-index violation on duplicate (source_peer, remote_id)")
	}
}
