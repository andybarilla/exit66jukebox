package config

import (
	"os"
	"strings"
	"testing"
)

func TestOIDCFromEnv(t *testing.T) {
	t.Setenv("EXIT66_OIDC_ISSUER", "  https://idp.example/  ")
	t.Setenv("EXIT66_OIDC_CLIENT_ID", "jukebox")
	t.Setenv("EXIT66_OIDC_CLIENT_SECRET", "s3cret")
	t.Setenv("EXIT66_OIDC_NAME", "Corp SSO")

	o := oidcFromEnv()
	if !o.Configured() {
		t.Fatal("a complete provider should be configured")
	}
	// Trailing slash kept: discovery compares the issuer claim to this exactly.
	if o.Issuer != "https://idp.example/" {
		t.Errorf("issuer = %q, want the trimmed value with its trailing slash intact", o.Issuer)
	}
	if o.ClientID != "jukebox" || o.ClientSecret != "s3cret" || o.ButtonLabel != "Corp SSO" {
		t.Errorf("unexpected config: %+v", o)
	}
}

func TestOIDCNeedsAllThreeToBeConfigured(t *testing.T) {
	full := OIDC{Issuer: "https://idp.example", ClientID: "jukebox", ClientSecret: "s3cret"}
	if !full.Configured() {
		t.Fatal("a complete provider should be configured")
	}
	for name, cfg := range map[string]OIDC{
		"no issuer":        {ClientID: full.ClientID, ClientSecret: full.ClientSecret},
		"no client id":     {Issuer: full.Issuer, ClientSecret: full.ClientSecret},
		"no client secret": {Issuer: full.Issuer, ClientID: full.ClientID},
	} {
		if cfg.Configured() {
			t.Errorf("%s: reported configured", name)
		}
	}
}

func TestOIDCButtonLabelDefaults(t *testing.T) {
	t.Setenv("EXIT66_OIDC_NAME", "   ")
	if got := oidcFromEnv().ButtonLabel; got != DefaultOIDCButtonLabel {
		t.Errorf("button label = %q, want %q", got, DefaultOIDCButtonLabel)
	}
}

// The README's Configuration section is the authoritative list of what exists,
// so a variable added here without a row there is invisible to the operator who
// has to set it.
func TestOIDCVariablesDocumentedInREADME(t *testing.T) {
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	sample, err := os.ReadFile("../../packaging/exit66.env.example")
	if err != nil {
		t.Fatalf("read env sample: %v", err)
	}
	for _, name := range []string{
		"EXIT66_OIDC_ISSUER", "EXIT66_OIDC_CLIENT_ID",
		"EXIT66_OIDC_CLIENT_SECRET", "EXIT66_OIDC_NAME",
	} {
		if !strings.Contains(string(readme), name) {
			t.Errorf("README does not document %s", name)
		}
		if !strings.Contains(string(sample), name) {
			t.Errorf("packaging/exit66.env.example does not mention %s", name)
		}
	}
}
