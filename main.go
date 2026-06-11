package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/andybarilla/exit66jukebox/internal/api"
	"github.com/andybarilla/exit66jukebox/internal/broadcast"
	"github.com/andybarilla/exit66jukebox/internal/config"
	"github.com/andybarilla/exit66jukebox/internal/events"
	"github.com/andybarilla/exit66jukebox/internal/jukebox"
	"github.com/andybarilla/exit66jukebox/internal/scan"
	"github.com/andybarilla/exit66jukebox/internal/store"
	"github.com/andybarilla/exit66jukebox/internal/web"
)

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

	// Initial scan in the background so the server comes up immediately.
	if roots := cfg.Library(); len(roots) > 0 {
		go func() {
			log.Printf("scanning %v ...", roots)
			res, err := scan.Scan(db, roots, cfg.ScanWorkers)
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
	// for the broadcaster. Called repeatedly; a no-op when the queue is empty.
	next := func() (string, bool) {
		tr, ok := jb.Next(houseID)
		if !ok {
			return "", false
		}
		_, path, found := store.GetTrack(db, tr.ID)
		if !found {
			return "", false
		}
		houseBus.Publish(events.Event{Type: "now-playing", Data: tr})
		return path, true
	}

	houseHub := broadcast.NewHub(broadcast.FFmpegSource{}, next, silence)
	go houseHub.Run(context.Background())

	uiFS, err := web.FS()
	if err != nil {
		log.Fatalf("ui fs: %v", err)
	}
	srv := api.NewServer(db, jb, uiFS)
	srv.RegisterStream(houseID, houseHub, houseBus)
	log.Printf("Exit 66 Jukebox listening on %s", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, srv.Handler()); err != nil {
		log.Fatalf("server: %v", err)
	}
}
