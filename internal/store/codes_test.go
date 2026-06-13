package store

import "testing"

func TestSlotLetter(t *testing.T) {
	cases := map[int]string{
		0: "A", 1: "B", 25: "Z",
		26: "AA", 27: "AB", 51: "AZ", 52: "BA",
		701: "ZZ", 702: "AAA",
	}
	for rank, want := range cases {
		if got := slotLetter(rank); got != want {
			t.Errorf("slotLetter(%d) = %q, want %q", rank, got, want)
		}
	}
}

func TestTone(t *testing.T) {
	want := []string{"cyan", "magenta", "amber", "violet", "cyan"}
	for rank, w := range want {
		if got := tone(rank); got != w {
			t.Errorf("tone(%d) = %q, want %q", rank, got, w)
		}
	}
}
