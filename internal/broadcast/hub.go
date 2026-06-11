package broadcast

import (
	"context"
	"io"
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

	mu        sync.Mutex
	listeners map[chan []byte]struct{}
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
	h.listeners[ch] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.listeners, ch)
			h.mu.Unlock()
			close(ch)
		})
	}
	return ch, cancel
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

func (h *Hub) idle(ctx context.Context) {
	if len(h.silence) > 0 {
		h.broadcast(h.silence)
	}
	select {
	case <-ctx.Done():
	case <-time.After(h.idlePace):
	}
}
