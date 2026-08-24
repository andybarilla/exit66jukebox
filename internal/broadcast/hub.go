package broadcast

import (
	"context"
	"io"
	"log"
	"sync"
	"time"
)

// Source opens a real-time-paced MP3 byte stream for a track path.
type Source interface {
	Open(path string) (io.ReadCloser, error)
}

// Hub fans one shared MP3 feed out to many HTTP listeners. It pulls tracks via
// next(); when the queue is empty it emits silence so listeners stay connected.
type Hub struct {
	src      Source
	next     func() (path string, ok bool)
	silence  []byte
	idlePace time.Duration

	// RequireListener holds the loop off next() while nobody is tuned in, so a
	// stream spawns no encoder until someone listens. The always-on house stream
	// leaves it false and advances regardless.
	RequireListener bool

	mu        sync.Mutex
	listeners map[chan []byte]struct{}
	closed    bool
}

func NewHub(src Source, next func() (string, bool), silence []byte) *Hub {
	return &Hub{
		src:       src,
		next:      next,
		silence:   silence,
		idlePace:  time.Second,
		listeners: make(map[chan []byte]struct{}),
	}
}

// Listen registers a listener, returning its byte channel and an unsubscribe
// func. The channel is buffered; a listener that falls behind drops chunks.
func (h *Hub) Listen() (<-chan []byte, func()) {
	ch := make(chan []byte, 64)
	h.mu.Lock()
	if h.closed {
		// The stream is gone; hand back an already-closed channel so the caller
		// unwinds instead of blocking on a feed that will never arrive.
		h.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	h.listeners[ch] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			_, live := h.listeners[ch]
			delete(h.listeners, ch)
			h.mu.Unlock()
			if live { // Close already closed it
				close(ch)
			}
		})
	}
	return ch, cancel
}

// Close drops every listener and marks the hub dead, so handlers blocked on a
// listener channel return rather than hanging when the stream is deleted.
// Idempotent, and safe alongside each listener's own cancel func.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for ch := range h.listeners {
		delete(h.listeners, ch)
		close(ch)
	}
}

func (h *Hub) broadcast(b []byte) {
	chunk := make([]byte, len(b)) // copy: caller reuses its read buffer
	copy(chunk, b)
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.listeners {
		select {
		case ch <- chunk:
		default: // listener behind; drop
		}
	}
}

// Run is the broadcast loop. It blocks until ctx is cancelled.
func (h *Hub) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		if h.RequireListener && h.ListenerCount() == 0 {
			h.waitIdle(ctx)
			continue
		}
		path, ok := h.next()
		if !ok {
			h.idle(ctx)
			continue
		}
		h.play(ctx, path)
	}
}

func (h *Hub) play(ctx context.Context, path string) {
	rc, err := h.src.Open(path)
	if err != nil {
		log.Printf("broadcast: open %q: %v", path, err)
		return
	}
	defer rc.Close()
	buf := make([]byte, 32*1024)
	for {
		if ctx.Err() != nil {
			return
		}
		n, err := rc.Read(buf)
		if n > 0 {
			h.broadcast(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// ListenerCount returns the number of connected listeners.
func (h *Hub) ListenerCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.listeners)
}

func (h *Hub) idle(ctx context.Context) {
	if len(h.silence) > 0 {
		h.broadcast(h.silence)
	}
	h.waitIdle(ctx)
}

// waitIdle paces the loop without emitting anything. Used while a
// listener-gated stream has nobody tuned in: there is no one to send silence to.
func (h *Hub) waitIdle(ctx context.Context) {
	select {
	case <-ctx.Done():
	case <-time.After(h.idlePace):
	}
}
