package config

import (
	"flag"
	"os"
)

// Config holds runtime options sourced from flags.
type Config struct {
	Addr          string
	DBPath        string
	Roots         multiFlag
	HistoryWindow int
	ScanWorkers   int
	Services      Services
}

// Services holds external-service credentials. They are read from the
// environment, never from flags: a token passed as -flag leaks via the process
// list. A service with no credentials is simply disabled.
type Services struct {
	ListenBrainzToken string
	LastfmAPIKey      string
	LastfmAPISecret   string
}

// ListenBrainzEnabled reports whether a ListenBrainz token is configured.
func (s Services) ListenBrainzEnabled() bool { return s.ListenBrainzToken != "" }

// LastfmConfigured reports whether both Last.fm credentials are present. Full
// Last.fm enablement also requires a persisted session key (a service_auth row)
// in the database, not config — this only covers the env half.
func (s Services) LastfmConfigured() bool {
	return s.LastfmAPIKey != "" && s.LastfmAPISecret != ""
}

// servicesFromEnv reads service credentials from the environment.
func servicesFromEnv() Services {
	return Services{
		ListenBrainzToken: os.Getenv("EXIT66_LISTENBRAINZ_TOKEN"),
		LastfmAPIKey:      os.Getenv("EXIT66_LASTFM_API_KEY"),
		LastfmAPISecret:   os.Getenv("EXIT66_LASTFM_API_SECRET"),
	}
}

type multiFlag []string

func (m *multiFlag) String() string { return "" }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

// Library returns the configured library roots.
func (c Config) Library() []string { return c.Roots }

// Parse reads flags from the given argument list.
func Parse(args []string) (Config, error) {
	fs := flag.NewFlagSet("exit66", flag.ContinueOnError)
	var c Config
	fs.StringVar(&c.Addr, "addr", ":8066", "listen address")
	fs.StringVar(&c.DBPath, "db", "exit66.db", "SQLite database path")
	fs.IntVar(&c.HistoryWindow, "history", 25, "recently-played window")
	fs.IntVar(&c.ScanWorkers, "workers", 8, "scan worker goroutines")
	fs.Var(&c.Roots, "root", "library root (repeatable)")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	c.Services = servicesFromEnv()
	return c, nil
}
