package config

import "flag"

// Config holds runtime options sourced from flags.
type Config struct {
	Addr          string
	DBPath        string
	Roots         multiFlag
	HistoryWindow int
	ScanWorkers   int
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
	return c, nil
}
