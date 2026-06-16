package config

import "testing"

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

// Service tokens must never be exposed as flags (they would leak via the process
// list).
func TestTokenNotAFlag(t *testing.T) {
	_, err := Parse([]string{"-listenbrainz-token", "x"})
	if err == nil {
		t.Fatal("expected -listenbrainz-token to be rejected as an unknown flag")
	}
}
