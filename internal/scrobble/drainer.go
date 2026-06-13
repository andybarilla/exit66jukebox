package scrobble

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/external"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

const (
	baseInterval = 10 * time.Second
	maxInterval  = 5 * time.Minute
)

// Submitter delivers a batch of completed listens to one service. external's
// ListenBrainz client satisfies it; Last.fm (#40) adds another.
type Submitter interface {
	Submit(ctx context.Context, listens []external.Listen) error
}

// Drainer is the single background worker that delivers queued scrobbles. It
// reads pending rows from SQLite each cycle, so it survives restarts and resumes
// wherever it left off.
type Drainer struct {
	db        *sql.DB
	services  map[string]Submitter
	batchSize int
}

// NewDrainer builds a Drainer over db dispatching each service name to its
// Submitter, draining up to batchSize rows per service per cycle.
func NewDrainer(db *sql.DB, services map[string]Submitter, batchSize int) *Drainer {
	return &Drainer{db: db, services: services, batchSize: batchSize}
}

// Run drains every service on a fixed interval until ctx is cancelled, backing
// off after a cycle in which any service failed.
func (d *Drainer) Run(ctx context.Context) {
	consecutiveFailures := 0
	for {
		if ctx.Err() != nil {
			return
		}
		if d.DrainOnce(ctx) {
			consecutiveFailures++
		} else {
			consecutiveFailures = 0
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(nextInterval(baseInterval, consecutiveFailures)):
		}
	}
}

// DrainOnce drains one batch per wired service. It returns true if any service's
// delivery failed (so the caller backs off before the next cycle).
func (d *Drainer) DrainOnce(ctx context.Context) (failed bool) {
	for service, sub := range d.services {
		if err := d.drainService(ctx, service, sub); err != nil {
			log.Printf("scrobble: drain %s: %v", service, err)
			failed = true
		}
	}
	return failed
}

// drainService delivers up to one batch for a single service: resolve metadata,
// submit, delete on success, bump attempts on failure. Orphaned rows (track
// gone) are dropped so they cannot wedge the queue.
func (d *Drainer) drainService(ctx context.Context, service string, sub Submitter) error {
	rows, err := store.ScrobbleBatch(d.db, service, d.batchSize)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	var listens []external.Listen
	var ids []int64
	for _, r := range rows {
		meta, ok, err := store.ScrobbleMetadata(d.db, r.TrackID)
		if err != nil {
			return err
		}
		if !ok {
			if err := store.DeleteScrobble(d.db, r.ID); err != nil {
				return err
			}
			continue
		}
		listens = append(listens, external.Listen{
			ListenedAt: r.PlayedAt,
			Meta: external.ListenMeta{
				ArtistName:  meta.ArtistName,
				TrackName:   meta.TrackName,
				ReleaseName: meta.ReleaseName,
			},
		})
		ids = append(ids, r.ID)
	}
	if len(listens) == 0 {
		return nil
	}
	if err := sub.Submit(ctx, listens); err != nil {
		_ = store.BumpScrobbleAttempts(d.db, ids)
		return err
	}
	for _, id := range ids {
		if err := store.DeleteScrobble(d.db, id); err != nil {
			return err
		}
	}
	return nil
}

// nextInterval returns the wait before the next drain cycle: base doubled per
// consecutive failed cycle, capped at maxInterval.
func nextInterval(base time.Duration, consecutiveFailures int) time.Duration {
	d := base
	for i := 0; i < consecutiveFailures; i++ {
		d *= 2
		if d >= maxInterval {
			return maxInterval
		}
	}
	return d
}
