package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/broadcast"
	"github.com/andybarilla/exit66jukebox/internal/events"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

// silentSource stands in for ffmpeg: it never blocks and never spawns anything.
type silentSource struct{}

func (silentSource) Open(string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader([]byte("audio"))), nil
}

// withFactory attaches a pipeline factory that counts how many pipelines it has
// built, and shortens the idle timeouts so teardown is observable in a test.
func withFactory(t *testing.T, srv *Server, built *int) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv.SetStreamIdleTimeouts(30*time.Millisecond, 5*time.Millisecond)
	srv.SetStreamFactory(ctx, func(id string) *StreamPipeline {
		*built++
		hub := broadcast.NewHub(silentSource{}, func() (string, bool) { return "", false }, []byte("s"))
		hub.RequireListener = true
		return &StreamPipeline{Hub: hub, Bus: events.NewBus(), NP: NewNowPlaying()}
	})
}

// Criterion 9, first half: no pipeline exists for a shared stream nobody has
// tuned into, and connecting to its audio feed is what starts one.
func TestSharedStreamPipelineStartsOnFirstListener(t *testing.T) {
	srv, _ := newTestServer(t)
	built := 0
	withFactory(t, srv, &built)
	if err := store.CreateSharedStream(srv.db, "kitchen", "Kitchen"); err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, ok := srv.pipeline("kitchen"); ok {
		t.Fatal("a pipeline existed before anyone tuned in")
	}
	if built != 0 {
		t.Fatalf("factory ran %d times before any listener", built)
	}

	stop := tuneIn(t, srv, "kitchen")
	defer stop()
	if _, ok := srv.pipeline("kitchen"); !ok {
		t.Fatal("no pipeline after a listener connected")
	}
	if built != 1 {
		t.Fatalf("factory ran %d times, want 1", built)
	}
}

// Criterion 9, second half: the pipeline is torn down once nobody is tuned in.
func TestSharedStreamPipelineIsReapedWhenIdle(t *testing.T) {
	srv, _ := newTestServer(t)
	built := 0
	withFactory(t, srv, &built)
	if err := store.CreateSharedStream(srv.db, "kitchen", "Kitchen"); err != nil {
		t.Fatalf("create: %v", err)
	}
	stop := tuneIn(t, srv, "kitchen")
	if _, ok := srv.pipeline("kitchen"); !ok {
		t.Fatal("no pipeline after a listener connected")
	}
	stop()

	if !eventually(time.Second, func() bool {
		_, ok := srv.pipeline("kitchen")
		return !ok
	}) {
		t.Fatal("pipeline still running well past the idle timeout")
	}
}

// house is exempt: it is registered at boot and is never reaped, however long
// it sits with nobody listening.
func TestHousePipelineIsNeverReaped(t *testing.T) {
	srv, _ := newTestServer(t)
	built := 0
	withFactory(t, srv, &built)
	registerTestPipeline(t, srv, "house", NewNowPlaying())

	time.Sleep(100 * time.Millisecond) // several reaper ticks
	if _, ok := srv.pipeline("house"); !ok {
		t.Fatal("the house pipeline was torn down")
	}
	// Deleting it is refused too, so nothing else can stop it.
	srv.stopPipeline("house")
	if _, ok := srv.pipeline("house"); !ok {
		t.Fatal("stopPipeline tore down the persistent house pipeline")
	}
}

// Criterion 3: a listener on a deleted stream is told the stream closed and
// then released, rather than left hanging on a feed that will never resume.
func TestDeleteReleasesListenersWithAStreamClosedEvent(t *testing.T) {
	srv, _ := newTestServer(t)
	built := 0
	withFactory(t, srv, &built)
	if err := store.CreateSharedStream(srv.db, "party", "Party"); err != nil {
		t.Fatalf("create: %v", err)
	}
	p, ok := srv.ensurePipeline("party")
	if !ok {
		t.Fatal("no pipeline")
	}
	sse, cancelSSE := p.Bus.Subscribe()
	defer cancelSSE()
	audio, cancelAudio := p.Hub.Listen()
	defer cancelAudio()

	admin := adminSession(t, srv.db)
	if rec := do(srv, http.MethodDelete, "/api/streams/party", "", admin); rec.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}

	select {
	case e, open := <-sse:
		if !open {
			t.Fatal("SSE channel closed without a stream-closed event")
		}
		if e.Type != "stream-closed" {
			t.Fatalf("want stream-closed, got %q", e.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber never got a stream-closed event")
	}
	// Drain whatever silence was already buffered; the channel closing is what
	// releases the handler.
	deadline := time.After(time.Second)
	for {
		select {
		case _, open := <-audio:
			if !open {
				return
			}
		case <-deadline:
			t.Fatal("audio listener left hanging after delete")
		}
	}
}

// A deleted stream must not come back to life on the next request.
func TestDeletedStreamNoLongerServesAudio(t *testing.T) {
	srv, _ := newTestServer(t)
	built := 0
	withFactory(t, srv, &built)
	if err := store.CreateSharedStream(srv.db, "party", "Party"); err != nil {
		t.Fatalf("create: %v", err)
	}
	admin := adminSession(t, srv.db)
	if rec := do(srv, http.MethodDelete, "/api/streams/party", "", admin); rec.Code != http.StatusOK {
		t.Fatalf("delete: %d", rec.Code)
	}
	rec := do(srv, http.MethodGet, "/api/streams/party/events", "", admin)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("events on a deleted stream: want 404, got %d", rec.Code)
	}
}

// A private stream has no pipeline: it must not be lazily given one.
func TestPrivateStreamGetsNoPipeline(t *testing.T) {
	srv, _ := newTestServer(t)
	built := 0
	withFactory(t, srv, &built)
	if _, ok := srv.ensurePipeline("me"); ok {
		t.Fatal("a private stream was given a broadcast pipeline")
	}
	if built != 0 {
		t.Fatalf("factory ran %d times for a private stream", built)
	}
}

// tuneIn opens /stream/{id}.mp3 in the background and returns a func that
// disconnects the listener and waits for the handler to unwind.
func tuneIn(t *testing.T, srv *Server, id string) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/stream/"+id+".mp3", nil).WithContext(ctx)
	req.AddCookie(adminSession(t, srv.db))
	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.Handler().ServeHTTP(httptest.NewRecorder(), req)
	}()
	// Wait for the handler to have registered its listener before returning, so
	// the caller does not race the pipeline into existence.
	if !eventually(time.Second, func() bool { return srv.listenerCount(id) > 0 }) {
		cancel()
		t.Fatalf("no listener registered on %s", id)
	}
	return func() {
		cancel()
		<-done
	}
}

func eventually(within time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return cond()
}
