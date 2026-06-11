package scan

import (
	"os/exec"
	"strconv"
	"strings"
)

// probeDuration returns the track length in whole seconds via ffprobe, or 0 if
// ffprobe is unavailable or fails. Best-effort: never errors the scan.
func probeDuration(path string) int {
	out, err := exec.Command("ffprobe", "-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", path).Output()
	if err != nil {
		return 0
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0
	}
	return int(f)
}
