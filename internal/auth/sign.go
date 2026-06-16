package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
)

// SignPath returns a token authorizing GET access to urlPath until expUnix (unix
// seconds). Format: "<exp>.<sig>", sig = HMAC(secret, urlPath + "." + exp). Used
// for Sonos casts, which fetch the house stream URL with no cookie.
func SignPath(secret []byte, urlPath string, expUnix int64) string {
	exp := strconv.FormatInt(expUnix, 10)
	sig := base64.RawURLEncoding.EncodeToString(mac(secret, urlPath+"."+exp))
	return exp + "." + sig
}

// VerifyPath reports whether token authorizes urlPath at nowUnix. False for
// malformed, forged, expired, or wrong-path tokens.
func VerifyPath(secret []byte, token, urlPath string, nowUnix int64) bool {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return false
	}
	exp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return false
	}
	want := base64.RawURLEncoding.EncodeToString(mac(secret, urlPath+"."+parts[0]))
	if !hmac.Equal([]byte(want), []byte(parts[1])) {
		return false
	}
	return nowUnix < exp
}

func mac(secret []byte, msg string) []byte {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(msg))
	return m.Sum(nil)
}
