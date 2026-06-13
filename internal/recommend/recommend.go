// Package recommend builds the external Recommended discovery surface: it pulls
// ListenBrainz collaborative-filter recs and Last.fm similar-artist data, maps
// them to local tracks, and caches the enriched result in memory. The cache is
// served instantly; a stale or empty cache kicks an async refresh so a Discover
// request never blocks on external HTTP. Each source is independently optional.
package recommend

import (
	"context"
	"database/sql"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/external"
	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

const (
	ttl             = 6 * time.Hour // how long a refresh stays fresh
	recCount        = 100           // ListenBrainz recommendations requested
	seedArtists     = 10            // top local artists used as similarity seeds
	similarPerSeed  = 20            // similar artists requested per seed
	tracksPerArtist = 3             // local tracks surfaced per matched similar artist
)

// lbRecommender is the slice of the ListenBrainz client the runner needs.
type lbRecommender interface {
	Username(ctx context.Context) (string, error)
	Recommendations(ctx context.Context, user string, count int) ([]external.RecRecording, error)
}

// lastfmSimilar is the slice of the Last.fm client the runner needs.
type lastfmSimilar interface {
	SimilarArtists(ctx context.Context, name, mbid string, limit int) ([]external.SimilarArtist, error)
}

// Runner owns the in-memory recommendation cache and its refresh state. lb and
// lf are nil when their service is unconfigured.
type Runner struct {
	db  *sql.DB
	lb  lbRecommender
	lf  lastfmSimilar
	now func() time.Time

	mu          sync.Mutex
	cache       []model.EnrichedTrack
	lastRefresh time.Time
	refreshing  bool
}

// NewRunner wires the runner. Pass nil for any unconfigured source.
func NewRunner(db *sql.DB, lb lbRecommender, lf lastfmSimilar) *Runner {
	return &Runner{db: db, lb: lb, lf: lf, now: time.Now}
}

// Get returns the cached recommendations immediately. When the cache has never
// been built or is older than the TTL, it kicks a single background refresh and
// returns whatever is currently cached (possibly empty) — it never blocks on
// the network.
func (r *Runner) Get() []model.EnrichedTrack {
	r.mu.Lock()
	defer r.mu.Unlock()
	stale := r.lastRefresh.IsZero() || r.now().Sub(r.lastRefresh) >= ttl
	if stale && !r.refreshing && (r.lb != nil || r.lf != nil) {
		r.refreshing = true
		go func() {
			defer func() {
				r.mu.Lock()
				r.refreshing = false
				r.mu.Unlock()
			}()
			r.Refresh(context.Background())
		}()
	}
	return r.cache
}

// Refresh rebuilds the cache synchronously: gather from each enabled source, map
// to local tracks, dedupe, enrich, and store. Used by Get's background goroutine
// and directly in tests.
func (r *Runner) Refresh(ctx context.Context) {
	tracks := r.gather(ctx)
	r.mu.Lock()
	if r.cache == nil {
		r.cache = []model.EnrichedTrack{}
	}
	r.cache = tracks
	r.lastRefresh = r.now()
	r.mu.Unlock()
}

// gather collects recommendations from every enabled source, mapping each to
// local tracks and deduping by track id. Sources degrade independently: a
// failing source is logged and contributes nothing.
func (r *Runner) gather(ctx context.Context) []model.EnrichedTrack {
	seen := make(map[int64]bool)
	var tracks []model.Track
	add := func(ts []model.Track) {
		for _, t := range ts {
			if !seen[t.ID] {
				seen[t.ID] = true
				tracks = append(tracks, t)
			}
		}
	}

	if r.lb != nil {
		add(r.listenBrainzTracks(ctx))
	}
	if r.lf != nil {
		add(r.lastfmTracks(ctx))
	}

	enriched, err := store.EnrichTracks(r.db, tracks)
	if err != nil {
		log.Printf("recommend: enrich: %v", err)
		return []model.EnrichedTrack{}
	}
	return enriched
}

func (r *Runner) listenBrainzTracks(ctx context.Context) []model.Track {
	user, err := r.lb.Username(ctx)
	if err != nil {
		log.Printf("recommend: listenbrainz username: %v", err)
		return nil
	}
	recs, err := r.lb.Recommendations(ctx, user, recCount)
	if err != nil {
		log.Printf("recommend: listenbrainz recommendations: %v", err)
		return nil
	}
	mbids := make([]string, len(recs))
	for i, rec := range recs {
		mbids[i] = rec.RecordingMBID
	}
	tracks, err := store.TracksByRecordingMBIDs(r.db, mbids)
	if err != nil {
		log.Printf("recommend: map recordings: %v", err)
		return nil
	}
	return tracks
}

func (r *Runner) lastfmTracks(ctx context.Context) []model.Track {
	seeds, err := store.TopArtists(r.db, seedArtists)
	if err != nil {
		log.Printf("recommend: top artists: %v", err)
		return nil
	}
	nameSet := make(map[string]bool)
	mbidSet := make(map[string]bool)
	for _, s := range seeds {
		sim, err := r.lf.SimilarArtists(ctx, s.Name, s.Mbid, similarPerSeed)
		if err != nil {
			log.Printf("recommend: similar to %q: %v", s.Name, err)
			continue
		}
		for _, a := range sim {
			if a.MBID != "" {
				mbidSet[a.MBID] = true
			} else if a.Name != "" {
				nameSet[strings.ToLower(a.Name)] = true
			}
		}
	}
	if len(nameSet) == 0 && len(mbidSet) == 0 {
		return nil
	}
	tracks, err := store.TracksBySimilarArtists(r.db, keys(nameSet), keys(mbidSet), tracksPerArtist)
	if err != nil {
		log.Printf("recommend: map similar artists: %v", err)
		return nil
	}
	return tracks
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
