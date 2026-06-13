package external

import "testing"

// Known vector: md5("api_keyabcmethodauth.getSessiontokenxyzsecret123"), i.e.
// params sorted by name, concatenated as name+value, secret appended.
func TestSignParamsKnownVector(t *testing.T) {
	got := signParams(map[string]string{
		"method":  "auth.getSession",
		"token":   "xyz",
		"api_key": "abc",
	}, "secret123")
	const want = "8117c5d4c40b151f6c064254246786da"
	if got != want {
		t.Errorf("signParams = %q, want %q", got, want)
	}
}

// format and callback are excluded from the signature base string per the
// Last.fm spec, so adding them must not change the api_sig.
func TestSignParamsExcludesFormatAndCallback(t *testing.T) {
	base := signParams(map[string]string{"api_key": "abc", "method": "m"}, "s")
	withExtras := signParams(map[string]string{
		"api_key": "abc", "method": "m", "format": "json", "callback": "cb",
	}, "s")
	if base != withExtras {
		t.Errorf("format/callback changed the signature: %q vs %q", base, withExtras)
	}
}
