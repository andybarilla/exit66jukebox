package config

import (
	"os"
	"strings"
	"testing"
)

func TestGenericProviderFromEnv(t *testing.T) {
	t.Setenv("EXIT66_OIDC_ISSUER", "  https://idp.example/  ")
	t.Setenv("EXIT66_OIDC_CLIENT_ID", "jukebox")
	t.Setenv("EXIT66_OIDC_CLIENT_SECRET", "s3cret")
	t.Setenv("EXIT66_OIDC_NAME", "Corp SSO")

	got := providersFromEnv()
	if len(got) != 1 {
		t.Fatalf("providers = %+v, want just the generic one", got)
	}
	o := got[0]
	if o.ID != ProviderOIDC {
		t.Errorf("id = %q, want %q", o.ID, ProviderOIDC)
	}
	// Trailing slash kept: discovery compares the issuer claim to this exactly.
	if o.Issuer != "https://idp.example/" {
		t.Errorf("issuer = %q, want the trimmed value with its trailing slash intact", o.Issuer)
	}
	if o.ClientID != "jukebox" || o.ClientSecret != "s3cret" || o.ButtonLabel != "Corp SSO" {
		t.Errorf("unexpected config: %+v", o)
	}
}

// Google is credentials only: the issuer and the button label are ours, not the
// operator's, so EXIT66_GOOGLE_ISSUER is a variable that deliberately does not
// exist and nothing an operator sets can point the "Google" button elsewhere.
func TestGoogleProviderFromEnv(t *testing.T) {
	t.Setenv("EXIT66_GOOGLE_CLIENT_ID", "1234.apps.googleusercontent.com")
	t.Setenv("EXIT66_GOOGLE_CLIENT_SECRET", "g-s3cret")
	t.Setenv("EXIT66_GOOGLE_ISSUER", "https://evil.example")

	got := providersFromEnv()
	if len(got) != 1 {
		t.Fatalf("providers = %+v, want just Google", got)
	}
	g := got[0]
	if g.ID != ProviderGoogle {
		t.Errorf("id = %q, want %q", g.ID, ProviderGoogle)
	}
	if g.Issuer != GoogleIssuer {
		t.Errorf("issuer = %q, want the fixed %q", g.Issuer, GoogleIssuer)
	}
	if g.ClientID != "1234.apps.googleusercontent.com" || g.ClientSecret != "g-s3cret" {
		t.Errorf("unexpected credentials: %+v", g)
	}
	if g.ButtonLabel != GoogleButtonLabel {
		t.Errorf("button label = %q, want %q", g.ButtonLabel, GoogleButtonLabel)
	}
}

// Both at once, in a fixed order: the order here is the order of the buttons on
// the sign-in screen, so it cannot be left to map iteration.
func TestProvidersCoexistInAFixedOrder(t *testing.T) {
	t.Setenv("EXIT66_GOOGLE_CLIENT_ID", "google-client")
	t.Setenv("EXIT66_GOOGLE_CLIENT_SECRET", "google-secret")
	t.Setenv("EXIT66_OIDC_ISSUER", "https://idp.example")
	t.Setenv("EXIT66_OIDC_CLIENT_ID", "jukebox")
	t.Setenv("EXIT66_OIDC_CLIENT_SECRET", "s3cret")

	got := providersFromEnv()
	if len(got) != 2 {
		t.Fatalf("providers = %+v, want both", got)
	}
	if got[0].ID != ProviderGoogle || got[1].ID != ProviderOIDC {
		t.Errorf("order = %q, %q; want the named provider first", got[0].ID, got[1].ID)
	}
}

func TestNoProvidersWithNoConfiguration(t *testing.T) {
	for _, k := range []string{
		"EXIT66_OIDC_ISSUER", "EXIT66_OIDC_CLIENT_ID", "EXIT66_OIDC_CLIENT_SECRET",
		"EXIT66_OIDC_NAME", "EXIT66_GOOGLE_CLIENT_ID", "EXIT66_GOOGLE_CLIENT_SECRET",
	} {
		t.Setenv(k, "")
	}
	if got := providersFromEnv(); len(got) != 0 {
		t.Errorf("providers = %+v, want none", got)
	}
}

// A half-filled block is off, not an error, and it does not take the other
// provider's block down with it.
func TestPartialProviderIsSkipped(t *testing.T) {
	full := Provider{Issuer: "https://idp.example", ClientID: "jukebox", ClientSecret: "s3cret"}
	if !full.Configured() {
		t.Fatal("a complete provider should be configured")
	}
	for name, cfg := range map[string]Provider{
		"no issuer":        {ClientID: full.ClientID, ClientSecret: full.ClientSecret},
		"no client id":     {Issuer: full.Issuer, ClientSecret: full.ClientSecret},
		"no client secret": {Issuer: full.Issuer, ClientID: full.ClientID},
	} {
		if cfg.Configured() {
			t.Errorf("%s: reported configured", name)
		}
	}

	t.Setenv("EXIT66_GOOGLE_CLIENT_ID", "google-client") // secret missing
	t.Setenv("EXIT66_OIDC_ISSUER", "https://idp.example")
	t.Setenv("EXIT66_OIDC_CLIENT_ID", "jukebox")
	t.Setenv("EXIT66_OIDC_CLIENT_SECRET", "s3cret")
	got := providersFromEnv()
	if len(got) != 1 || got[0].ID != ProviderOIDC {
		t.Errorf("providers = %+v, want the generic one alone", got)
	}
}

func TestGenericButtonLabelDefaults(t *testing.T) {
	t.Setenv("EXIT66_OIDC_ISSUER", "https://idp.example")
	t.Setenv("EXIT66_OIDC_CLIENT_ID", "jukebox")
	t.Setenv("EXIT66_OIDC_CLIENT_SECRET", "s3cret")
	t.Setenv("EXIT66_OIDC_NAME", "   ")
	got := providersFromEnv()
	if len(got) != 1 {
		t.Fatalf("providers = %+v, want the generic one", got)
	}
	if got[0].ButtonLabel != DefaultOIDCButtonLabel {
		t.Errorf("button label = %q, want %q", got[0].ButtonLabel, DefaultOIDCButtonLabel)
	}
}

// The README's Configuration section is the authoritative list of what exists,
// so a variable added here without a row there is invisible to the operator who
// has to set it.
func TestSSOVariablesDocumentedInREADME(t *testing.T) {
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
		"EXIT66_GOOGLE_CLIENT_ID", "EXIT66_GOOGLE_CLIENT_SECRET",
	} {
		if !strings.Contains(string(readme), name) {
			t.Errorf("README does not document %s", name)
		}
		if !strings.Contains(string(sample), name) {
			t.Errorf("packaging/exit66.env.example does not mention %s", name)
		}
	}
}
