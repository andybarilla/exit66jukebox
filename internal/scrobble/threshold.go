// Package scrobble owns the rules and delivery for outward scrobbling: the
// completed-play threshold gate and the durable-queue drainer. The house
// stream's current-track + start-time holder lives in the playback loop
// (main.go); this package supplies the pure decision it consults.
package scrobble

import "time"

// minDuration is the shortest track worth scrobbling; ListenBrainz/Last.fm both
// ignore anything 30s or under.
const minDuration = 30

// maxRequired caps the elapsed-listening requirement at 4 minutes, so long
// tracks scrobble well before they finish.
const maxRequired = 240

// PassesThreshold reports whether a completed play should be scrobbled: the
// track must be longer than 30s and at least min(duration/2, 240s) must have
// elapsed.
func PassesThreshold(duration int, elapsed time.Duration) bool {
	if duration <= minDuration {
		return false
	}
	required := duration / 2
	if required > maxRequired {
		required = maxRequired
	}
	return int(elapsed.Seconds()) >= required
}

// Enqueuer persists a completed listen for every enabled service.
type Enqueuer func(trackID, playedAt int64) error

// Finish evaluates a finished house play. When it clears the threshold the
// listen is enqueued via enq with played_at = the track's start time. ok is
// whether it enqueued. The same call handles the play→idle transition (the last
// track) — there is no separate idle path.
func Finish(trackID int64, duration int, startedAt, end time.Time, enq Enqueuer) (ok bool, err error) {
	if !PassesThreshold(duration, end.Sub(startedAt)) {
		return false, nil
	}
	return true, enq(trackID, startedAt.Unix())
}
