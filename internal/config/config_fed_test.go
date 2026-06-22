package config

import (
	"testing"
)

func TestFederationFromEnv(t *testing.T) {
	t.Setenv("EXIT66_FED_ROLE", "member")
	t.Setenv("EXIT66_FED_HUB", "hub.example.com:8443")
	t.Setenv("EXIT66_FED_LISTEN", ":8443")
	t.Setenv("EXIT66_FED_TOKEN", "s3cret")
	t.Setenv("EXIT66_FED_PEER_ID", "home")

	f := federationFromEnv()
	if !f.Enabled() {
		t.Fatal("expected federation enabled")
	}
	if f.Role != "member" || f.HubAddr != "hub.example.com:8443" || f.Listen != ":8443" || f.Token != "s3cret" || f.PeerID != "home" {
		t.Fatalf("unexpected federation config: %+v", f)
	}
}

func TestFederationDisabledWhenRoleUnset(t *testing.T) {
	t.Setenv("EXIT66_FED_ROLE", "")
	if federationFromEnv().Enabled() {
		t.Fatal("federation should be disabled when role unset")
	}
}

func TestFederationPeerRoleEnabled(t *testing.T) {
	t.Setenv("EXIT66_FED_ROLE", "peer")

	if !federationFromEnv().Enabled() {
		t.Fatal("peer role should enable federation")
	}
}
