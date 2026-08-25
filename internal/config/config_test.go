package config

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestServicesFromEnv(t *testing.T) {
	t.Setenv("EXIT66_LISTENBRAINZ_TOKEN", "tok-123")
	c, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.Services.ListenBrainzToken != "tok-123" {
		t.Errorf("token = %q, want tok-123", c.Services.ListenBrainzToken)
	}
	if !c.Services.ListenBrainzEnabled() {
		t.Error("ListenBrainzEnabled() = false, want true")
	}
}

func TestServicesAbsentDisabled(t *testing.T) {
	t.Setenv("EXIT66_LISTENBRAINZ_TOKEN", "")
	c, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.Services.ListenBrainzEnabled() {
		t.Error("ListenBrainzEnabled() = true with no token, want false")
	}
}

func TestLastfmServicesFromEnv(t *testing.T) {
	t.Setenv("EXIT66_LASTFM_API_KEY", "key-abc")
	t.Setenv("EXIT66_LASTFM_API_SECRET", "secret-xyz")
	c, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.Services.LastfmAPIKey != "key-abc" {
		t.Errorf("api key = %q, want key-abc", c.Services.LastfmAPIKey)
	}
	if c.Services.LastfmAPISecret != "secret-xyz" {
		t.Errorf("api secret = %q, want secret-xyz", c.Services.LastfmAPISecret)
	}
	if !c.Services.LastfmConfigured() {
		t.Error("LastfmConfigured() = false, want true with key+secret")
	}
}

func TestLastfmConfiguredNeedsBoth(t *testing.T) {
	t.Setenv("EXIT66_LASTFM_API_KEY", "key-abc")
	t.Setenv("EXIT66_LASTFM_API_SECRET", "")
	c, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.Services.LastfmConfigured() {
		t.Error("LastfmConfigured() = true with no secret, want false")
	}
}

func TestMuteLocalOnCastDefaultsTrue(t *testing.T) {
	t.Setenv("EXIT66_MUTE_LOCAL_ON_CAST", "")
	c, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !c.MuteLocalOnCast {
		t.Error("MuteLocalOnCast = false with env unset, want true (default)")
	}
}

func TestMuteLocalOnCastDisabled(t *testing.T) {
	t.Setenv("EXIT66_MUTE_LOCAL_ON_CAST", "false")
	c, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.MuteLocalOnCast {
		t.Error("MuteLocalOnCast = true with env=false, want false")
	}
}

func TestSMTPFromEnv(t *testing.T) {
	t.Setenv("EXIT66_SMTP_HOST", "smtp.example.com")
	t.Setenv("EXIT66_SMTP_FROM", "jukebox@example.com")
	c, err := Parse(nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.SMTP.Host != "smtp.example.com" || c.SMTP.From != "jukebox@example.com" {
		t.Fatalf("SMTP not parsed: %+v", c.SMTP)
	}
	if c.SMTP.Port != "587" {
		t.Fatalf("default port: want 587, got %q", c.SMTP.Port)
	}
}

func TestMFAKeyFromEnvAcceptsBase64(t *testing.T) {
	want := []byte("0123456789abcdef0123456789abcdef")
	t.Setenv("EXIT66_MFA_KEY", base64.StdEncoding.EncodeToString(want))
	c, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !bytes.Equal(c.MFAKey, want) {
		t.Fatalf("MFAKey = %x, want %x", c.MFAKey, want)
	}
}

func TestLoadMFAKeyAcceptsHex(t *testing.T) {
	want := []byte("0123456789abcdef0123456789abcdef")
	got, err := LoadMFAKey(hex.EncodeToString(want))
	if err != nil {
		t.Fatalf("LoadMFAKey: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("LoadMFAKey = %x, want %x", got, want)
	}
}

func TestLoadMFAKeyMissingReturnsNil(t *testing.T) {
	got, err := LoadMFAKey("")
	if err != nil {
		t.Fatalf("LoadMFAKey: %v", err)
	}
	if got != nil {
		t.Fatalf("LoadMFAKey = %x, want nil", got)
	}
}

func TestLoadMFAKeyInvalidLengthReturnsError(t *testing.T) {
	_, err := LoadMFAKey(base64.StdEncoding.EncodeToString([]byte("too-short")))
	if err == nil {
		t.Fatal("LoadMFAKey returned nil error, want invalid length error")
	}
	if !strings.Contains(err.Error(), "EXIT66_MFA_KEY must be 32 bytes") {
		t.Fatalf("LoadMFAKey error = %q, want EXIT66_MFA_KEY must be 32 bytes", err)
	}
}

// Service tokens must never be exposed as flags (they would leak via the process
// list).
func TestTokenNotAFlag(t *testing.T) {
	_, err := Parse([]string{"-listenbrainz-token", "x"})
	if err == nil {
		t.Fatal("expected -listenbrainz-token to be rejected as an unknown flag")
	}
}

// TestEnvExampleHasNoTrailingComments guards the installed sample against
// systemd's EnvironmentFile rule: `#` starts a comment only at the beginning of
// a line, so anything after `=` -- including a hint the operator was never meant
// to keep -- becomes part of the value. The unit interpolates $EXIT66_ARGS
// unquoted, so those words arrive as extra argv, where flag.Parse stops at the
// first non-flag and drops them without a word.
//
// This covers commented-out samples too: their hazard is realised the moment an
// operator uncomments the line.
func TestEnvExampleHasNoTrailingComments(t *testing.T) {
	sample, err := os.ReadFile("../../packaging/exit66.env.example")
	if err != nil {
		t.Fatalf("read env sample: %v", err)
	}
	assignment := regexp.MustCompile(`^#?[A-Z0-9_]+=`)
	checked := 0
	for i, line := range strings.Split(string(sample), "\n") {
		if !assignment.MatchString(line) {
			continue
		}
		checked++
		_, value, _ := strings.Cut(line, "=")
		if strings.Contains(value, "#") {
			t.Errorf("line %d: %q carries a trailing comment; systemd keeps it as part of the value", i+1, line)
		}
	}
	if checked == 0 {
		t.Fatal("matched no assignment lines in the env sample; the test is not reading what it thinks it is")
	}
}
