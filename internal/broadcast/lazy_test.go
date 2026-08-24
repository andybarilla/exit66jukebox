package broadcast

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// A listener-gated hub must not pull from the queue while nobody is tuned in:
// next() is what makes the hub open a source, and opening a source is what
// spawns ffmpeg. Gating next() is therefore what makes the encoder lazy.
func TestRequireListenerHoldsNextUntilSomeoneTunesIn(t *testing.T) {
	var pulls atomic.Int64
	next := func() (string, bool) {
		pulls.Add(1)
		return "A", true
	}
	h := NewHub(fakeSource{data: map[string][]byte{"A": []byte("aaaa")}}, next, []byte("S"))
	h.RequireListener = true
	h.idlePace = time.Millisecond

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go h.Run(ctx)

	time.Sleep(30 * time.Millisecond)
	if n := pulls.Load(); n != 0 {
		t.Fatalf("pulled %d times with no listener; want 0", n)
	}

	ch, cancel := h.Listen()
	defer cancel()
	if got := collect(ch, 4, time.Second); len(got) == 0 {
		t.Fatal("no audio after a listener connected")
	}
	if n := pulls.Load(); n == 0 {
		t.Fatal("still not pulling with a listener connected")
	}
}

// The house stream is exempt: it advances whether or not anyone is listening.
func TestWithoutRequireListenerHubPullsImmediately(t *testing.T) {
	var pulls atomic.Int64
	next := func() (string, bool) {
		pulls.Add(1)
		return "", false
	}
	h := NewHub(fakeSource{data: map[string][]byte{}}, next, []byte("S"))
	h.idlePace = time.Millisecond

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go h.Run(ctx)

	deadline := time.After(time.Second)
	for pulls.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("house-style hub never pulled with no listener")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

// Close releases every listener so an HTTP handler blocked on the channel
// returns instead of hanging when its stream is deleted.
func TestCloseReleasesListeners(t *testing.T) {
	h := NewHub(fakeSource{data: map[string][]byte{}}, func() (string, bool) { return "", false }, nil)
	ch, cancel := h.Listen()
	defer cancel()

	h.Close()
	select {
	case _, open := <-ch:
		if open {
			t.Fatal("want a closed channel after Close")
		}
	case <-time.After(time.Second):
		t.Fatal("listener channel still open a second after Close")
	}
	if n := h.ListenerCount(); n != 0 {
		t.Fatalf("listeners after Close: want 0, got %d", n)
	}
}

// Close must be safe alongside the listener's own cancel func and a second
// Close — both happen when a stream is deleted while listeners disconnect.
func TestCloseIsIdempotentWithCancel(t *testing.T) {
	h := NewHub(fakeSource{data: map[string][]byte{}}, func() (string, bool) { return "", false }, nil)
	_, cancel := h.Listen()
	h.Close()
	cancel()
	h.Close()
}

// A closed hub must not hand out live listeners: a late /stream/x.mp3 request
// arriving after delete would otherwise block forever.
func TestListenAfterCloseReturnsClosedChannel(t *testing.T) {
	h := NewHub(fakeSource{data: map[string][]byte{}}, func() (string, bool) { return "", false }, nil)
	h.Close()
	ch, cancel := h.Listen()
	defer cancel()
	select {
	case _, open := <-ch:
		if open {
			t.Fatal("want closed channel from Listen after Close")
		}
	case <-time.After(time.Second):
		t.Fatal("Listen after Close handed out a live channel")
	}
}
