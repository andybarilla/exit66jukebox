package api

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"
)

// fakeIDP is a minimal OpenID provider on loopback: discovery document, JWKS and
// a token endpoint. Nothing here reaches the network — the whole point is that
// the real library does real discovery, real JWKS fetching and real ID-token
// validation against a provider whose every claim the test controls.
type fakeIDP struct {
	srv    *httptest.Server
	issuer string

	// Claims the next ID token carries. issuerClaim and audience default to
	// this provider and the client id; a test overrides one to see it rejected.
	subject       string
	email         string
	emailVerified bool
	name          string
	nonce         string
	issuerClaim   string
	audience      string
	expiry        time.Time

	// signWrong signs with a key that is not in the published JWKS.
	signWrong bool

	// gotVerifier is the PKCE code_verifier the token request carried.
	gotVerifier string
	// gotClientID and gotClientSecret are the credentials it authenticated with.
	gotClientID, gotClientSecret string
}

// idpKeys are generated once: RSA keygen is the slowest thing in this file, and
// every test wants the same two keys — the published one and one that is not.
var idpKeys = sync.OnceValue(func() [2]*rsa.PrivateKey {
	signing, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return [2]*rsa.PrivateKey{signing, other}
})

func newFakeIDP(t *testing.T) *fakeIDP {
	t.Helper()
	idp := &fakeIDP{
		subject:       "subject-1",
		email:         "sso@example.com",
		emailVerified: true,
		name:          "SSO User",
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"issuer":                                idp.issuer,
			"authorization_endpoint":                idp.issuer + "/authorize",
			"token_endpoint":                        idp.issuer + "/token",
			"jwks_uri":                              idp.issuer + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("GET /jwks", func(w http.ResponseWriter, r *http.Request) {
		pub := idpKeys()[0].PublicKey
		writeJSON(w, http.StatusOK, map[string]any{"keys": []map[string]string{{
			"kty": "RSA",
			"alg": "RS256",
			"use": "sig",
			"kid": "test-key",
			"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		idp.gotVerifier = r.Form.Get("code_verifier")
		idp.gotClientID, idp.gotClientSecret = r.Form.Get("client_id"), r.Form.Get("client_secret")
		if user, pass, ok := r.BasicAuth(); ok {
			idp.gotClientID, idp.gotClientSecret = user, pass
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"access_token": "access-token",
			"token_type":   "Bearer",
			"id_token":     idp.idToken(),
		})
	})
	idp.srv = httptest.NewServer(mux)
	idp.issuer = idp.srv.URL
	t.Cleanup(idp.srv.Close)
	return idp
}

// idToken mints an RS256 JWS by hand rather than pulling in a JOSE library:
// header and payload are base64url JSON, and the signature is PKCS#1 v1.5 over
// their SHA-256. Only the test signs tokens; the code under test never does.
func (f *fakeIDP) idToken() string {
	iss, aud := f.issuerClaim, f.audience
	if iss == "" {
		iss = f.issuer
	}
	if aud == "" {
		aud = testOIDCClientID
	}
	exp := f.expiry
	if exp.IsZero() {
		exp = time.Now().Add(time.Hour)
	}
	claims := map[string]any{
		"iss": iss, "aud": aud, "sub": f.subject,
		"iat": time.Now().Add(-time.Minute).Unix(), "exp": exp.Unix(),
		"email": f.email, "email_verified": f.emailVerified, "name": f.name,
		"nonce": f.nonce,
	}
	b64 := base64.RawURLEncoding.EncodeToString
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "kid": "test-key"})
	payload, _ := json.Marshal(claims)
	signingInput := b64(header) + "." + b64(payload)
	key := idpKeys()[0]
	if f.signWrong {
		key = idpKeys()[1]
	}
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		panic(err)
	}
	return signingInput + "." + b64(sig)
}

// verifierMatchesChallenge reports whether the PKCE verifier the token endpoint
// received hashes to the challenge sent on the authorization request.
func verifierMatchesChallenge(verifier, challenge string) bool {
	sum := sha256.Sum256([]byte(verifier))
	return verifier != "" && base64.RawURLEncoding.EncodeToString(sum[:]) == challenge
}

func mustCookie(t *testing.T, res *http.Response, name string) *http.Cookie {
	t.Helper()
	for _, c := range res.Cookies() {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("response set no %s cookie (cookies: %v)", name, res.Cookies())
	return nil
}

func cookieNamed(res *http.Response, name string) *http.Cookie {
	for _, c := range res.Cookies() {
		if c.Name == name && c.MaxAge >= 0 && c.Value != "" {
			return c
		}
	}
	return nil
}

// redirectQuery parses the query of a redirect response's Location.
func redirectQuery(t *testing.T, rec *httptest.ResponseRecorder) url.Values {
	t.Helper()
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("Location %q: %v", rec.Header().Get("Location"), err)
	}
	return loc.Query()
}
