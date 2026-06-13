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

// Tokens must never be exposed as flags (they would leak via the process list).
func TestTokenNotAFlag(t *testing.T) {
	_, err := Parse([]string{"-listenbrainz-token", "x"})
	if err == nil {
		t.Fatal("expected -listenbrainz-token to be rejected as an unknown flag")
	}
}
