package api

import (
	"context"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/broadcast"
	"github.com/andybarilla/exit66jukebox/internal/events"
)

// Default lifecycle timings for lazily-started shared streams. Overridable per
// server (tests run them in milliseconds).
const (
	defaultStreamIdleTTL   = time.Minute
	defaultStreamIdleCheck = 5 * time.Second
)

// StreamPipeline is one shared stream's broadcast machinery: the MP3 fan-out
// hub, the SSE bus, and the now-playing holder. house's is built at boot and
// registered directly; every other shared stream's is built on demand by the
// factory and torn down once nobody is tuned in.
type StreamPipeline struct {
	Hub *broadcast.Hub
	Bus *events.Bus
	NP  *NowPlaying

	// persistent pipelines (house) are never reaped and are not started lazily.
	persistent bool
	cancel     context.CancelFunc
}

// dead reports whether this pipeline has been torn down. stopPipeline closes
// both fan-outs, so either being closed means the pipeline can no longer serve.
func (p *StreamPipeline) dead() bool {
	return p.Hub.Closed() || p.Bus.Closed()
}

// SetStreamFactory attaches the builder that constructs a shared stream's
// pipeline on first use, and the context that bounds every pipeline it makes
// (cancelled at shutdown). Left unset in tests and in any build with no
// broadcaster; shared streams then simply have no pipeline.
func (s *Server) SetStreamFactory(ctx context.Context, build func(id string) *StreamPipeline) {
	s.pipesMu.Lock()
	defer s.pipesMu.Unlock()
	s.pipeCtx = ctx
	s.newPipe = build
}

// setStreamIdleTimeouts overrides how long a lazily-started shared stream may
// sit with nobody tuned in before its pipeline (and its ffmpeg child) is torn
// down, and how often that is checked. Unexported: only this package's tests
// need it, and production runs on the defaults above.
func (s *Server) setStreamIdleTimeouts(ttl, check time.Duration) {
	s.pipesMu.Lock()
	defer s.pipesMu.Unlock()
	s.idleTTL, s.idleCheck = ttl, check
}

// RegisterStream attaches an already-running pipeline for a shared stream id.
// This is the house stream's path: built and started at boot, exempt from lazy
// start and from idle teardown. np may be nil for a stream that tracks no
// current track (GET /api/streams/{id} then reports now_playing: null).
func (s *Server) RegisterStream(id string, hub *broadcast.Hub, bus *events.Bus, np *NowPlaying) {
	s.pipesMu.Lock()
	defer s.pipesMu.Unlock()
	s.pipes[id] = &StreamPipeline{Hub: hub, Bus: bus, NP: np, persistent: true}
}

// pipeline returns a stream's running pipeline without starting one.
func (s *Server) pipeline(id string) (*StreamPipeline, bool) {
	s.pipesMu.Lock()
	defer s.pipesMu.Unlock()
	p, ok := s.pipes[id]
	return p, ok
}

// ensurePipeline returns the stream's pipeline, starting one if the stream is
// shared and none is running. This is the lazy-start seam: it is called when a
// listener connects to the audio feed or subscribes to the event stream, so a
// shared stream nobody has tuned into costs nothing.
func (s *Server) ensurePipeline(id string) (*StreamPipeline, bool) {
	if p, ok := s.pipeline(id); ok && !p.dead() {
		return p, true
	}
	// Read the stream row outside the lock: the store call can block, and the
	// double-check below makes the race harmless.
	if !s.isSharedStream(id) {
		return nil, false
	}

	s.pipesMu.Lock()
	// A pipeline the reaper (or a delete) has already closed is a corpse: it
	// hands out pre-closed channels, so a listener attaching to it would get a
	// 200 that immediately EOFs. Replace it rather than serve from it.
	if p, ok := s.pipes[id]; ok {
		if !p.dead() {
			s.pipesMu.Unlock()
			return p, true
		}
		delete(s.pipes, id)
	}
	if s.newPipe == nil {
		s.pipesMu.Unlock()
		return nil, false
	}
	p := s.newPipe(id)
	if p == nil {
		s.pipesMu.Unlock()
		return nil, false
	}
	root := s.pipeCtx
	if root == nil {
		root = context.Background()
	}
	ctx, cancel := context.WithCancel(root)
	p.cancel = cancel
	s.pipes[id] = p
	ttl, check := s.idleTTL, s.idleCheck
	s.pipesMu.Unlock()

	// Tracked so shutdown can wait for the hub to unwind: that is what kills its
	// ffmpeg child, and main would otherwise exit and orphan it.
	s.pipeWG.Add(2)
	go func() { defer s.pipeWG.Done(); p.Hub.Run(ctx) }()
	go func() { defer s.pipeWG.Done(); s.reapWhenIdle(ctx, id, p, ttl, check) }()
	return p, true
}

// attach hands back a live pipeline together with a fan-out registration made
// on it, retrying if the pipeline is torn down in the window between the lookup
// and the registration. Without the retry a listener arriving as the reaper
// fires registers on a corpse and is handed an already-closed channel, which
// reads as a stream that connects and then plays nothing.
//
// register must return false when it registered on a closed fan-out, having
// released whatever it took.
func (s *Server) attach(id string, register func(*StreamPipeline) bool) bool {
	for attempt := 0; attempt < 3; attempt++ {
		p, ok := s.ensurePipeline(id)
		if !ok {
			return false
		}
		if register(p) {
			return true
		}
	}
	return false
}

// stopPipeline tears a stream's pipeline down: subscribers are told the stream
// is closing, then both fan-outs are closed so blocked handlers return rather
// than hanging, then the hub goroutine is cancelled (which kills its ffmpeg
// child). The house pipeline is persistent and is never stopped this way.
func (s *Server) stopPipeline(id string) {
	s.pipesMu.Lock()
	p, ok := s.pipes[id]
	if !ok || p.persistent {
		s.pipesMu.Unlock()
		return
	}
	delete(s.pipes, id)
	s.pipesMu.Unlock()

	p.Bus.Publish(events.Event{Type: "stream-closed", Data: id})
	p.Hub.Close()
	p.Bus.Close()
	if p.cancel != nil {
		p.cancel()
	}
}

// reapWhenIdle stops the pipeline once it has gone ttl with no audio listener
// and no event subscriber. It runs per pipeline and exits with it, so there is
// no server-wide janitor to own.
func (s *Server) reapWhenIdle(ctx context.Context, id string, p *StreamPipeline, ttl, check time.Duration) {
	if ttl <= 0 || check <= 0 {
		return
	}
	tick := time.NewTicker(check)
	defer tick.Stop()
	var idleSince time.Time // zero while someone is tuned in
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tick.C:
			if p.Hub.ListenerCount() > 0 || p.Bus.SubscriberCount() > 0 {
				idleSince = time.Time{}
				continue
			}
			if idleSince.IsZero() {
				idleSince = now
				continue
			}
			if now.Sub(idleSince) >= ttl {
				s.stopPipeline(id)
				return
			}
		}
	}
}

// WaitForPipelines blocks until every lazily-started shared-stream pipeline has
// unwound — each hub's in-flight play() returning is what closes its ffmpeg
// child — or until timeout elapses, reporting whether they all finished. The
// house pipeline is main's own goroutine and is waited on separately.
func (s *Server) WaitForPipelines(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		s.pipeWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// listenerCount returns connected audio listeners for a stream with a running
// pipeline, or 0 for a private stream or one nobody has tuned into.
func (s *Server) listenerCount(streamID string) int {
	if p, ok := s.pipeline(streamID); ok {
		return p.Hub.ListenerCount()
	}
	return 0
}
