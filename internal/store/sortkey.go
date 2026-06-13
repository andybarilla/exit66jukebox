package store

import (
	"regexp"
	"strings"
)

// The backend is the single source of truth for library ordering, so the sort
// key only needs to be deterministic and naturally ordered — it does not have to
// match the old client-side compareNames accent handling exactly.

var (
	leadingPunct = regexp.MustCompile(`^[^\p{L}\p{N}]+`)
	leadingThe   = regexp.MustCompile(`(?i)^(the|a|an)\s+`)
	digitRun     = regexp.MustCompile(`\d+`)
)

// normalizeSortKey lowercases a name, strips leading punctuation and a leading
// article (the/a/an), and zero-pads digit runs so numbers sort naturally
// ("Album 2" before "Album 10"). Digit runs longer than padWidth keep their full
// length and still sort correctly relative to shorter runs.
func normalizeSortKey(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = leadingPunct.ReplaceAllString(s, "")
	s = leadingThe.ReplaceAllString(s, "")
	s = digitRun.ReplaceAllStringFunc(s, padNumber)
	return strings.TrimSpace(s)
}

const padWidth = 10

func padNumber(run string) string {
	if len(run) >= padWidth {
		return run
	}
	return strings.Repeat("0", padWidth-len(run)) + run
}
