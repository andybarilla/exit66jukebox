package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/api"
	"github.com/andybarilla/exit66jukebox/internal/broadcast"
	"github.com/andybarilla/exit66jukebox/internal/config"
	"github.com/andybarilla/exit66jukebox/internal/enrich"
	"github.com/andybarilla/exit66jukebox/internal/events"
	"github.com/andybarilla/exit66jukebox/internal/external"
	"github.com/andybarilla/exit66jukebox/internal/jukebox"
	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/scan"
	"github.com/andybarilla/exit66jukebox/internal/scrobble"
	"github.com/andybarilla/exit66jukebox/internal/store"
	"github.com/andybarilla/exit66jukebox/internal/web"
)

// houseTrack is the single holder of the currently-playing house track and when
// it started, owned by the house playback loop. It feeds the scrobble threshold
// (this issue) and is the holder #28's now-playing work reuses rather than
// duplicating.
type houseTrack struct {
	id        int64
	duration  int
	startedAt time.Time
}

func main() {
	cfg, err := config.Parse(os.Args[1:])
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	jb := jukebox.New(db, jukebox.Config{HistoryWindow: cfg.HistoryWindow})

	// Shared rate-limited HTTP client for all external services (MusicBrainz
	// enrichment + scrobbling). Scrobble services are configured from env; a
	// service with no credentials stays disabled and the app runs as before.
	extClient := external.New("exit66jukebox/0.1 (+https://github.com/andybarilla/exit66jukebox)", time.Second)
	var lb *external.ListenBrainz
	var enabledServices []string
	submitters := map[string]scrobble.Submitter{}
	if cfg.Services.ListenBrainzEnabled() {
		lb = external.NewListenBrainz(extClient, cfg.Services.ListenBrainzToken)
		submitters["listenbrainz"] = lb
		enabledServices = append(enabledServices, "listenbrainz")
		log.Print("ListenBrainz scrobbling enabled")
	}

	// Initial scan in the background so the server comes up immediately. The
	// shared Progress is attached to the API server below so GET /api/scan can
	// report live status; it stays nil when no library is configured.
	var scanProgress *scan.Progress
	if roots := cfg.Library(); len(roots) > 0 {
		scanProgress = &scan.Progress{}
		scanProgress.SetRunning(true)
		go func() {
			defer scanProgress.SetRunning(false)
			log.Printf("scanning %v ...", roots)
			res, err := scan.Scan(db, roots, cfg.ScanWorkers, scanProgress)
			if err != nil {
				log.Printf("scan error: %v", err)
				return
			}
			log.Printf("scan done: added=%d updated=%d skipped=%d failed=%d",
				res.Added, res.Updated, res.Skipped, res.Failed)
		}()
	}

	// Always-on "house" shared stream: one continuous MP3 feed driven by the
	// shared queue, that any browser/Sonos can tune into.
	const houseID = "house"
	if err := jb.EnsureStream(houseID, "shared"); err != nil {
		log.Fatalf("ensure house stream: %v", err)
	}
	houseBus := events.NewBus()
	silence := broadcast.GenerateSilence(1)
	if silence == nil {
		log.Print("warning: MP3 silence generation failed (is ffmpeg installed?); the house stream will send nothing while idle")
	}

	// next pops the house queue and publishes now-playing; returns the file path
	// for the broadcaster. Called repeatedly in the hub's single goroutine, so
	// the current-track holder needs no lock. Publishes a null now-playing once
	// when the stream transitions from playing to idle (empty queue).
	//
	// Scrobble seam: the broadcast Source is real-time-paced, so the gap between
	// two pops ≈ the just-finished track's play time. On each pop (and on the
	// play→idle transition) the previous house track is settled — enqueued for
	// every enabled service when it clears the threshold. Any network work
	// (now-playing) is fire-and-forget so it never stalls playback.
	rootCtx := context.Background()
	var current *houseTrack
	enqueue := func(trackID, playedAt int64) error {
		return store.EnqueueScrobble(db, enabledServices, trackID, playedAt)
	}
	settle := func() {
		if current == nil {
			return
		}
		if _, err := scrobble.Finish(current.id, current.duration, current.startedAt, time.Now(), enqueue); err != nil {
			log.Printf("scrobble: enqueue track %d: %v", current.id, err)
		}
		current = nil
	}
	next := func() (string, bool) {
		tr, ok := jb.Next(houseID)
		if !ok {
			if current != nil {
				settle()
				houseBus.Publish(events.Event{Type: "now-playing", Data: nil})
			}
			return "", false
		}
		_, path, found := store.GetTrack(db, tr.ID)
		if !found {
			return "", false
		}
		// A new track is starting: settle the one that just finished, then make
		// this the current house track.
		settle()
		current = &houseTrack{id: tr.ID, duration: tr.Duration, startedAt: time.Now()}
		// Now-playing is fire-and-forget — never queued, never retried.
		if lb != nil {
			id := tr.ID
			go func() {
				if m, ok, err := store.ScrobbleMetadata(db, id); err == nil && ok {
					_ = lb.NowPlaying(rootCtx, external.ListenMeta{
						ArtistName: m.ArtistName, TrackName: m.TrackName, ReleaseName: m.ReleaseName})
				}
			}()
		}
		if enriched, err := store.EnrichTracks(db, []model.Track{tr}); err == nil && len(enriched) > 0 {
			houseBus.Publish(events.Event{Type: "now-playing", Data: enriched[0]})
		} else {
			houseBus.Publish(events.Event{Type: "now-playing", Data: tr})
		}
		// The pop removed this track from the queue; tell listeners so their
		// "up next" view doesn't keep showing the now-playing track.
		houseBus.Publish(events.Event{Type: "queue-changed", Data: houseID})
		return path, true
	}

	// Single background drainer delivers queued scrobbles. ctx-aware so #23's
	// graceful shutdown can cancel it without changing the signature.
	if len(submitters) > 0 {
		go scrobble.NewDrainer(db, submitters, 50).Run(rootCtx)
	}

	houseHub := broadcast.NewHub(broadcast.FFmpegSource{}, next, silence)
	go houseHub.Run(context.Background())

	uiFS, err := web.FS()
	if err != nil {
		log.Fatalf("ui fs: %v", err)
	}
	srv := api.NewServer(db, jb, uiFS)
	srv.SetListenAddr(cfg.Addr)
	srv.RegisterStream(houseID, houseHub, houseBus)
	srv.SetScanProgress(scanProgress)

	// MusicBrainz/Cover Art Archive enrichment, triggered via POST /api/enrich.
	// Covers are cached next to the DB file. ≤1 req/sec, descriptive UA.
	coversDir := filepath.Join(filepath.Dir(cfg.DBPath), "covers")
	if err := os.MkdirAll(coversDir, 0o755); err != nil {
		log.Fatalf("covers dir: %v", err)
	}
	srv.SetEnrichRunner(enrich.NewRunner(db,
		external.NewMusicBrainz(extClient), external.NewCoverArt(extClient), coversDir))
	log.Printf("Exit 66 Jukebox listening on %s", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, srv.Handler()); err != nil {
		log.Fatalf("server: %v", err)
	}
}
