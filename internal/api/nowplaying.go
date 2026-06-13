package api

import (
	"sync"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/model"
)

// NowPlaying tracks the track a shared stream is currently playing and when it
// started, so a freshly-connected client can seed its now-playing view instead
// of waiting for the next SSE event. It is written by the broadcaster's next
// closure and read by HTTP handlers, so all access is mutex-guarded.
type NowPlaying struct {
	mu        sync.Mutex
	track     model.Track
	startedAt time.Time
	playing   bool
	now       func() time.Time // injectable clock for tests
}

func NewNowPlaying() *NowPlaying {
	return &NowPlaying{now: time.Now}
}

// Set records the track as now playing, starting its offset clock from now.
func (np *NowPlaying) Set(tr model.Track) {
	np.mu.Lock()
	defer np.mu.Unlock()
	np.track = tr
	np.startedAt = np.now()
	np.playing = true
}

// Clear marks the stream idle.
func (np *NowPlaying) Clear() {
	np.mu.Lock()
	defer np.mu.Unlock()
	np.playing = false
	np.track = model.Track{}
}

// Current returns the playing track and its approximate playback offset in
// seconds. ok is false when the stream is idle.
func (np *NowPlaying) Current() (tr model.Track, offsetSeconds int, ok bool) {
	np.mu.Lock()
	defer np.mu.Unlock()
	if !np.playing {
		return model.Track{}, 0, false
	}
	offset := int(np.now().Sub(np.startedAt).Seconds())
	if offset < 0 {
		offset = 0
	}
	return np.track, offset, true
}
