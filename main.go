package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/api"
	"github.com/andybarilla/exit66jukebox/internal/broadcast"
	"github.com/andybarilla/exit66jukebox/internal/config"
	"github.com/andybarilla/exit66jukebox/internal/enrich"
	"github.com/andybarilla/exit66jukebox/internal/events"
	"github.com/andybarilla/exit66jukebox/internal/external"
	"github.com/andybarilla/exit66jukebox/internal/jukebox"
	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/recommend"
	"github.com/andybarilla/exit66jukebox/internal/scan"
	"github.com/andybarilla/exit66jukebox/internal/scrobble"
	"github.com/andybarilla/exit66jukebox/internal/store"
	"github.com/andybarilla/exit66jukebox/internal/web"
)

func main() {
	// One-time Last.fm authorization, before flag parsing (the subcommand name is
	// not a flag). Remaining args (e.g. -db) are parsed normally.
	if len(os.Args) > 1 && os.Args[1] == "lastfm-auth" {
		runLastfmAuth(os.Args[2:])
		return
	}

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
	submitters := map[string]scrobble.Submitter{}
	// nowPlayers receive a fire-and-forget notification on each track start.
	var nowPlayers []nowPlayer
	var lb *external.ListenBrainz
	if cfg.Services.ListenBrainzEnabled() {
		lb = external.NewListenBrainz(extClient, cfg.Services.ListenBrainzToken)
		submitters["listenbrainz"] = lb
		nowPlayers = append(nowPlayers, lb)
		log.Print("ListenBrainz scrobbling enabled")
	}
	// Last.fm is enabled only when configured AND a session key was persisted by
	// `exit66 lastfm-auth`; otherwise the client is nil (disabled / pending auth).
	lfm := newLastfm(extClient, db, cfg.Services)
	if lfm != nil {
		submitters["lastfm"] = lfm
		nowPlayers = append(nowPlayers, lfm)
		log.Print("Last.fm scrobbling enabled")
	} else if cfg.Services.LastfmConfigured() {
		log.Print("Last.fm configured but not authorized; run `exit66 lastfm-auth`")
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
	// for the broadcaster. Called repeatedly in the hub's single goroutine.
	// Publishes a null now-playing once when the stream transitions from playing
	// to idle (empty queue).
	//
	// houseNP is the single house current-track + start-time holder: it seeds a
	// client connecting mid-track (#28) and is the same holder the scrobble seam
	// reads from. The broadcast Source is real-time-paced, so houseNP's offset ≈
	// the just-finished track's play time. On each pop (and the play→idle
	// transition) settle() evaluates that finished track against the scrobble
	// threshold and enqueues it for every enabled service when it qualifies. Any
	// network work (now-playing) is fire-and-forget so it never stalls playback.
	houseNP := api.NewNowPlaying()
	// Root context cancelled on SIGINT/SIGTERM. Threaded through every long-lived
	// goroutine (scrobble drainer, now-playing fan-out, house hub) so Ctrl-C stops
	// them cleanly. stop() is called at shutdown to restore default signal handling
	// so a second signal force-exits instead of hanging.
	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	// Services are resolved per call, not captured once: Last.fm can self-disable
	// at runtime (error 9), and nothing should be enqueued for it after that.
	enqueue := func(trackID, playedAt int64) error {
		return store.EnqueueScrobble(db, activeScrobbleServices(cfg.Services.ListenBrainzEnabled(), lfm), trackID, playedAt)
	}
	settle := func() {
		prev, offset, ok := houseNP.Current()
		if !ok {
			return
		}
		end := time.Now()
		start := end.Add(-time.Duration(offset) * time.Second)
		if _, err := scrobble.Finish(prev.ID, prev.Duration, start, end, enqueue); err != nil {
			log.Printf("scrobble: enqueue track %d: %v", prev.ID, err)
		}
	}
	playing := false
	next := func() (string, bool) {
		tr, ok := jb.Next(houseID)
		if !ok {
			if playing {
				playing = false
				settle()
				houseNP.Clear()
				houseBus.Publish(events.Event{Type: "now-playing", Data: nil})
			}
			return "", false
		}
		_, path, found := store.GetTrack(db, tr.ID)
		if !found {
			return "", false
		}
		// A new track is starting: settle the one that just finished, then make
		// this the current house track in the shared holder.
		settle()
		playing = true
		houseNP.Set(tr)
		// Now-playing is fire-and-forget — never queued, never retried — and fans
		// out to every enabled service (ListenBrainz, Last.fm).
		if len(nowPlayers) > 0 {
			id := tr.ID
			go func() {
				m, ok, err := store.ScrobbleMetadata(db, id)
				if err != nil || !ok {
					return
				}
				meta := external.ListenMeta{ArtistName: m.ArtistName, TrackName: m.TrackName, ReleaseName: m.ReleaseName}
				for _, np := range nowPlayers {
					_ = np.NowPlaying(rootCtx, meta)
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
	// hubDone closes once Run returns, after its in-flight play() unwinds and the
	// ffmpeg child is killed via rc.Close(). main waits on it before exiting.
	hubDone := make(chan struct{})
	go func() {
		defer close(hubDone)
		houseHub.Run(rootCtx)
	}()

	uiFS, err := web.FS()
	if err != nil {
		log.Fatalf("ui fs: %v", err)
	}
	srv := api.NewServer(db, jb, uiFS)
	srv.SetListenAddr(cfg.Addr)
	srv.RegisterStream(houseID, houseHub, houseBus, houseNP)
	srv.SetScanProgress(scanProgress)

	// MusicBrainz/Cover Art Archive enrichment, triggered via POST /api/enrich.
	// Covers are cached next to the DB file. ≤1 req/sec, descriptive UA.
	coversDir := filepath.Join(filepath.Dir(cfg.DBPath), "covers")
	if err := os.MkdirAll(coversDir, 0o755); err != nil {
		log.Fatalf("covers dir: %v", err)
	}
	srv.SetEnrichRunner(enrich.NewRunner(db,
		external.NewMusicBrainz(extClient), external.NewCoverArt(extClient), coversDir))

	// External recommendations → Discovery (GET /api/discover/recommended). Each
	// source is independent: ListenBrainz recs run whenever its token is set;
	// Last.fm similar-artist uses an unsigned read (api_key only) and so is built
	// from env creds directly, independent of the scrobble session-key auth flow.
	var recLB recommend.LBSource
	if lb != nil {
		recLB = lb
	}
	var recLF recommend.LFSource
	if cfg.Services.LastfmConfigured() {
		recLF = external.NewLastfm(extClient, cfg.Services.LastfmAPIKey, cfg.Services.LastfmAPISecret, "")
	}
	if recLB != nil || recLF != nil {
		srv.SetRecommendRunner(recommend.NewRunner(db, recLB, recLF))
		log.Print("External recommendations enabled")
	}

	log.Printf("Exit 66 Jukebox listening on %s", cfg.Addr)
	httpServer := &http.Server{Addr: cfg.Addr, Handler: srv.Handler()}
	go func() {
		// ListenAndServe returns ErrServerClosed on a graceful Shutdown; only a
		// real bind/serve failure is fatal.
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	<-rootCtx.Done()
	// Restore default signal handling so a second SIGINT force-exits immediately
	// instead of waiting on the shutdown sequence below.
	stop()
	log.Print("shutting down ...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown: %v", err)
	}
	// Wait for the hub goroutine to unwind (killing the ffmpeg child) before
	// exiting, but never hang on it past the bounded timeout.
	if !waitForClose(hubDone, 5*time.Second) {
		log.Print("hub did not stop in time; exiting anyway")
	}
}

// waitForClose blocks until done is closed or timeout elapses, returning true if
// done closed first. It bounds shutdown so a stuck goroutine cannot hang exit.
func waitForClose(done <-chan struct{}, timeout time.Duration) bool {
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// nowPlayer is anything that accepts a fire-and-forget now-playing notification.
// Both ListenBrainz and Last.fm clients satisfy it.
type nowPlayer interface {
	NowPlaying(context.Context, external.ListenMeta) error
}

// newLastfm builds a Last.fm client only when it is both configured (env creds)
// and authorized (a persisted session key). It returns nil otherwise — disabled
// or pending `exit66 lastfm-auth`. On an invalid session at runtime the client
// clears its service_auth row, reverting cleanly to pending-auth.
func newLastfm(c *external.Client, db *sql.DB, svc config.Services) *external.Lastfm {
	if !svc.LastfmConfigured() {
		return nil
	}
	key, _, ok, err := store.GetServiceAuth(db, "lastfm")
	if err != nil {
		log.Printf("lastfm: reading session: %v", err)
		return nil
	}
	if !ok {
		return nil
	}
	lfm := external.NewLastfm(c, svc.LastfmAPIKey, svc.LastfmAPISecret, key)
	lfm.SetOnDisabled(func() {
		if err := store.DeleteServiceAuth(db, "lastfm"); err != nil {
			log.Printf("lastfm: clearing invalid session: %v", err)
		}
	})
	return lfm
}

// activeScrobbleServices is the live set of services to enqueue for, recomputed
// each call so a Last.fm self-disable (error 9) stops enqueueing immediately.
func activeScrobbleServices(listenBrainz bool, lfm *external.Lastfm) []string {
	var svcs []string
	if listenBrainz {
		svcs = append(svcs, "listenbrainz")
	}
	if lfm != nil && lfm.Authorized() {
		svcs = append(svcs, "lastfm")
	}
	return svcs
}

// runLastfmAuth performs the one-time desktop auth flow: getToken, prompt the
// user to approve in a browser, getSession, and persist the session key.
func runLastfmAuth(args []string) {
	cfg, err := config.Parse(args)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if !cfg.Services.LastfmConfigured() {
		log.Fatal("set EXIT66_LASTFM_API_KEY and EXIT66_LASTFM_API_SECRET first")
	}
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	c := external.New("exit66jukebox/0.1 (+https://github.com/andybarilla/exit66jukebox)", time.Second)
	lfm := external.NewLastfm(c, cfg.Services.LastfmAPIKey, cfg.Services.LastfmAPISecret, "")
	ctx := context.Background()

	token, err := lfm.GetToken(ctx)
	if err != nil {
		log.Fatalf("lastfm getToken: %v", err)
	}
	fmt.Println("Open this URL in a browser and approve access:")
	fmt.Println("  " + lfm.AuthorizeURL(token))
	fmt.Print("Press Enter once you have approved... ")
	bufio.NewReader(os.Stdin).ReadString('\n')

	key, username, err := lfm.GetSession(ctx, token)
	if err != nil {
		log.Fatalf("lastfm getSession: %v", err)
	}
	if err := store.PutServiceAuth(db, "lastfm", key, username); err != nil {
		log.Fatalf("persist session: %v", err)
	}
	fmt.Printf("Last.fm authorized as %s.\n", username)
}
