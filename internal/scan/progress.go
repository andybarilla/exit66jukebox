package scan

import "sync/atomic"

// Progress is a live, concurrency-safe view of a scan run. The scan workers
// increment its counters as they go; callers read a consistent snapshot via
// Snapshot. The Running flag is owned by whoever launches the scan (it is not
// touched by Scan itself).
type Progress struct {
	running                         atomic.Bool
	added, updated, skipped, failed atomic.Int64
}

// Snapshot is a point-in-time copy of a Progress, suitable for JSON encoding.
type Snapshot struct {
	Running bool `json:"running"`
	Added   int  `json:"added"`
	Updated int  `json:"updated"`
	Skipped int  `json:"skipped"`
	Failed  int  `json:"failed"`
}

// SetRunning marks whether a scan is currently in flight.
func (p *Progress) SetRunning(v bool) { p.running.Store(v) }

// Snapshot returns a consistent copy of the current counts.
func (p *Progress) Snapshot() Snapshot {
	return Snapshot{
		Running: p.running.Load(),
		Added:   int(p.added.Load()),
		Updated: int(p.updated.Load()),
		Skipped: int(p.skipped.Load()),
		Failed:  int(p.failed.Load()),
	}
}
