package main

import (
	"bytes"
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/jukebox"
	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

// fakeSource stands in for ffmpeg so the playback loop can be driven without
// spawning a process. Each track "plays" instantly, so a queue drains as fast
// as the loop can pop it.
type fakeSource struct{}

func (fakeSource) Open(string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader([]byte("mp3"))), nil
}

// heldSource keeps every track playing until the test releases it, so
// now-playing can be observed mid-track instead of racing the loop.
type heldSource struct{ release chan struct{} }

func newHeldSource() *heldSource { return &heldSource{release: make(chan struct{})} }

func (h *heldSource) Open(string) (io.ReadCloser, error) { return &heldReader{h.release}, nil }

type heldReader struct{ release <-chan struct{} }

func (r *heldReader) Read(p []byte) (int, error) {
	<-r.release
	return 0, io.EOF
}

func (r *heldReader) Close() error { return nil }

// recorder collects the listens a stream enqueues. The enqueue callback runs on
// the hub's goroutine, so reads from the test goroutine are guarded.
type recorder struct {
	mu  sync.Mutex
	ids []int64
}

func (r *recorder) add(id int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ids = append(r.ids, id)
}

func (r *recorder) snapshot() []int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]int64(nil), r.ids...)
}

func testBuilder(t *testing.T, enqueued *recorder) (*streamBuilder, *jukebox.Jukebox) {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	jb := jukebox.New(db, jukebox.Config{})
	return &streamBuilder{
		db: db, jb: jb, ctx: context.Background(),
		selfBaseURL:   "http://127.0.0.1:8066",
		signingSecret: []byte("secret"),
		src:           fakeSource{},
		enqueue: func(trackID, playedAt int64) error {
			enqueued.add(trackID)
			return nil
		},
	}, jb
}

// queueTrack adds a track long enough to clear the scrobble threshold if the
// stream were to settle it.
func queueTrack(t *testing.T, b *streamBuilder, streamID, title string) int64 {
	t.Helper()
	id, err := store.UpsertTrack(b.db,
		model.Track{Path: "/m/" + title + ".mp3", Title: title, Duration: 240}, "Band", "", "LP")
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := store.Enqueue(b.db, streamID, id, "tester"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	return id
}

// Criterion 10: only house produces scrobbles. The two streams below run the
// identical playback loop over an identical queue, with the offset clock wound
// forward past the scrobble threshold in both cases; only the house one
// enqueues anything.
func TestOnlyHouseStreamScrobbles(t *testing.T) {
	for _, tc := range []struct {
		name     string
		streamID string
		isHouse  bool
		want     int
	}{
		{"house settles both finished tracks", "house", true, 2},
		{"a side stream settles nothing", "kitchen", false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			enqueued := &recorder{}
			b, _ := testBuilder(t, enqueued)
			if err := store.EnsureSharedStream(b.db, tc.streamID, tc.streamID); err != nil {
				t.Fatalf("ensure: %v", err)
			}
			queueTrack(t, b, tc.streamID, "First")
			queueTrack(t, b, tc.streamID, "Second")

			p := b.build(tc.streamID, tc.isHouse)
			// A clock that jumps 300s per reading, so the offset between a
			// track starting and the next pop settling it clears the scrobble
			// threshold without the test waiting in real time.
			var ticks atomic.Int64
			base := time.Now()
			p.NP.SetClock(func() time.Time {
				return base.Add(time.Duration(ticks.Add(1)) * 300 * time.Second)
			})
			if !tc.isHouse {
				_, cancel := p.Hub.Listen() // a side stream needs a listener to advance
				defer cancel()
			}
			ctx, stop := context.WithCancel(context.Background())
			defer stop()
			go p.Hub.Run(ctx)

			waitFor(t, 2*time.Second, func() bool {
				n, _ := store.QueueLen(b.db, tc.streamID)
				return n == 0
			})
			time.Sleep(50 * time.Millisecond) // let the play->idle settle land

			if got := enqueued.snapshot(); len(got) != tc.want {
				t.Fatalf("%s: enqueued %v, want %d listen(s)", tc.streamID, got, tc.want)
			}
		})
	}
}

// House runs with no listener; every other shared stream waits for one.
func TestOnlyHouseAdvancesWithNoListener(t *testing.T) {
	enqueued := &recorder{}
	b, _ := testBuilder(t, enqueued)
	if err := store.CreateSharedStream(b.db, "kitchen", "Kitchen"); err != nil {
		t.Fatalf("create: %v", err)
	}
	queueTrack(t, b, "kitchen", "Side")

	p := b.build("kitchen", false)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go p.Hub.Run(ctx)

	time.Sleep(100 * time.Millisecond)
	if n, _ := store.QueueLen(b.db, "kitchen"); n != 1 {
		t.Fatalf("a listener-less stream drained its queue: %d left", n)
	}
}

// Two shared streams each pop their own queue: the unit-level half of
// criterion 8 (the end-to-end half is verified against a running binary).
func TestSharedStreamsPlayTheirOwnQueues(t *testing.T) {
	enqueued := &recorder{}
	b, _ := testBuilder(t, enqueued)
	for _, id := range []string{"kitchen", "patio"} {
		if err := store.CreateSharedStream(b.db, id, id); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	kitchenTrack := queueTrack(t, b, "kitchen", "KitchenSong")
	patioTrack := queueTrack(t, b, "patio", "PatioSong")

	// Hold both tracks mid-play so each stream's now-playing can be read while
	// the other one is also playing.
	held := newHeldSource()
	b.src = held
	defer close(held.release)

	want := map[string]int64{"kitchen": kitchenTrack, "patio": patioTrack}
	current := map[string]func() (model.Track, int, bool){}
	for _, id := range []string{"kitchen", "patio"} {
		p := b.build(id, false)
		_, cancel := p.Hub.Listen()
		defer cancel()
		ctx, stop := context.WithCancel(context.Background())
		defer stop()
		go p.Hub.Run(ctx)
		current[id] = p.NP.Current
	}

	for id, trackID := range want {
		np := current[id]
		waitFor(t, 2*time.Second, func() bool {
			tr, _, ok := np()
			return ok && tr.ID == trackID
		})
	}
	// Both are playing at the same moment, and each is playing its own track.
	kitchenNP, _, kOK := current["kitchen"]()
	patioNP, _, pOK := current["patio"]()
	if !kOK || !pOK {
		t.Fatalf("both streams should be playing: kitchen=%v patio=%v", kOK, pOK)
	}
	if kitchenNP.ID == patioNP.ID {
		t.Fatalf("both streams are playing the same track %d", kitchenNP.ID)
	}
	if kitchenNP.ID != kitchenTrack || patioNP.ID != patioTrack {
		t.Fatalf("wrong tracks: kitchen=%d (want %d) patio=%d (want %d)",
			kitchenNP.ID, kitchenTrack, patioNP.ID, patioTrack)
	}
}

func waitFor(t *testing.T, within time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	if !cond() {
		t.Fatal("condition never became true")
	}
}
