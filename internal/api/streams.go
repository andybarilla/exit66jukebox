package api

import (
	"net/http"
	"strconv"

	"github.com/andybarilla/exit66jukebox/internal/events"
	"github.com/andybarilla/exit66jukebox/internal/jukebox"
)

func (s *Server) getStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.jb.EnsureStream(id, "private"); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
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
		"id":        id,
		"queue":     q,
		"listeners": s.listenerCount(id),
	})
}

func (s *Server) nextTrack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.jb.EnsureStream(id, "private"); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
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
	if err := s.jb.EnsureStream(id, "private"); err != nil {
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
	if bus, ok := s.buses[streamID]; ok {
		bus.Publish(events.Event{Type: "queue-changed", Data: streamID})
	}
}

func (s *Server) setShuffle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	r.ParseForm()
	on := r.FormValue("value") == "true" || r.FormValue("value") == "1"
	s.jb.SetShuffle(id, on)
	writeJSON(w, http.StatusOK, map[string]any{"shuffle": on})
}
