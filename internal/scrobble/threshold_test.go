package scrobble

import (
	"testing"
	"time"
)

func TestPassesThreshold(t *testing.T) {
	cases := []struct {
		name     string
		duration int
		elapsed  time.Duration
		want     bool
	}{
		{"too short to ever qualify", 30, time.Hour, false},
		{"just over min length, half elapsed", 60, 30 * time.Second, true},
		{"just over min length, under half", 60, 29 * time.Second, false},
		{"long track capped at 4min: 4min elapsed passes", 600, 240 * time.Second, true},
		{"long track capped at 4min: under 4min fails", 600, 239 * time.Second, false},
		{"long track does not require full half (would be 5min)", 600, 241 * time.Second, true},
		{"31s track needs 15s (duration/2 floored)", 31, 15 * time.Second, true},
		{"31s track 14s fails", 31, 14 * time.Second, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PassesThreshold(c.duration, c.elapsed); got != c.want {
				t.Errorf("PassesThreshold(%d, %v) = %v, want %v", c.duration, c.elapsed, got, c.want)
			}
		})
	}
}

func TestFinishEnqueuesOnPass(t *testing.T) {
	var gotTrack, gotPlayed int64
	calls := 0
	enq := func(trackID, playedAt int64) error {
		calls++
		gotTrack, gotPlayed = trackID, playedAt
		return nil
	}
	start := time.Unix(1000, 0)
	end := start.Add(45 * time.Second) // 90s track, >half

	ok, err := Finish(7, 90, start, end, enq)
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if !ok || calls != 1 {
		t.Fatalf("expected enqueue, ok=%v calls=%d", ok, calls)
	}
	if gotTrack != 7 || gotPlayed != 1000 {
		t.Fatalf("enqueued track=%d played_at=%d, want 7/1000", gotTrack, gotPlayed)
	}
}

func TestFinishSkipsShortPlay(t *testing.T) {
	calls := 0
	enq := func(trackID, playedAt int64) error { calls++; return nil }
	start := time.Unix(0, 0)

	// Skipped after 5s of a 200s track.
	ok, err := Finish(1, 200, start, start.Add(5*time.Second), enq)
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if ok || calls != 0 {
		t.Fatalf("short play must not enqueue, ok=%v calls=%d", ok, calls)
	}

	// A <=30s track never qualifies even if played fully (idle transition).
	ok, _ = Finish(2, 20, start, start.Add(20*time.Second), enq)
	if ok || calls != 0 {
		t.Fatalf("<=30s track must not enqueue, ok=%v calls=%d", ok, calls)
	}
}
