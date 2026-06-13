package external

import (
	"crypto/md5"
	"encoding/hex"
	"sort"
)

// signParams computes a Last.fm api_sig: every request param except format and
// callback, sorted by name and concatenated as name+value, with the shared
// secret appended, hashed with md5. (Last.fm API auth spec.)
func signParams(params map[string]string, secret string) string {
	names := make([]string, 0, len(params))
	for name := range params {
		if name == "format" || name == "callback" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	var b []byte
	for _, name := range names {
		b = append(b, name...)
		b = append(b, params[name]...)
	}
	b = append(b, secret...)
	sum := md5.Sum(b)
	return hex.EncodeToString(sum[:])
}
