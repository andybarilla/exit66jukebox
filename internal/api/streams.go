package api

import (
	"net/http"
	"strconv"

	"github.com/andybarilla/exit66jukebox/internal/events"
	"github.com/andybarilla/exit66jukebox/internal/jukebox"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

// getStream reports a stream's queue and live state. It is a read: it creates
// no row, so a GET cannot be used to mint streams.
//
// An id with no row is refused rather than answered with an empty stream. It
// used to be answered, reporting a kind nothing had stored — "private", which
// since #128 names one particular user's queue. 404 is what request and
// streamGate already answer for an unknown id, so the read and the mutating
// routes now agree. It also closes an oracle: resolvePersonalStream 404s a
// private row precisely so the answer does not reveal that it exists, and a
// 200 here gave that away by contrast, since only an id with no row got one
// (#142).
func (s *Server) getStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	st, ok, err := store.GetStream(s.db, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, "no such stream")
		return
	}
	q, err := s.jb.Queue(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if q == nil {
		q = []jukebox.QueuedTrack{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":          id,
		"name":        st.Name,
		"kind":        st.Kind,
		"queue":       q,
		"listeners":   s.listenerCount(id),
		"now_playing": s.nowPlayingPayload(id),
	})
}

// nowPlayingPayload returns {track, offset_seconds} for a shared stream with a
// running pipeline, or nil (JSON null) when idle, private, or not yet started.
func (s *Server) nowPlayingPayload(streamID string) any {
	p, ok := s.pipeline(streamID)
	if !ok || p.NP == nil {
		return nil
	}
	tr, offset, playing := p.NP.Current()
	if !playing {
		return nil
	}
	return map[string]any{
		"track":          s.enrichOne(tr),
		"offset_seconds": offset,
	}
}

func (s *Server) nextTrack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tr, ok := s.jb.Next(id)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false})
		return
	}
	s.publishQueueChanged(id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "track": s.enrichOne(tr)})
}

func (s *Server) request(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// A request names a stream, it does not create one: creating on touch let
	// any listener mint unbounded private streams by choosing URLs.
	if _, ok, err := store.GetStream(s.db, id); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	} else if !ok {
		writeErr(w, http.StatusNotFound, "no such stream")
		return
	}
	r.ParseForm()
	by := r.FormValue("by")
	kind := r.FormValue("kind")
	if kind == "" {
		kind = "track"
	}
	targetID, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
	if err != nil || targetID <= 0 {
		writeErr(w, http.StatusBadRequest, "missing or invalid id")
		return
	}

	switch kind {
	case "album":
		n := s.jb.RequestAlbum(id, targetID, by)
		if n > 0 {
			s.publishQueueChanged(id)
		}
		writeJSON(w, http.StatusOK, map[string]any{"queued": n, "message": ""})
	case "artist":
		n := s.jb.RequestArtist(id, targetID, by)
		if n > 0 {
			s.publishQueueChanged(id)
		}
		writeJSON(w, http.StatusOK, map[string]any{"queued": n, "message": ""})
	case "track":
		res, err := s.jb.Request(id, targetID, by)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		queued := 0
		if res == jukebox.Requested {
			queued = 1
			s.publishQueueChanged(id)
		}
		writeJSON(w, http.StatusOK, map[string]any{"queued": queued, "message": res.Message()})
	default:
		writeErr(w, http.StatusBadRequest, "invalid kind")
	}
}

func (s *Server) removeRequest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	trackID, err := strconv.ParseInt(r.PathValue("trackID"), 10, 64)
	if err != nil || trackID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid track id")
		return
	}
	if err := s.jb.Remove(id, trackID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.publishQueueChanged(id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) clearRequests(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.jb.Clear(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.publishQueueChanged(id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) publishQueueChanged(streamID string) {
	if p, ok := s.pipeline(streamID); ok {
		p.Bus.Publish(events.Event{Type: "queue-changed", Data: streamID})
	}
}

func (s *Server) setShuffle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	r.ParseForm()
	on := r.FormValue("value") == "true" || r.FormValue("value") == "1"
	s.jb.SetShuffle(id, on)
	writeJSON(w, http.StatusOK, map[string]any{"shuffle": on})
}
