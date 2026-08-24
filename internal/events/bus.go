package events

import "sync"

// Event is a single SSE message: a type and an arbitrary JSON-serializable body.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// Bus is a per-stream fan-out of events to SSE subscribers. Non-blocking:
// a subscriber that can't keep up drops events rather than stalling publishers.
type Bus struct {
	mu     sync.Mutex
	subs   map[chan Event]struct{}
	closed bool
}

func NewBus() *Bus {
	return &Bus{subs: make(map[chan Event]struct{})}
}

// Subscribe returns a buffered event channel and a cancel func that unsubscribes
// and closes the channel. Always call cancel (e.g. defer) when done.
func (b *Bus) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 16)
	b.mu.Lock()
	if b.closed {
		// The stream is gone; hand back a closed channel so the subscriber
		// unwinds instead of waiting on events that will never come.
		b.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	b.subs[ch] = struct{}{}
	b.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			_, live := b.subs[ch]
			delete(b.subs, ch)
			b.mu.Unlock()
			if live { // Close already closed it
				close(ch)
			}
		})
	}
	return ch, cancel
}

// SubscriberCount returns how many SSE subscribers are attached.
func (b *Bus) SubscriberCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}

// Close drops every subscriber and marks the bus dead, so SSE handlers return
// instead of hanging when their stream is deleted. Idempotent, and safe
// alongside each subscriber's own cancel func.
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for ch := range b.subs {
		delete(b.subs, ch)
		close(ch)
	}
}

// Publish delivers an event to all current subscribers, dropping it for any
// subscriber whose buffer is full.
func (b *Bus) Publish(e Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- e:
		default: // subscriber is behind; drop
		}
	}
}
