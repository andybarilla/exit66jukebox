// Package enrich runs the MusicBrainz/Cover Art Archive enrichment pass: it
// walks tracks with no MBID, matches each against MusicBrainz, records the
// MBIDs, backfills placeholder tags, and downloads a front cover for albums
// that have none. The pass runs in a background goroutine, is resumable
// (skips already-matched tracks), and refuses to run twice concurrently.
package enrich

import (
	"context"
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/andybarilla/exit66jukebox/internal/external"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

// matchThreshold is the minimum MusicBrainz score (0–100) accepted as a
// confident match. Below it the track is left for a future pass.
const matchThreshold = 90

// recordingSearcher and coverFetcher are the slices of the external clients the
// pass needs; narrow interfaces let tests inject fakes with no network.
type recordingSearcher interface {
	SearchRecording(ctx context.Context, artist, title, album string) (external.RecordingMatch, bool, error)
}

type coverFetcher interface {
	FetchFrontCover(ctx context.Context, releaseMBID string) (data []byte, contentType string, ok bool, err error)
}

// Status is a snapshot of the current/last pass.
type Status struct {
	Running   bool `json:"running"`
	Processed int  `json:"processed"`
	Matched   int  `json:"matched"`
	Covers    int  `json:"covers"`
	Failed    int  `json:"failed"`
}

// Runner owns the pass and its guarded status.
type Runner struct {
	db        *sql.DB
	mb        recordingSearcher
	ca        coverFetcher
	coversDir string

	mu     sync.Mutex
	status Status
}

// NewRunner wires the runner to its dependencies. coversDir must already exist.
func NewRunner(db *sql.DB, mb recordingSearcher, ca coverFetcher, coversDir string) *Runner {
	return &Runner{db: db, mb: mb, ca: ca, coversDir: coversDir}
}

// Start launches a pass in the background. If one is already running it returns
// the current status and false without starting a second.
func (r *Runner) Start(ctx context.Context) (Status, bool) {
	r.mu.Lock()
	if r.status.Running {
		s := r.status
		r.mu.Unlock()
		return s, false
	}
	r.status = Status{Running: true}
	s := r.status
	r.mu.Unlock()

	go r.run(ctx)
	return s, true
}

// Status returns a snapshot of the current/last pass.
func (r *Runner) Status() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

func (r *Runner) run(ctx context.Context) {
	defer func() {
		r.mu.Lock()
		r.status.Running = false
		s := r.status
		r.mu.Unlock()
		log.Printf("enrich done: processed=%d matched=%d covers=%d failed=%d",
			s.Processed, s.Matched, s.Covers, s.Failed)
	}()

	targets, err := store.TracksNeedingEnrichment(r.db)
	if err != nil {
		log.Printf("enrich: list targets: %v", err)
		return
	}
	for _, t := range targets {
		if ctx.Err() != nil {
			return
		}
		r.processTrack(ctx, t)
	}
}

func (r *Runner) processTrack(ctx context.Context, t store.EnrichTarget) {
	defer func() {
		r.mu.Lock()
		r.status.Processed++
		r.mu.Unlock()
	}()

	match, ok, err := r.mb.SearchRecording(ctx, t.Artist, t.Title, t.Album)
	if err != nil {
		log.Printf("enrich: search track %d: %v", t.TrackID, err)
		r.bump(&r.status.Failed)
		return
	}
	if !ok || match.Score < matchThreshold {
		return
	}

	if err := store.ApplyEnrichment(r.db, store.Enrichment{
		TrackID: t.TrackID, ArtistID: t.ArtistID, AlbumID: t.AlbumID, Path: t.Path,
		RecordingMBID: match.RecordingMBID, ArtistMBID: match.ArtistMBID, ReleaseMBID: match.ReleaseMBID,
		NewTitle: match.RecordingTitle, NewArtist: match.ArtistName, NewAlbum: match.ReleaseTitle,
	}); err != nil {
		log.Printf("enrich: apply track %d: %v", t.TrackID, err)
		r.bump(&r.status.Failed)
		return
	}
	r.bump(&r.status.Matched)

	r.maybeFetchCover(ctx, t, match.ReleaseMBID)
}

// maybeFetchCover downloads a front cover when the matched album has a release
// MBID and no cover yet.
func (r *Runner) maybeFetchCover(ctx context.Context, t store.EnrichTarget, releaseMBID string) {
	if releaseMBID == "" {
		return
	}
	if _, has := store.AlbumCoverByTrack(r.db, t.TrackID); has {
		return
	}
	data, ct, ok, err := r.ca.FetchFrontCover(ctx, releaseMBID)
	if err != nil {
		log.Printf("enrich: fetch cover album %d: %v", t.AlbumID, err)
		r.bump(&r.status.Failed)
		return
	}
	if !ok {
		return
	}
	path := filepath.Join(r.coversDir, coverFilename(t.AlbumID, ct))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Printf("enrich: write cover %s: %v", path, err)
		r.bump(&r.status.Failed)
		return
	}
	if err := store.SetAlbumCover(r.db, t.AlbumID, path); err != nil {
		log.Printf("enrich: set cover album %d: %v", t.AlbumID, err)
		r.bump(&r.status.Failed)
		return
	}
	r.bump(&r.status.Covers)
}

func (r *Runner) bump(field *int) {
	r.mu.Lock()
	*field++
	r.mu.Unlock()
}

// coverFilename names the cached cover by album id with an extension inferred
// from the content type.
func coverFilename(albumID int64, contentType string) string {
	ext := ".img"
	switch contentType {
	case "image/jpeg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	case "image/gif":
		ext = ".gif"
	case "image/webp":
		ext = ".webp"
	}
	return strconv.FormatInt(albumID, 10) + ext
}
