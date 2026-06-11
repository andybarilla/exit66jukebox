package broadcast

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"
)

// fakeSource serves canned bytes per "path".
type fakeSource struct{ data map[string][]byte }

func (f fakeSource) Open(path string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(f.data[path])), nil
}

// collect reads up to `want` bytes from a listener channel within a deadline.
func collect(ch <-chan []byte, want int, deadline time.Duration) []byte {
	var out []byte
	timer := time.After(deadline)
	for len(out) < want {
		select {
		case b, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, b...)
		case <-timer:
			return out
		}
	}
	return out
}

func TestHubFansOutQueuedTracksInOrder(t *testing.T) {
	src := fakeSource{data: map[string][]byte{
		"A": []byte("aaaa"),
		"B": []byte("bbbb"),
	}}
	queue := []string{"A", "B"}
	next := func() (string, bool) {
		if len(queue) == 0 {
			return "", false
		}
		p := queue[0]
		queue = queue[1:]
		return p, true
	}

	h := NewHub(src, next, []byte("S"))
	h.idlePace = 5 * time.Millisecond

	ch, cancel := h.Listen()
	defer cancel()

	ctx, stop := context.WithCancel(context.Background())
	go h.Run(ctx)
	defer stop()

	got := collect(ch, 8, time.Second)
	if !bytes.Contains(got, []byte("aaaa")) || !bytes.Contains(got, []byte("bbbb")) {
		t.Fatalf("expected both tracks' bytes, got %q", got)
	}
	if bytes.Index(got, []byte("aaaa")) > bytes.Index(got, []byte("bbbb")) {
		t.Fatalf("expected A before B, got %q", got)
	}
}

func TestHubStreamsSilenceWhenEmpty(t *testing.T) {
	src := fakeSource{data: map[string][]byte{}}
	next := func() (string, bool) { return "", false }

	h := NewHub(src, next, []byte("S"))
	h.idlePace = time.Millisecond

	ch, cancel := h.Listen()
	defer cancel()
	ctx, stop := context.WithCancel(context.Background())
	go h.Run(ctx)
	defer stop()

	got := collect(ch, 3, time.Second)
	if !bytes.Contains(got, []byte("S")) {
		t.Fatalf("expected silence bytes when empty, got %q", got)
	}
}

func TestHubRunReturnsOnCancel(t *testing.T) {
	src := fakeSource{data: map[string][]byte{}}
	next := func() (string, bool) { return "", false }
	h := NewHub(src, next, nil) // nil silence: idle just waits
	h.idlePace = time.Millisecond

	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { h.Run(ctx); close(done) }()
	stop()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return promptly after ctx cancel")
	}
}

func TestListenerCount(t *testing.T) {
	h := NewHub(nil, func() (string, bool) { return "", false }, nil)
	if h.ListenerCount() != 0 {
		t.Fatalf("want 0, got %d", h.ListenerCount())
	}
	_, c1 := h.Listen()
	_, c2 := h.Listen()
	if h.ListenerCount() != 2 {
		t.Fatalf("want 2, got %d", h.ListenerCount())
	}
	c1()
	if h.ListenerCount() != 1 {
		t.Fatalf("want 1, got %d", h.ListenerCount())
	}
	c2()
	if h.ListenerCount() != 0 {
		t.Fatalf("want 0, got %d", h.ListenerCount())
	}
}

func TestHubFansOutToMultipleListeners(t *testing.T) {
	src := fakeSource{data: map[string][]byte{"A": []byte("aaaa")}}
	queue := []string{"A"}
	next := func() (string, bool) {
		if len(queue) == 0 {
			return "", false
		}
		p := queue[0]
		queue = queue[1:]
		return p, true
	}
	h := NewHub(src, next, []byte("S"))
	h.idlePace = 5 * time.Millisecond

	ch1, c1 := h.Listen()
	defer c1()
	ch2, c2 := h.Listen()
	defer c2()

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go h.Run(ctx)

	g1 := collect(ch1, 4, time.Second)
	g2 := collect(ch2, 4, time.Second)
	if !bytes.Contains(g1, []byte("aaaa")) || !bytes.Contains(g2, []byte("aaaa")) {
		t.Fatalf("both listeners should receive track A: g1=%q g2=%q", g1, g2)
	}
}
