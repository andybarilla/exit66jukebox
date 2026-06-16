package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
)

// SignMedia returns a token authorizing access to trackID until expUnix (unix
// seconds). Format: "<trackID>.<exp>.<sig-b64>", where sig = HMAC(secret,
// "<trackID>.<exp>"). Used for Sonos casts, which fetch audio with no cookie.
func SignMedia(secret []byte, trackID, expUnix int64) string {
	msg := strconv.FormatInt(trackID, 10) + "." + strconv.FormatInt(expUnix, 10)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(msg))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return msg + "." + sig
}

// VerifyMedia checks a token against secret and the current time nowUnix,
// returning the authorized trackID. ok is false for malformed, forged, or
// expired tokens.
func VerifyMedia(secret []byte, token string, nowUnix int64) (trackID int64, ok bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return 0, false
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, false
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, false
	}
	msg := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(msg))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(parts[2])) {
		return 0, false
	}
	if nowUnix >= exp {
		return 0, false
	}
	return id, true
}
