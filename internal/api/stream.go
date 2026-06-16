package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/auth"
)

// streamAudio fans the shared stream's continuous MP3 feed to this listener.
// Handles GET /stream/<id>.mp3 (the standard mux doesn't support suffix wildcards).
func (s *Server) streamAudio(w http.ResponseWriter, r *http.Request) {
	seg := strings.TrimPrefix(r.URL.Path, "/stream/")
	id := strings.TrimSuffix(seg, ".mp3")
	if id == "" || id == seg {
		http.NotFound(w, r)
		return
	}
	hub, ok := s.hubs[id]
	if !ok {
		http.NotFound(w, r)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch, cancel := hub.Listen()
	defer cancel()
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

// streamAudioGuarded authorizes a /stream/ request by EITHER a session/guest/
// loopback (mediaAllowed) OR a valid path-scoped signed token (the Sonos cast,
// which fetches with no cookie), then serves the stream. The /stream/ route is
// not under /api/, so the top-level auth middleware doesn't cover it — this is
// its gate.
func (s *Server) streamAudioGuarded(w http.ResponseWriter, r *http.Request) {
	if s.mediaAllowed(r) {
		s.streamAudio(w, r)
		return
	}
	if sig := r.URL.Query().Get("sig"); sig != "" &&
		auth.VerifyPath(s.signingSecret, sig, r.URL.Path, time.Now().Unix()) {
		s.streamAudio(w, r)
		return
	}
	writeErr(w, http.StatusUnauthorized, "login required")
}

// streamEvents is an SSE endpoint pushing now-playing/queue-changed events.
func (s *Server) streamEvents(w http.ResponseWriter, r *http.Request) {
	bus, ok := s.buses[r.PathValue("id")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch, cancel := bus.Subscribe()
	defer cancel()
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
