package main

import (
	"log"
	"net/http"
	"os"

	"github.com/andybarilla/exit66jukebox/internal/api"
	"github.com/andybarilla/exit66jukebox/internal/config"
	"github.com/andybarilla/exit66jukebox/internal/jukebox"
	"github.com/andybarilla/exit66jukebox/internal/scan"
	"github.com/andybarilla/exit66jukebox/internal/store"
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

	srv := api.NewServer(db, jb)
	log.Printf("Exit 66 Jukebox listening on %s", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, srv.Handler()); err != nil {
		log.Fatalf("server: %v", err)
	}
}
