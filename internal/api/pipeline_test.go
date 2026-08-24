package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
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
	srv.setStreamIdleTimeouts(30*time.Millisecond, 5*time.Millisecond)
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

// L1: a lazily-started pipeline's hub goroutine owns an ffmpeg child, which is
// only killed when Run unwinds. If main exits without waiting for it, those
// children are orphaned — invisible with one stream, and exactly what happens
// once there are several.
func TestWaitForPipelinesBlocksUntilLazyHubsUnwind(t *testing.T) {
	srv, _ := newTestServer(t)
	built := 0
	ctx, cancel := context.WithCancel(context.Background())
	// Long timeouts: this is testing shutdown, not the reaper.
	srv.setStreamIdleTimeouts(time.Hour, time.Hour)
	running := make(chan struct{}, 4)
	srv.SetStreamFactory(ctx, func(id string) *StreamPipeline {
		built++
		hub := broadcast.NewHub(blockingSource{running: running}, func() (string, bool) { return "track", true }, nil)
		return &StreamPipeline{Hub: hub, Bus: events.NewBus(), NP: NewNowPlaying()}
	})
	for _, id := range []string{"a", "b"} {
		if err := store.CreateSharedStream(srv.db, id, id); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
		if _, ok := srv.ensurePipeline(id); !ok {
			t.Fatalf("no pipeline for %s", id)
		}
	}
	// Both hubs are inside play(), holding their "encoder" open.
	for i := 0; i < 2; i++ {
		select {
		case <-running:
		case <-time.After(2 * time.Second):
			t.Fatal("pipelines never started playing")
		}
	}

	if srv.WaitForPipelines(50 * time.Millisecond) {
		t.Fatal("WaitForPipelines returned true while both hubs were still running")
	}
	cancel()
	if !srv.WaitForPipelines(5 * time.Second) {
		t.Fatal("lazy pipelines did not unwind after the root context was cancelled")
	}
}

// blockingSource stands in for an ffmpeg child: it keeps producing audio
// indefinitely, a chunk at a time, until the hub closes it. Like the real
// encoder it never ends on its own, so the only way play() returns is the
// context check between reads — which is what shutdown relies on.
type blockingSource struct{ running chan struct{} }

func (b blockingSource) Open(string) (io.ReadCloser, error) {
	b.running <- struct{}{}
	return &blockingReader{}, nil
}

type blockingReader struct{ closed atomic.Bool }

func (r *blockingReader) Read(p []byte) (int, error) {
	if r.closed.Load() {
		return 0, io.EOF
	}
	time.Sleep(time.Millisecond) // pace it like a real-time encoder
	p[0] = 'x'
	return 1, nil
}

func (r *blockingReader) Close() error { r.closed.Store(true); return nil }

// L2: a listener arriving as the reaper tears the stream down must get a fresh
// pipeline, not the dead one's already-closed channel. Otherwise the request
// succeeds with a 200 and then immediately EOFs, which to a browser is a stream
// that connects and plays nothing.
func TestConnectDuringTeardownGetsAFreshPipeline(t *testing.T) {
	srv, _ := newTestServer(t)
	built := 0
	withFactory(t, srv, &built)
	if err := store.CreateSharedStream(srv.db, "kitchen", "Kitchen"); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Hand out the pipeline, then tear it down underneath the caller — the exact
	// interleaving the reaper produces.
	stale, ok := srv.ensurePipeline("kitchen")
	if !ok {
		t.Fatal("no pipeline")
	}
	srv.stopPipeline("kitchen")

	fresh, ok := srv.ensurePipeline("kitchen")
	if !ok {
		t.Fatal("no pipeline after teardown")
	}
	if fresh == stale {
		t.Fatal("ensurePipeline handed back the torn-down pipeline")
	}
	if fresh.Hub.Closed() {
		t.Fatal("ensurePipeline handed back a closed hub")
	}

	// And the listener actually receives audio rather than an instant EOF.
	ch, cancel := fresh.Hub.Listen()
	defer cancel()
	select {
	case _, open := <-ch:
		if !open {
			t.Fatal("listener got a closed channel from a fresh pipeline")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("fresh pipeline delivered nothing")
	}
}

// The HTTP path must survive the same interleaving: /stream/{id}.mp3 arriving
// against a pipeline that is being reaped has to serve audio, not an empty 200.
func TestStreamAudioSurvivesATeardownRace(t *testing.T) {
	srv, _ := newTestServer(t)
	built := 0
	withFactory(t, srv, &built)
	if err := store.CreateSharedStream(srv.db, "kitchen", "Kitchen"); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Warm a pipeline, then kill it so the handler's first lookup finds a corpse.
	if _, ok := srv.ensurePipeline("kitchen"); !ok {
		t.Fatal("no pipeline")
	}
	srv.stopPipeline("kitchen")

	stop := tuneIn(t, srv, "kitchen")
	defer stop()
	if n := srv.listenerCount("kitchen"); n == 0 {
		t.Fatal("listener did not attach to a live pipeline after a teardown")
	}
}
