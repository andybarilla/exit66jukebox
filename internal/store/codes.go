package store

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
