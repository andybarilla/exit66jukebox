package broadcast

import "testing"

func TestSourceInputIsLoopbackAudioURL(t *testing.T) {
	got := SourceInput("http://127.0.0.1:8066", 42)
	want := "http://127.0.0.1:8066/api/tracks/42/audio"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
