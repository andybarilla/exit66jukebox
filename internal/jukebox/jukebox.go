package jukebox

import (
	"database/sql"

	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

// Result is the outcome of a single track request.
type Result int

const (
	Requested Result = iota
	AlreadyQueued
	RecentlyPlayed
)

func (r Result) Message() string {
	switch r {
	case AlreadyQueued:
		return "That track is already in your queue."
	case RecentlyPlayed:
		return "That track was just played. Try something else."
	default:
		return "Thanks for the request!"
	}
}

// Config holds per-stream fairness tuning.
type Config struct {
	HistoryWindow int // how many recent plays block a re-request
}

// Jukebox applies fairness rules over the store. Safe for concurrent use because
// SQLite serializes writes; callers may share one instance.
type Jukebox struct {
	db  *sql.DB
	cfg Config
}

func New(db *sql.DB, cfg Config) *Jukebox {
	if cfg.HistoryWindow < 0 {
		cfg.HistoryWindow = 0
	}
	return &Jukebox{db: db, cfg: cfg}
}

// EnsureStream creates the stream if it does not yet exist.
func (j *Jukebox) EnsureStream(id, kind string) error {
	return store.EnsureStream(j.db, id, "", kind)
}

// Request applies the fairness rules and enqueues the track if it passes.
func (j *Jukebox) Request(streamID string, trackID int64) Result {
	if dup, _ := store.InQueue(j.db, streamID, trackID); dup {
		return AlreadyQueued
	}
	if j.cfg.HistoryWindow > 0 {
		if recent, _ := store.RecentlyPlayed(j.db, streamID, trackID, j.cfg.HistoryWindow); recent {
			return RecentlyPlayed
		}
	}
	store.Enqueue(j.db, streamID, trackID, streamID)
	return Requested
}

// Next pops the next track in play order. ok=false if the queue is empty.
func (j *Jukebox) Next(streamID string) (model.Track, bool) {
	id, ok := store.PopNext(j.db, streamID)
	if !ok {
		return model.Track{}, false
	}
	tr, _, found := store.GetTrack(j.db, id)
	if !found {
		return model.Track{}, false
	}
	return tr, true
}

// Queue returns the queued tracks in play order.
func (j *Jukebox) Queue(streamID string) ([]model.Track, error) {
	ids, err := store.QueueTrackIDs(j.db, streamID)
	if err != nil {
		return nil, err
	}
	out := make([]model.Track, 0, len(ids))
	for _, id := range ids {
		if tr, _, ok := store.GetTrack(j.db, id); ok {
			out = append(out, tr)
		}
	}
	return out, nil
}

// Remove drops one track from the queue.
func (j *Jukebox) Remove(streamID string, trackID int64) error {
	return store.RemoveFromQueue(j.db, streamID, trackID)
}

// Clear empties the queue.
func (j *Jukebox) Clear(streamID string) error {
	return store.ClearQueue(j.db, streamID)
}

// RequestAlbum requests every track on an album, returning how many were newly
// queued (tracks rejected by fairness are not counted).
func (j *Jukebox) RequestAlbum(streamID string, albumID int64) int {
	ids, _ := store.TrackIDsByAlbum(j.db, albumID)
	return j.requestMany(streamID, ids)
}

// RequestArtist requests every track by an artist, returning how many were newly
// queued.
func (j *Jukebox) RequestArtist(streamID string, artistID int64) int {
	ids, _ := store.TrackIDsByArtist(j.db, artistID)
	return j.requestMany(streamID, ids)
}

func (j *Jukebox) requestMany(streamID string, ids []int64) int {
	queued := 0
	for _, id := range ids {
		if j.Request(streamID, id) == Requested {
			queued++
		}
	}
	return queued
}
