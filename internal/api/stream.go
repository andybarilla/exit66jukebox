package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/andybarilla/exit66jukebox/internal/events"
)

// streamAudio fans a shared stream's continuous MP3 feed to this listener.
// Handles GET /stream/<id>.mp3 (the standard mux doesn't support suffix wildcards).
func (s *Server) streamAudio(w http.ResponseWriter, r *http.Request) {
	seg := strings.TrimPrefix(r.URL.Path, "/stream/")
	id := strings.TrimSuffix(seg, ".mp3")
	if id == "" || id == seg {
		http.NotFound(w, r)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	// First listener on a shared stream is what starts its pipeline (and, once
	// it pops a track, its encoder). house's is already running. attach retries
	// if the pipeline is reaped between the lookup and the registration.
	var ch <-chan []byte
	var cancel func()
	if !s.attach(id, func(p *StreamPipeline) bool {
		c, stop := p.Hub.Listen()
		if p.Hub.Closed() {
			stop()
			return false
		}
		ch, cancel = c, stop
		return true
	}) {
		http.NotFound(w, r)
		return
	}
	defer cancel()
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case chunk, open := <-ch:
			if !open {
				return
			}
			if _, err := w.Write(chunk); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// streamAudioGuarded authorizes a /stream/ request by EITHER a session/guest
// (mediaAllowed) OR a valid path-scoped signed token (the Sonos cast, which
// fetches with no cookie), then serves the stream. The /stream/ route is not
// under /api/, so the top-level auth middleware doesn't cover it — this is its
// gate.
func (s *Server) streamAudioGuarded(w http.ResponseWriter, r *http.Request) {
	if s.mediaAllowed(r) || s.signedOK(r) {
		s.streamAudio(w, r)
		return
	}
	writeErr(w, http.StatusUnauthorized, "login required")
}

// streamEvents is an SSE endpoint pushing now-playing/queue-changed events.
func (s *Server) streamEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	var ch <-chan events.Event
	var cancel func()
	if !s.attach(r.PathValue("id"), func(p *StreamPipeline) bool {
		c, stop := p.Bus.Subscribe()
		if p.Bus.Closed() {
			stop()
			return false
		}
		ch, cancel = c, stop
		return true
	}) {
		http.NotFound(w, r)
		return
	}
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case e, open := <-ch:
			if !open {
				return
			}
			data, _ := json.Marshal(e)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}
