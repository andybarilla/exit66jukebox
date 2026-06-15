package store

import "testing"

func TestUpsertRemoteTrackAndResolve(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id, err := UpsertRemoteTrack(db, RemoteTrack{
		SourcePeer: "home", RemoteID: 42,
		Title: "Song", ArtistName: "Artist", AlbumArtist: "Artist", AlbumName: "Album",
		TrackNo: 1, Duration: 180,
	})
	if err != nil {
		t.Fatal(err)
	}
	tr, path, ok := GetTrack(db, id)
	if !ok {
		t.Fatal("track not found")
	}
	// Remote rows get a synthetic non-empty path (never opened — audio resolution
	// branches on source_peer first); the source fields are what callers act on.
	if path != "fed://home/42" {
		t.Fatalf("expected synthetic remote path, got %q", path)
	}
	if tr.SourcePeer != "home" || tr.RemoteID != 42 {
		t.Fatalf("source fields not returned: %+v", tr)
	}

	// Re-upsert same (peer, remote_id) updates rather than duplicating.
	id2, err := UpsertRemoteTrack(db, RemoteTrack{
		SourcePeer: "home", RemoteID: 42, Title: "Song v2",
		ArtistName: "Artist", AlbumArtist: "Artist", AlbumName: "Album",
	})
	if err != nil {
		t.Fatal(err)
	}
	if id2 != id {
		t.Fatalf("expected same row id on re-upsert, got %d != %d", id2, id)
	}
}
