package scan

import "testing"

func TestProbeDurationReadsFixtureLength(t *testing.T) {
	d := probeDuration("testdata/sample.mp3")
	if d <= 0 {
		t.Fatalf("expected positive duration for 1s fixture, got %d", d)
	}
}

func TestProbeDurationMissingFileReturnsZero(t *testing.T) {
	if d := probeDuration("testdata/does-not-exist.mp3"); d != 0 {
		t.Fatalf("expected 0 for missing file, got %d", d)
	}
}
