package store

import "testing"

func TestNormalizeSortKey(t *testing.T) {
	cases := []struct{ in, want string }{
		{"The Beatles", "beatles"},
		{"A Tribe Called Quest", "tribe called quest"},
		{"An Awesome Album", "awesome album"},
		{"...And Justice For All", "and justice for all"},
		{"  Spaced Out ", "spaced out"},
		{"ABBA", "abba"},
		// "a"/"an"/"the" only stripped as a leading article followed by a space,
		// not when they're the whole name or part of a word.
		{"Theory", "theory"},
		{"Anodyne", "anodyne"},
	}
	for _, c := range cases {
		if got := normalizeSortKey(c.in); got != c.want {
			t.Errorf("normalizeSortKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeSortKeyNaturalNumeric(t *testing.T) {
	// Numbers must sort naturally: "Album 2" before "Album 10".
	two := normalizeSortKey("Album 2")
	ten := normalizeSortKey("Album 10")
	if !(two < ten) {
		t.Errorf("expected %q < %q for natural-numeric order", two, ten)
	}
	// And digit runs anywhere in the string get padded.
	if normalizeSortKey("Track 3 Reprise") >= normalizeSortKey("Track 12 Reprise") {
		t.Errorf("expected 'Track 3' to sort before 'Track 12'")
	}
}
