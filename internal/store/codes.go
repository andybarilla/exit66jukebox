package store

import (
	"regexp"
	"strconv"
	"strings"
)

// slotLetter maps a 0-based album rank to its crate-wall letter: A, B … Z, AA,
// AB … (base-26, ports format.js albumLetter). The rank is the album's position
// in the global alphabetical (sort_key) order.
func slotLetter(rank int) string {
	n, out := rank, ""
	for {
		out = string(rune('A'+n%26)) + out
		n = n/26 - 1
		if n < 0 {
			break
		}
	}
	return out
}

// tones cycles the four neon gradients used for slot-code tiles (ports
// format.js toneFor).
var tones = []string{"cyan", "magenta", "amber", "violet"}

func tone(rank int) string { return tones[rank%len(tones)] }

// slotCode assembles a track's crate-wall code from its album rank and track
// number, falling back to 1 when the track number is missing (matching the old
// client now-playing/queue fallback), so the same track yields the same code
// everywhere it is serialized.
func slotCode(rank, trackNo int) string {
	n := trackNo
	if n <= 0 {
		n = 1
	}
	return slotLetter(rank) + strconv.Itoa(n)
}

var codePattern = regexp.MustCompile(`^([A-Za-z]+)(\d+)$`)

// parseCode interprets a search string as a slot code (e.g. "A3") and returns
// the album rank and track number it points at. ok is false when the string is
// not a bare letter-run + number.
func parseCode(s string) (rank, trackNo int, ok bool) {
	m := codePattern.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, 0, false
	}
	rank = letterToRank(m[1])
	n, err := strconv.Atoi(m[2])
	if err != nil || rank < 0 {
		return 0, 0, false
	}
	return rank, n, true
}

// letterToRank inverts slotLetter: "A" -> 0, "Z" -> 25, "AA" -> 26, …
func letterToRank(s string) int {
	n := 0
	for _, c := range strings.ToUpper(s) {
		if c < 'A' || c > 'Z' {
			return -1
		}
		n = n*26 + int(c-'A'+1)
	}
	return n - 1
}
