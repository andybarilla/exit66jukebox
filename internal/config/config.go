package config

import (
	"flag"
	"os"
	"strconv"
)

// Config holds runtime options sourced from flags.
type Config struct {
	Addr          string
	DBPath        string
	Roots         multiFlag
	HistoryWindow int
	ScanWorkers   int
	Services      Services
	Federation    Federation

	// MuteLocalOnCast silences the browser's local <audio> while a Sonos cast is
	// active. Sourced from EXIT66_MUTE_LOCAL_ON_CAST for now; a settings UI will
	// replace the env source later. Defaults true when unset.
	MuteLocalOnCast bool

	// AdminPassword is the shared secret that unlocks admin mode (skip, queue
	// removal/clear, shuffle, Sonos casting). Empty leaves the gate disabled and
	// every action open. Unlike service tokens it's exposed as a flag as well as
	// EXIT66_ADMIN_PASSWORD: a deliberately-soft LAN-party gate, not a service
	// credential, so the no-process-list-leak rule is relaxed for ergonomics. A
	// flag value wins over the env var when both are set.
	AdminPassword string
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

// Federation holds peer-sharing config. Role is "hub", "member", or "" (off).
// Like Services, the token comes from the environment, never a flag, so it
// doesn't leak via the process list. HubAddr is the public host:port a member
// dials; Listen is the hub's local listen address. PeerID is this instance's
// stable identifier within the federation.
type Federation struct {
	Role    string // "hub" | "member" | ""
	HubAddr string // members only: hub's public address to dial
	Listen  string // hub only: local address to listen on (e.g. ":8443")
	Token   string // shared secret presented at registration
	PeerID  string // this instance's id (e.g. "home", "vps")
}

// Enabled reports whether federation is configured.
func (f Federation) Enabled() bool { return f.Role == "hub" || f.Role == "member" }

func federationFromEnv() Federation {
	return Federation{
		Role:    os.Getenv("EXIT66_FED_ROLE"),
		HubAddr: os.Getenv("EXIT66_FED_HUB"),
		Listen:  os.Getenv("EXIT66_FED_LISTEN"),
		Token:   os.Getenv("EXIT66_FED_TOKEN"),
		PeerID:  os.Getenv("EXIT66_FED_PEER_ID"),
	}
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
	fs.StringVar(&c.AdminPassword, "admin-password", "", "shared admin-mode password (or EXIT66_ADMIN_PASSWORD); empty disables the gate")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	c.Services = servicesFromEnv()
	c.Federation = federationFromEnv()
	c.MuteLocalOnCast = envBool("EXIT66_MUTE_LOCAL_ON_CAST", true)
	if c.AdminPassword == "" {
		c.AdminPassword = os.Getenv("EXIT66_ADMIN_PASSWORD")
	}
	return c, nil
}

// envBool reads a truthy boolean from the environment. An unset or unparseable
// value falls back to def, so EXIT66_MUTE_LOCAL_ON_CAST defaults on but an
// explicit "false"/"0" turns it off.
func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
