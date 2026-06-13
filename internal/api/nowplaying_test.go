package api

import (
	"testing"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/model"
)

func TestNowPlayingSetCurrentOffset(t *testing.T) {
	np := NewNowPlaying()
	now := time.Unix(1000, 0)
	np.now = func() time.Time { return now }

	if _, _, ok := np.Current(); ok {
		t.Fatal("fresh tracker should report nothing playing")
	}

	np.Set(model.Track{ID: 7, Title: "Hello"})
	now = now.Add(12 * time.Second)

	tr, offset, ok := np.Current()
	if !ok {
		t.Fatal("Current should report playing after Set")
	}
	if tr.ID != 7 {
		t.Fatalf("track id: want 7, got %d", tr.ID)
	}
	if offset != 12 {
		t.Fatalf("offset: want 12, got %d", offset)
	}
}

func TestNowPlayingClear(t *testing.T) {
	np := NewNowPlaying()
	np.Set(model.Track{ID: 1})
	np.Clear()
	if _, _, ok := np.Current(); ok {
		t.Fatal("Current should report nothing playing after Clear")
	}
}
