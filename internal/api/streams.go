package api

import (
	"net/http"
	"strconv"

	"github.com/andybarilla/exit66jukebox/internal/events"
	"github.com/andybarilla/exit66jukebox/internal/jukebox"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

// getStream reports a stream's queue and live state. It is a read: an unknown
// id reports an empty stream rather than creating a row, so a GET can no longer
// be used to mint streams.
func (s *Server) getStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	q, err := s.jb.Queue(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if q == nil {
		q = []jukebox.QueuedTrack{}
	}
	name, kind := "", store.KindPrivate
	if st, ok, err := store.GetStream(s.db, id); err == nil && ok {
		name, kind = st.Name, st.Kind
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":          id,
		"name":        name,
		"kind":        kind,
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
	// The personal stream is still created on first touch (#22 leaves its
	// keying alone, see #128). The kind is pinned to private, so no id invented
	// here can ever become a shared stream and reach the admin-gated controls.
	if err := store.EnsurePrivateStream(s.db, id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
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
