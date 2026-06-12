package jukebox

import (
	"database/sql"
	"sync"

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

// QueuedTrack is a queued track plus who requested it.
type QueuedTrack struct {
	Track       model.Track `json:"track"`
	RequestedBy string      `json:"requested_by"`
}

// Jukebox applies fairness rules over the store. Safe for concurrent use because
// SQLite serializes writes; callers may share one instance.
type Jukebox struct {
	db      *sql.DB
	cfg     Config
	mu      sync.Mutex
	shuffle map[string]bool
}

func New(db *sql.DB, cfg Config) *Jukebox {
	if cfg.HistoryWindow < 0 {
		cfg.HistoryWindow = 0
	}
	return &Jukebox{db: db, cfg: cfg, shuffle: make(map[string]bool)}
}

// EnsureStream creates the stream if it does not yet exist.
func (j *Jukebox) EnsureStream(id, kind string) error {
	return store.EnsureStream(j.db, id, "", kind)
}

// SetShuffle sets the per-stream shuffle flag (affects what Next pops).
func (j *Jukebox) SetShuffle(streamID string, on bool) {
	j.mu.Lock()
	j.shuffle[streamID] = on
	j.mu.Unlock()
}

// Shuffle reports the per-stream shuffle flag.
func (j *Jukebox) Shuffle(streamID string) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.shuffle[streamID]
}

// Request applies the fairness rules and enqueues the track if it passes.
// Returns a non-nil error only on an underlying DB failure (not on a fairness
// rejection, which is reported via Result).
func (j *Jukebox) Request(streamID string, trackID int64, requestedBy string) (Result, error) {
	dup, err := store.InQueue(j.db, streamID, trackID)
	if err != nil {
		return Requested, err
	}
	if dup {
		return AlreadyQueued, nil
	}
	if j.cfg.HistoryWindow > 0 {
		recent, err := store.RecentlyPlayed(j.db, streamID, trackID, j.cfg.HistoryWindow)
		if err != nil {
			return Requested, err
		}
		if recent {
			return RecentlyPlayed, nil
		}
	}
	if err := store.Enqueue(j.db, streamID, trackID, requestedBy); err != nil {
		return Requested, err
	}
	return Requested, nil
}

// Next pops the next track in play order. ok=false if the queue is empty.
func (j *Jukebox) Next(streamID string) (model.Track, bool) {
	var id int64
	var ok bool
	if j.Shuffle(streamID) {
		id, ok = store.PopNextShuffle(j.db, streamID)
	} else {
		id, ok = store.PopNext(j.db, streamID)
	}
	if !ok {
		return model.Track{}, false
	}
	tr, _, found := store.GetTrack(j.db, id)
	if !found {
		return model.Track{}, false
	}
	j.refill(streamID)
	return tr, true
}

// StartStation attaches a genre radio to the stream and immediately fills the
// queue. An empty queue never drains (Next is never called), so the initial
// fill is what gets playback going.
func (j *Jukebox) StartStation(streamID, genre string, threshold, batch int) error {
	if err := j.EnsureStream(streamID, "private"); err != nil {
		return err
	}
	if err := store.UpsertStation(j.db, store.Station{
		StreamID: streamID, Genre: genre, Threshold: threshold, Batch: batch,
	}); err != nil {
		return err
	}
	j.refill(streamID)
	return nil
}

// StopStation detaches the radio; queued tracks keep playing.
func (j *Jukebox) StopStation(streamID string) error {
	return store.DeleteStation(j.db, streamID)
}

// GetStation returns the stream's station, ok=false if none.
func (j *Jukebox) GetStation(streamID string) (store.Station, bool) {
	return store.GetStation(j.db, streamID)
}

// refill tops the queue up to the station's batch size when it has fallen below
// the threshold. No-op when no station is attached or the genre is exhausted.
// Reuses Request so fairness rules (dedupe + HistoryWindow) apply.
func (j *Jukebox) refill(streamID string) {
	st, ok := store.GetStation(j.db, streamID)
	if !ok {
		return
	}
	n, err := store.QueueLen(j.db, streamID)
	if err != nil || n >= st.Threshold {
		return
	}
	tracks, err := store.DiscoverTracks(j.db, store.DiscoverOpts{
		Genre:         st.Genre,
		OrderBy:       "random",
		ExcludeStream: streamID,
		Window:        j.cfg.HistoryWindow,
		Limit:         st.Batch,
	})
	if err != nil {
		return
	}
	for _, tr := range tracks {
		j.Request(streamID, tr.ID, "station") // fairness-checked; ignore per-track result
	}
}

// Queue returns the queued tracks in play order.
func (j *Jukebox) Queue(streamID string) ([]QueuedTrack, error) {
	rows, err := store.QueueWithRequester(j.db, streamID)
	if err != nil {
		return nil, err
	}
	out := make([]QueuedTrack, 0, len(rows))
	for _, r := range rows {
		if tr, _, ok := store.GetTrack(j.db, r.TrackID); ok {
			out = append(out, QueuedTrack{Track: tr, RequestedBy: r.RequestedBy})
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
func (j *Jukebox) RequestAlbum(streamID string, albumID int64, requestedBy string) int {
	ids, _ := store.TrackIDsByAlbum(j.db, albumID)
	return j.requestMany(streamID, ids, requestedBy)
}

// RequestArtist requests every track by an artist, returning how many were newly
// queued.
func (j *Jukebox) RequestArtist(streamID string, artistID int64, requestedBy string) int {
	ids, _ := store.TrackIDsByArtist(j.db, artistID)
	return j.requestMany(streamID, ids, requestedBy)
}

func (j *Jukebox) requestMany(streamID string, ids []int64, requestedBy string) int {
	queued := 0
	for _, id := range ids {
		if res, err := j.Request(streamID, id, requestedBy); err == nil && res == Requested {
			queued++
		}
	}
	return queued
}
