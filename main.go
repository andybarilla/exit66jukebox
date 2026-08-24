package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/api"
	"github.com/andybarilla/exit66jukebox/internal/auth"
	"github.com/andybarilla/exit66jukebox/internal/broadcast"
	"github.com/andybarilla/exit66jukebox/internal/config"
	"github.com/andybarilla/exit66jukebox/internal/email"
	"github.com/andybarilla/exit66jukebox/internal/enrich"
	"github.com/andybarilla/exit66jukebox/internal/events"
	"github.com/andybarilla/exit66jukebox/internal/external"
	"github.com/andybarilla/exit66jukebox/internal/fed"
	"github.com/andybarilla/exit66jukebox/internal/jukebox"
	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/recommend"
	"github.com/andybarilla/exit66jukebox/internal/scan"
	"github.com/andybarilla/exit66jukebox/internal/scrobble"
	"github.com/andybarilla/exit66jukebox/internal/store"
	"github.com/andybarilla/exit66jukebox/internal/web"
	"github.com/pion/webrtc/v4"
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

	if err := store.PurgeExpiredSessions(db); err != nil {
		log.Printf("purge sessions: %v", err)
	}
	signingSecret, err := store.MediaSigningSecret(db)
	if err != nil {
		log.Fatalf("signing secret: %v", err)
	}
	libraryRoots, err := startupLibraryRoots(db, cfg.Library())
	if err != nil {
		log.Fatalf("library settings: %v", err)
	}
	fedSettings, err := federationSettings(db, cfg.Federation)
	if err != nil {
		log.Fatalf("federation settings: %v", err)
	}

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
	if len(libraryRoots) > 0 {
		scanProgress = &scan.Progress{}
		scanProgress.SetRunning(true)
		go func() {
			defer scanProgress.SetRunning(false)
			log.Printf("scanning %v ...", libraryRoots)
			res, err := scan.Scan(db, libraryRoots, cfg.ScanWorkers, scanProgress)
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

	// next pops the house queue and publishes now-playing; returns the loopback
	// audio URL for the broadcaster. Called repeatedly in the hub's single goroutine.
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
	// The shared stream feeds ffmpeg the instance's own audio endpoint so remote
	// tracks resolve through the same local/remote branch as browser playback.
	_, selfPort, _ := net.SplitHostPort(cfg.Addr)
	selfBaseURL := "http://127.0.0.1:" + selfPort

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
		if _, _, found := store.GetTrack(db, tr.ID); !found {
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
		// The ffmpeg house source fetches this URL with no session cookie. Auth no
		// longer trusts loopback (a same-host reverse proxy would make every request
		// look local), so the URL carries a path-scoped signed token instead.
		src := broadcast.SourceInput(selfBaseURL, tr.ID)
		sig := auth.SignPath(signingSecret, fmt.Sprintf("/api/tracks/%d/audio", tr.ID),
			time.Now().Add(2*time.Hour).Unix())
		return src + "?sig=" + sig, true
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
	srv.SetMFAKey(cfg.MFAKey)
	srv.SetListenAddr(cfg.Addr)
	srv.SetPublicOrigin(cfg.PublicOrigin)
	if token, ok := bootstrapToken(db); ok {
		log.Printf("First admin bootstrap URL: %s", srv.SetBootstrapToken(token))
	}
	srv.SetMuteLocalOnCast(cfg.MuteLocalOnCast)
	srv.SetSigningSecret(signingSecret)
	srv.SetScanWorkers(cfg.ScanWorkers)
	mailer := email.New(email.Config{
		Host: cfg.SMTP.Host, Port: cfg.SMTP.Port, User: cfg.SMTP.User,
		Pass: cfg.SMTP.Pass, From: cfg.SMTP.From,
	})
	if mailer.Enabled() {
		srv.SetInviteEmailer(func(to, link string) {
			if err := mailer.SendInvite(to, link); err != nil {
				log.Printf("invite email to %s: %v", to, err)
			}
		})
		srv.SetPasswordResetEmailer(func(to, link string) {
			if err := mailer.SendPasswordReset(to, link); err != nil {
				log.Printf("password reset email to %s: %v", to, err)
			}
		})
		srv.SetVerificationEmailer(func(to, link string) error {
			return mailer.SendVerification(to, link)
		})
		log.Print("SMTP invite email enabled")
	}
	srv.RegisterStream(houseID, houseHub, houseBus, houseNP)
	srv.SetScanProgress(scanProgress)
	srv.SetActiveFederation(fedSettings)

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

	// Federation: dial/listen for peers and resolve remote-track audio through the
	// relay. MemberHandler exposes this instance's API over the session so a hub
	// can fetch /api/tracks/{id}/audio from it.
	if fedSettings.Enabled {
		reg := fed.NewRegistry()
		signaler := fed.NewSignaler()
		// Caps describe the transports this instance advertises to peers. They ride
		// the token-authenticated session after the handshake. The WebRTC transport
		// is only meaningful for the peer role with direct P2P enabled.
		caps := fed.Capabilities{DirectWebRTC: fedSettings.Role == "peer" && fedSettings.DirectP2P}
		fm := &fed.Manager{
			Role:          fedSettings.Role,
			Token:         fedSettings.Token,
			PeerID:        fedSettings.PeerID,
			HubAddr:       fedSettings.HubAddr, // member: hub to dial
			HubListen:     fedSettings.Listen,  // hub/peer: local listen addr
			MemberHandler: fed.WithCapsRoute(caps, srv.Handler()),
			Registry:      reg,
			DB:            db,
			Caps:          caps,
			Signaler:      signaler,
		}
		fm.SetContext(rootCtx)
		if fedSettings.Role == "hub" {
			// One relay instance shared by the session handler and the resolver so
			// the hub's own browse sees remote tracks (catalog ingest lands here in a
			// later task). It holds the registry and the hub's DB.
			relay := fed.NewRelay(reg, db)
			relay.SetSelf(fedSettings.PeerID)
			fm.Relay = relay
			fm.HubHandler = fed.WithCapsRoute(caps, fed.WithSignalRelay(signaler, relay.Routes()))
		}
		if fedSettings.Role == "peer" {
			fm.PeerHandler = fed.WithCapsRoute(caps, fed.PeerRoutes(db, srv.Handler()))
			fed.StartLANDiscovery(rootCtx, db, fedSettings.PeerID, fedSettings.PeerID, fedSettings.Listen)
			// Direct P2P (WebRTC) transport: NAT-traversing audio path that bypasses
			// the hub when both peers advertise support and ICE connects. Disabled by
			// setting; falls back to the yamux-direct then hub-relay tiers on any
			// failure, so playback is never broken.
			if fedSettings.DirectP2P {
				fm.WebRTC = fed.NewWebRTCTransport(fedSettings.PeerID, fedICEServers(fedSettings), signaler, nil)
			}
		}
		fm.Start()
		srv.SetFedResolver(fed.NewResolverFor(fm))
		srv.SetFedPeers(fm.OnlinePeers)
		log.Printf("federation: role=%s peer=%s direct_p2p=%t", fedSettings.Role, fedSettings.PeerID, fedSettings.DirectP2P)
	}

	log.Printf("Exit 66 Jukebox listening on %s", cfg.Addr)
	publicHandler := srv.RequireAuthMiddleware(srv.Handler())
	httpServer := &http.Server{Addr: cfg.Addr, Handler: publicHandler}
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

// bootstrapToken mints the startup first-admin token, or reports false when the
// instance already has users (or the count/mint fails, in which case the server
// still starts — a missing bootstrap link is not worth refusing to boot over).
func bootstrapToken(db *sql.DB) (string, bool) {
	n, err := store.CountUsers(db)
	if err != nil {
		log.Printf("bootstrap token skipped: count users: %v", err)
		return "", false
	}
	if n != 0 {
		return "", false
	}
	token, err := auth.GenerateToken()
	if err != nil {
		log.Printf("bootstrap token skipped: generate: %v", err)
		return "", false
	}
	return token, true
}

func startupLibraryRoots(db *sql.DB, cliRoots []string) ([]string, error) {
	if err := store.SeedLocalLibraries(db, cliRoots); err != nil {
		return nil, err
	}
	return store.EnabledLocalLibraryRoots(db)
}

func federationSettings(db *sql.DB, env config.Federation) (store.FederationSettings, error) {
	settings, ok, err := store.LoadFederationSettings(db)
	if err != nil {
		return store.FederationSettings{}, err
	}
	if ok {
		return settings, nil
	}
	return store.FederationSettings{
		Enabled:     env.Enabled(),
		Role:        env.Role,
		HubAddr:     env.HubAddr,
		Listen:      env.Listen,
		Token:       env.Token,
		PeerID:      env.PeerID,
		DirectP2P:   env.DirectP2P,
		STUNServers: env.STUNServers,
		TURNURL:     env.TURNURL,
	}, nil
}

// fedICEServers builds the webrtc.ICEServer list from settings: at least one
// STUN server (defaulting to Google's public STUN when none configured), plus a
// TURN server when EXIT66_FED_TURN is set. TURN credentials live in the URL
// (turn://user:pass@host:port) so the federation token never appears here.
var defaultSTUNServer = "stun:stun.l.google.com:19302"

func fedICEServers(s store.FederationSettings) []webrtc.ICEServer {
	stun := s.STUNServers
	if len(stun) == 0 {
		stun = []string{defaultSTUNServer}
	}
	var servers []webrtc.ICEServer
	for _, u := range stun {
		servers = append(servers, webrtc.ICEServer{URLs: []string{u}})
	}
	if s.TURNURL != "" {
		servers = append(servers, webrtc.ICEServer{URLs: []string{s.TURNURL}})
	}
	return servers
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
