package broadcast

import (
	"fmt"
	"io"
	"os/exec"
	"strconv"
)

// SourceInput returns the ffmpeg -i argument for a track: the instance's own
// loopback audio URL for the track's local id. Local and remote tracks both
// resolve through /api/tracks/{id}/audio (which branches on source_peer), so the
// broadcaster never needs to know which is which.
func SourceInput(selfBaseURL string, localTrackID int64) string {
	return fmt.Sprintf("%s/api/tracks/%d/audio", selfBaseURL, localTrackID)
}

// FFmpegSource transcodes an audio file or URL to a real-time-paced MP3 byte
// stream using ffmpeg's -re flag (read input at native rate), so the shared feed
// advances in real time and late joiners hear the current position.
type FFmpegSource struct{}

func (FFmpegSource) Open(input string) (io.ReadCloser, error) {
	cmd := exec.Command("ffmpeg",
		"-re", "-i", input,
		"-vn", "-f", "mp3", "-b:a", "192k", "-")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &procReadCloser{cmd: cmd, r: stdout}, nil
}

type procReadCloser struct {
	cmd *exec.Cmd
	r   io.ReadCloser
}

func (p *procReadCloser) Read(b []byte) (int, error) { return p.r.Read(b) }

func (p *procReadCloser) Close() error {
	_ = p.cmd.Process.Kill()
	return p.cmd.Wait()
}

// GenerateSilence renders `seconds` of MP3 silence via ffmpeg, used by the hub
// to keep listeners connected when the queue is empty. Returns nil on failure;
// the hub treats empty silence as "send nothing".
func GenerateSilence(seconds int) []byte {
	out, err := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo",
		"-t", strconv.Itoa(seconds), "-f", "mp3", "-b:a", "192k", "-").Output()
	if err != nil {
		return nil
	}
	return out
}
