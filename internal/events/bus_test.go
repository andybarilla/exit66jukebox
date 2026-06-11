package events

import "testing"

func TestBusDeliversToSubscriber(t *testing.T) {
	b := NewBus()
	ch, cancel := b.Subscribe()
	defer cancel()

	b.Publish(Event{Type: "now-playing", Data: "song"})

	select {
	case e := <-ch:
		if e.Type != "now-playing" {
			t.Fatalf("want now-playing, got %q", e.Type)
		}
	default:
		t.Fatal("expected an event, got none")
	}
}

func TestBusCancelStopsDelivery(t *testing.T) {
	b := NewBus()
	ch, cancel := b.Subscribe()
	cancel()
	b.Publish(Event{Type: "x"})
	if _, open := <-ch; open {
		t.Fatal("channel should be closed after cancel")
	}
}

func TestBusDropsWhenSubscriberFull(t *testing.T) {
	b := NewBus()
	_, cancel := b.Subscribe()
	defer cancel()
	// Publishing many events without draining must not block/panic.
	for i := 0; i < 1000; i++ {
		b.Publish(Event{Type: "spam"})
	}
}
