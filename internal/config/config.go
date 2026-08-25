package config

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"flag"
	"os"
	"strconv"
	"strings"
)

const mfaKeySize = 32

// Config holds runtime options sourced from flags.
type Config struct {
	Addr          string
	DBPath        string
	Roots         multiFlag
	HistoryWindow int
	ScanWorkers   int
	Services      Services
	Federation    Federation
	SMTP          SMTP
	// SSO holds every configured sign-in provider, in the order the sign-in
	// surface offers them. Empty means password-only.
	SSO           []Provider
	PublicOrigin  string
	MFAKey        []byte

	// MuteLocalOnCast silences the browser's local <audio> while a Sonos is
	// casting the stream that browser is itself playing — a cast of some other
	// stream leaves it alone (#130). Sourced from EXIT66_MUTE_LOCAL_ON_CAST for
	// now; a settings UI will replace the env source later. Defaults true when
	// unset.
	MuteLocalOnCast bool
}

// SMTP holds optional invite-email settings (env only). Host empty = disabled.
type SMTP struct {
	Host, Port, User, Pass, From string
}

func smtpFromEnv() SMTP {
	port := os.Getenv("EXIT66_SMTP_PORT")
	if port == "" {
		port = "587"
	}
	return SMTP{
		Host: os.Getenv("EXIT66_SMTP_HOST"),
		Port: port,
		User: os.Getenv("EXIT66_SMTP_USER"),
		Pass: os.Getenv("EXIT66_SMTP_PASS"),
		From: os.Getenv("EXIT66_SMTP_FROM"),
	}
}

// Provider is one configured sign-in provider. ID is its stable slug: the
// route segment (/api/auth/sso/{id}/start) and the key /api/config advertises
// it under. Issuer is the OpenID issuer URL — used verbatim for discovery, and
// the value external identities are keyed on in oidc_identity, so it must never
// change for a provider once accounts exist against it. ButtonLabel is what the
// sign-in surface calls the provider.
//
// Credentials come from the environment only, like Services: a client secret
// passed as a -flag leaks via the process list.
type Provider struct {
	ID           string
	Issuer       string
	ClientID     string
	ClientSecret string
	ButtonLabel  string
}

// Provider IDs. These are wire values — they appear in the start route and in
// /api/config — so they are fixed here rather than derived from anything the
// operator sets.
const (
	// ProviderOIDC is the generic provider configured with an issuer of the
	// operator's choosing (EXIT66_OIDC_*).
	ProviderOIDC = "oidc"
	// ProviderGoogle is Google, whose issuer is fixed and whose operator only
	// supplies credentials.
	ProviderGoogle = "google"
)

// GoogleIssuer is Google's OpenID issuer. It is a constant rather than an
// operator setting: it is fixed for everyone, it is what Google's ID tokens
// carry as their iss claim, and it is the value google identities are keyed on.
const GoogleIssuer = "https://accounts.google.com"

// GoogleButtonLabel is what the sign-in surface calls Google. Unlike the
// generic provider there is no name to choose — an operator renaming this
// button would only make it less recognisable.
const GoogleButtonLabel = "Google"

// DefaultOIDCButtonLabel names the generic provider on the sign-in surface when
// the operator has not chosen a name.
const DefaultOIDCButtonLabel = "single sign-on"

// Configured reports whether a provider is fully specified. A partial config is
// deliberately "off" rather than an error: the same treatment Services gets, so
// a half-filled env file can never take password login down with it.
func (p Provider) Configured() bool {
	return p.Issuer != "" && p.ClientID != "" && p.ClientSecret != ""
}

// providersFromEnv reads every configured sign-in provider, in the order the
// sign-in surface will offer them: named providers first, because a button
// carrying a name the user recognises belongs above a generic one.
//
// Each provider is its own block of environment variables rather than a list to
// be enumerated (EXIT66_SSO_PROVIDERS=google,corp and per-name variables under
// it). The environment is flat, every named provider we add needs exactly two
// variables because its issuer and label are fixed, and a list is one more
// thing to spell wrong for no gain. The internal type is a list either way, so
// a future need for several *generic* providers can add a list source here
// without changing anything downstream.
func providersFromEnv() []Provider {
	var out []Provider
	google := Provider{
		ID:           ProviderGoogle,
		Issuer:       GoogleIssuer,
		ClientID:     os.Getenv("EXIT66_GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("EXIT66_GOOGLE_CLIENT_SECRET"),
		ButtonLabel:  GoogleButtonLabel,
	}
	if google.Configured() {
		out = append(out, google)
	}
	label := strings.TrimSpace(os.Getenv("EXIT66_OIDC_NAME"))
	if label == "" {
		label = DefaultOIDCButtonLabel
	}
	generic := Provider{
		ID: ProviderOIDC,
		// Whitespace only. A trailing slash is NOT trimmed: discovery requires
		// the issuer claim to equal this string exactly, and some providers
		// (Auth0 among them) really do publish one with the slash.
		Issuer:       strings.TrimSpace(os.Getenv("EXIT66_OIDC_ISSUER")),
		ClientID:     os.Getenv("EXIT66_OIDC_CLIENT_ID"),
		ClientSecret: os.Getenv("EXIT66_OIDC_CLIENT_SECRET"),
		ButtonLabel:  label,
	}
	if generic.Configured() {
		out = append(out, generic)
	}
	return out
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

// Federation holds peer-sharing config. Role is "hub", "member", "peer", or "" (off).
// Like Services, the token comes from the environment, never a flag, so it
// doesn't leak via the process list. HubAddr is the public host:port a member
// dials; Listen is the hub's local listen address. PeerID is this instance's
// stable identifier within the federation.
type Federation struct {
	Role    string // "hub" | "member" | "peer" | ""
	HubAddr string // members only: hub's public address to dial
	Listen  string // hub only: local address to listen on (e.g. ":8443")
	Token   string // shared secret presented at registration
	PeerID  string // this instance's id (e.g. "home", "vps")
	// DirectP2P enables the WebRTC direct transport (peer role) so audio can
	// bypass the hub relay when NAT traversal succeeds. Defaults on.
	DirectP2P   bool
	STUNServers []string
	TURNURL     string
}

// Enabled reports whether federation is configured.
func (f Federation) Enabled() bool { return f.Role == "hub" || f.Role == "member" || f.Role == "peer" }

func federationFromEnv() Federation {
	f := Federation{
		Role:    os.Getenv("EXIT66_FED_ROLE"),
		HubAddr: os.Getenv("EXIT66_FED_HUB"),
		Listen:  os.Getenv("EXIT66_FED_LISTEN"),
		Token:   os.Getenv("EXIT66_FED_TOKEN"),
		PeerID:  os.Getenv("EXIT66_FED_PEER_ID"),
		TURNURL: os.Getenv("EXIT66_FED_TURN"),
	}
	// Direct P2P defaults on; an explicit EXIT66_FED_DIRECT_P2P=0 turns it off.
	f.DirectP2P = os.Getenv("EXIT66_FED_DIRECT_P2P") != "0"
	f.STUNServers = parseSTUNList(os.Getenv("EXIT66_FED_STUN"))
	return f
}

// parseSTUNList splits a comma-separated STUN URL list, trimming spaces. An
// empty value yields a nil slice (callers apply the default STUN server).
func parseSTUNList(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// servicesFromEnv reads service credentials from the environment.
func servicesFromEnv() Services {
	return Services{
		ListenBrainzToken: os.Getenv("EXIT66_LISTENBRAINZ_TOKEN"),
		LastfmAPIKey:      os.Getenv("EXIT66_LASTFM_API_KEY"),
		LastfmAPISecret:   os.Getenv("EXIT66_LASTFM_API_SECRET"),
	}
}

// LoadMFAKey parses EXIT66_MFA_KEY as a 32-byte base64 or hex-encoded key.
func LoadMFAKey(value string) ([]byte, error) {
	if value == "" {
		return nil, nil
	}

	if key, err := base64.StdEncoding.DecodeString(value); err == nil {
		if len(key) == mfaKeySize {
			return key, nil
		}
	}

	if key, err := base64.RawStdEncoding.DecodeString(value); err == nil {
		if len(key) == mfaKeySize {
			return key, nil
		}
	}

	if key, err := hex.DecodeString(value); err == nil {
		if len(key) == mfaKeySize {
			return key, nil
		}
	}

	return nil, errors.New("EXIT66_MFA_KEY must be 32 bytes encoded as base64 or hex")
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
	c.Federation = federationFromEnv()
	c.SMTP = smtpFromEnv()
	c.SSO = providersFromEnv()
	c.PublicOrigin = os.Getenv("EXIT66_PUBLIC_ORIGIN")
	mfaKey, err := LoadMFAKey(os.Getenv("EXIT66_MFA_KEY"))
	if err != nil {
		return Config{}, err
	}
	c.MFAKey = mfaKey
	c.MuteLocalOnCast = envBool("EXIT66_MUTE_LOCAL_ON_CAST", true)
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
